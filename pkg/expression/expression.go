// Package expression implements the MyWork Automate sandboxed expression
// language (docs/spec/06 §3). It evaluates the body found INSIDE {{ }} template
// delimiters; the brace-scanning template layer lives elsewhere.
//
// The engine is deliberately minimal and standard-library only. It performs no
// IO, no network, no filesystem access and has no loops that can run
// unbounded: every evaluation is bounded by both a wall-clock deadline
// (evalTimeout, default 100ms per docs/spec/07) and a step budget (maxSteps).
// It never panics; any internal panic is recovered and returned as an error.
package expression

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrEval is the sentinel wrapped by every error this package returns, so
// callers can classify expression failures with errors.Is.
var ErrEval = errors.New("expression")

// evalTimeout is the maximum wall-clock time a single evaluation may take.
// It is a package var (not a const) so tests can drive the deadline path.
var evalTimeout = 100 * time.Millisecond

// maxSteps bounds the number of evaluation steps (references resolved + pipe
// stages executed). It is a package var so tests can drive the budget path.
var maxSteps = 10000

// EvalContext is the sandboxed data available to an expression. No IO, no
// network.
type EvalContext struct {
	Nodes map[string]NodeResult // keyed by node name: $node["Query Payroll"]
	Flow  FlowContext
	Vars  map[string]any // $vars.foo
}

// NodeResult is the output of a previously executed node.
type NodeResult struct {
	Items []map[string]any // .items
	Meta  map[string]any   // .meta.rowCount, etc.
}

// FlowContext holds flow-scoped values.
type FlowContext struct {
	RunDate time.Time      // $flow.runDate
	Params  map[string]any // $flow.params.period
}

// Eval evaluates ONE expression body (the text INSIDE {{ }}), returning its
// value. It never panics: an internal panic is recovered and returned as an
// error wrapping ErrEval.
func Eval(body string, ec EvalContext) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("%w: internal panic: %v", ErrEval, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	ev := &evaluator{ctx: ctx, ec: ec, budget: maxSteps}

	node, err := parse(body)
	if err != nil {
		return nil, err
	}
	return ev.eval(node)
}

// EvalString evaluates and coerces the result to a string.
func EvalString(body string, ec EvalContext) (string, error) {
	v, err := Eval(body, ec)
	if err != nil {
		return "", err
	}
	return coerceString(v), nil
}
