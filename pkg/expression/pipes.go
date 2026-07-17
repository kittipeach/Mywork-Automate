package expression

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	// Embed the IANA tz database so timezone resolution (pipeTz) never reads the
	// host filesystem — keeps the sandbox IO-free and behaviour host-independent.
	_ "time/tzdata"
)

// maxPadWidth bounds the output width of padLeft/padRight. Pad width is an
// attacker-controllable literal; without a cap, e.g. padLeft:500000000 would
// allocate and loop far past the 100ms sandbox budget (a DoS). Real fixed-width
// records never approach this, so exceeding it is treated as an error.
const maxPadWidth = 1 << 16 // 65536

// pipeFunc is the signature of a pipe function: it takes the piped-in value and
// the parsed literal arguments, and returns the transformed value.
type pipeFunc func(v any, args []any) (any, error)

// pipeFuncs is the registry of all supported pipe functions. It is a package
// var (not const) so tests can inject a panicking function to exercise the
// recover path.
var pipeFuncs = map[string]pipeFunc{
	"upper":    pipeUpper,
	"lower":    pipeLower,
	"format":   pipeFormat,
	"tz":       pipeTz,
	"addDays":  pipeAddDays,
	"number":   pipeNumber,
	"padLeft":  pipePadLeft,
	"padRight": pipePadRight,
	"sum":      pipeSum,
	"count":    pipeCount,
	"avg":      pipeAvg,
}

// applyPipe looks up and invokes a pipe function.
func applyPipe(p pipeNode, v any) (any, error) {
	fn, ok := pipeFuncs[p.fn]
	if !ok {
		return nil, fmt.Errorf("%w: unknown function %q", ErrEval, p.fn)
	}
	return fn(v, p.args)
}

// arityErr builds a consistent "wrong number of args" error.
func arityErr(fn string, want string, got int) error {
	return fmt.Errorf("%w: function %q expects %s, got %d", ErrEval, fn, want, got)
}

func pipeUpper(v any, args []any) (any, error) {
	if len(args) != 0 {
		return nil, arityErr("upper", "no arguments", len(args))
	}
	return strings.ToUpper(coerceString(v)), nil
}

func pipeLower(v any, args []any) (any, error) {
	if len(args) != 0 {
		return nil, arityErr("lower", "no arguments", len(args))
	}
	return strings.ToLower(coerceString(v)), nil
}

// pipeFormat formats a time value (or an RFC3339 string) with a Go layout.
func pipeFormat(v any, args []any) (any, error) {
	if len(args) != 1 {
		return nil, arityErr("format", "1 layout argument", len(args))
	}
	layout, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: format layout must be a string", ErrEval)
	}
	t, err := asTime(v)
	if err != nil {
		return nil, fmt.Errorf("%w: format expects a time value: %w", ErrEval, err)
	}
	return t.Format(layout), nil
}

// pipeTz converts a time value into the given IANA timezone.
func pipeTz(v any, args []any) (any, error) {
	if len(args) != 1 {
		return nil, arityErr("tz", "1 timezone argument", len(args))
	}
	zone, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: tz argument must be a string", ErrEval)
	}
	t, err := asTime(v)
	if err != nil {
		return nil, fmt.Errorf("%w: tz expects a time value: %w", ErrEval, err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("%w: unknown timezone %q: %w", ErrEval, zone, err)
	}
	return t.In(loc), nil
}

// pipeAddDays shifts a time value by N calendar days.
func pipeAddDays(v any, args []any) (any, error) {
	if len(args) != 1 {
		return nil, arityErr("addDays", "1 numeric argument", len(args))
	}
	n, ok := args[0].(float64)
	if !ok {
		return nil, fmt.Errorf("%w: addDays argument must be a number", ErrEval)
	}
	t, err := asTime(v)
	if err != nil {
		return nil, fmt.Errorf("%w: addDays expects a time value: %w", ErrEval, err)
	}
	return t.AddDate(0, 0, int(n)), nil
}

// pipeNumber formats a numeric value with a fixed number of decimals
// (default 0).
func pipeNumber(v any, args []any) (any, error) {
	if len(args) > 1 {
		return nil, arityErr("number", "0 or 1 arguments", len(args))
	}
	decimals := 0
	if len(args) == 1 {
		d, ok := args[0].(float64)
		if !ok {
			return nil, fmt.Errorf("%w: number decimals argument must be a number", ErrEval)
		}
		decimals = int(d)
	}
	f, err := toFloat(v)
	if err != nil {
		return nil, fmt.Errorf("%w: number expects a numeric value: %w", ErrEval, err)
	}
	return strconv.FormatFloat(f, 'f', decimals, 64), nil
}

func pipePadLeft(v any, args []any) (any, error) {
	return pad(v, args, "padLeft", true)
}

func pipePadRight(v any, args []any) (any, error) {
	return pad(v, args, "padRight", false)
}

// pad implements padLeft/padRight: width, and an optional 1-char pad
// (default space).
func pad(v any, args []any, name string, left bool) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, arityErr(name, "1 or 2 arguments (width[, pad])", len(args))
	}
	w, ok := args[0].(float64)
	if !ok {
		return nil, fmt.Errorf("%w: %s width must be a number", ErrEval, name)
	}
	width := int(w)
	if width > maxPadWidth {
		return nil, fmt.Errorf("%w: %s width %d exceeds maximum %d", ErrEval, name, width, maxPadWidth)
	}
	padStr := " "
	if len(args) == 2 {
		p, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s pad must be a string", ErrEval, name)
		}
		if p != "" {
			padStr = p
		}
	}
	s := coerceString(v)
	// Build the pad prefix/suffix a rune at a time so the result width is
	// exact even for multi-rune pad strings. No unbounded loop: the count is
	// bounded by the requested width.
	deficit := width - len([]rune(s))
	if deficit <= 0 {
		return s, nil
	}
	padRunes := []rune(padStr)
	fill := make([]rune, deficit)
	for i := 0; i < deficit; i++ {
		fill[i] = padRunes[i%len(padRunes)]
	}
	if left {
		return string(fill) + s, nil
	}
	return s + string(fill), nil
}

// pipeSum sums a slice; with a field arg it sums that field across item maps.
func pipeSum(v any, args []any) (any, error) {
	total, _, err := reduceNumbers(v, args, "sum")
	if err != nil {
		return nil, err
	}
	return total, nil
}

// pipeAvg averages a slice; with a field arg it averages that field. Empty
// input yields 0.
func pipeAvg(v any, args []any) (any, error) {
	total, n, err := reduceNumbers(v, args, "avg")
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return float64(0), nil
	}
	return total / float64(n), nil
}

// pipeCount returns the length of a slice.
func pipeCount(v any, args []any) (any, error) {
	if len(args) != 0 {
		return nil, arityErr("count", "no arguments", len(args))
	}
	s, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: count expects an array value, got %T", ErrEval, v)
	}
	return len(s), nil
}

// reduceNumbers walks a slice and returns (sum, count). With an optional field
// argument it reads that field from each element map.
func reduceNumbers(v any, args []any, name string) (float64, int, error) {
	if len(args) > 1 {
		return 0, 0, arityErr(name, "0 or 1 field arguments", len(args))
	}
	s, ok := v.([]any)
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s expects an array value, got %T", ErrEval, name, v)
	}
	var field string
	hasField := len(args) == 1
	if hasField {
		f, ok := args[0].(string)
		if !ok {
			return 0, 0, fmt.Errorf("%w: %s field argument must be a string", ErrEval, name)
		}
		field = f
	}
	var total float64
	for _, el := range s {
		var num any
		if hasField {
			m, ok := el.(map[string]any)
			if !ok {
				return 0, 0, fmt.Errorf("%w: %s field %q requires object items, got %T", ErrEval, name, field, el)
			}
			val, ok := m[field]
			if !ok {
				return 0, 0, fmt.Errorf("%w: %s field %q not found in item", ErrEval, name, field)
			}
			num = val
		} else {
			num = el
		}
		f, err := toFloat(num)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: %s requires numeric values: %w", ErrEval, name, err)
		}
		total += f
	}
	return total, len(s), nil
}
