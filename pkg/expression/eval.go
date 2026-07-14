package expression

import (
	"context"
	"fmt"
)

// evaluator carries per-evaluation state: the deadline context, the data
// context, and the remaining step budget.
type evaluator struct {
	ctx    context.Context
	ec     EvalContext
	budget int
}

// step consumes one unit of the step budget and checks the wall-clock
// deadline. It returns an error (never panics) when either limit is hit, so the
// sandbox can never run unbounded.
func (ev *evaluator) step() error {
	if err := ev.ctx.Err(); err != nil {
		return fmt.Errorf("%w: evaluation timeout exceeded: %v", ErrEval, err)
	}
	ev.budget--
	if ev.budget < 0 {
		return fmt.Errorf("%w: evaluation step budget exceeded (possible timeout)", ErrEval)
	}
	return nil
}

// eval resolves the primary value then applies each pipe stage in order.
func (ev *evaluator) eval(node exprNode) (any, error) {
	if err := ev.step(); err != nil {
		return nil, err
	}
	val, err := ev.evalPrimary(node.primary)
	if err != nil {
		return nil, err
	}
	for _, pipe := range node.pipes {
		if err := ev.step(); err != nil {
			return nil, err
		}
		val, err = applyPipe(pipe, val)
		if err != nil {
			return nil, err
		}
	}
	return val, nil
}

// evalPrimary returns the literal value or resolves a reference.
func (ev *evaluator) evalPrimary(p primaryNode) (any, error) {
	if !p.isRef {
		return p.literal, nil
	}
	return ev.evalRef(p.ref)
}

// evalRef resolves the reference root, then walks its accessor chain.
func (ev *evaluator) evalRef(ref refNode) (any, error) {
	var cur any
	switch ref.root {
	case "flow":
		// $flow itself is not addressable; require at least one accessor.
		return ev.evalFlow(ref)
	case "vars":
		if ev.ec.Vars == nil {
			return nil, fmt.Errorf("%w: unknown reference $vars (no variables in context)", ErrEval)
		}
		cur = mapAny(ev.ec.Vars)
	case "node":
		nr, ok := ev.ec.Nodes[ref.nodeName]
		if !ok {
			return nil, fmt.Errorf("%w: unknown node %q", ErrEval, ref.nodeName)
		}
		cur = nodeValue(nr)
	}
	return ev.walk(cur, ref.accessors)
}

// evalFlow handles $flow.<field> resolution because FlowContext is a struct,
// not a map.
func (ev *evaluator) evalFlow(ref refNode) (any, error) {
	if len(ref.accessors) == 0 {
		return nil, fmt.Errorf("%w: unknown reference $flow (expected .runDate or .params)", ErrEval)
	}
	first := ref.accessors[0]
	if first.isIndex {
		return nil, fmt.Errorf("%w: cannot index into $flow with [...]; use .runDate or .params", ErrEval)
	}
	var cur any
	switch first.name {
	case "runDate":
		cur = ev.ec.Flow.RunDate
	case "params":
		if ev.ec.Flow.Params == nil {
			return nil, fmt.Errorf("%w: unknown reference $flow.params (no params in context)", ErrEval)
		}
		cur = mapAny(ev.ec.Flow.Params)
	default:
		return nil, fmt.Errorf("%w: unknown field %q on $flow", ErrEval, first.name)
	}
	return ev.walk(cur, ref.accessors[1:])
}

// nodeValue exposes a NodeResult as a map so accessors (.items/.meta) work
// uniformly with the generic walker.
func nodeValue(nr NodeResult) map[string]any {
	items := make([]any, len(nr.Items))
	for i, it := range nr.Items {
		items[i] = mapAny(it)
	}
	return map[string]any{
		"items": items,
		"meta":  mapAny(nr.Meta),
	}
}

// mapAny normalizes a map[string]any (which may be nil) to a non-nil value so
// downstream lookups produce "unknown key" errors rather than panics.
func mapAny(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// walk applies a chain of accessors to a value.
func (ev *evaluator) walk(cur any, accs []accessor) (any, error) {
	for _, a := range accs {
		if err := ev.step(); err != nil {
			return nil, err
		}
		next, err := access(cur, a)
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

// access performs one accessor step against the current value.
func access(cur any, a accessor) (any, error) {
	if a.isIndex && a.keyIsInt {
		return indexSlice(cur, a.keyInt)
	}
	// Both ".name" and ["strkey"] are string-keyed map lookups.
	key := a.name
	if a.isIndex {
		key = a.keyStr
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: cannot access field %q on a non-object value (%T)", ErrEval, key, cur)
	}
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("%w: unknown field %q", ErrEval, key)
	}
	return v, nil
}

// indexSlice performs integer indexing into a []any.
func indexSlice(cur any, idx int) (any, error) {
	s, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: cannot index into a non-array value (%T) with [%d]", ErrEval, cur, idx)
	}
	if idx < 0 || idx >= len(s) {
		return nil, fmt.Errorf("%w: index %d out of range (len %d)", ErrEval, idx, len(s))
	}
	return s[idx], nil
}
