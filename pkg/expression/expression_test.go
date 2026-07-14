package expression

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// baseCtx builds a representative EvalContext used across many tests.
func baseCtx() EvalContext {
	return EvalContext{
		Nodes: map[string]NodeResult{
			"Query Payroll": {
				Items: []map[string]any{
					{"name": "alice", "salary": 100, "dept": "eng"},
					{"name": "bob", "salary": 200, "dept": "eng"},
					{"name": "carol", "salary": 300, "dept": "hr"},
				},
				Meta: map[string]any{"rowCount": 3, "source": "pg"},
			},
			"Empty Node": {
				Items: []map[string]any{},
				Meta:  map[string]any{"rowCount": 0},
			},
		},
		Flow: FlowContext{
			RunDate: time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC),
			Params: map[string]any{
				"period": "2026-07",
				"count":  int64(5),
			},
		},
		Vars: map[string]any{
			"foo":    "bar",
			"n":      42,
			"nested": map[string]any{"deep": "value"},
			"list":   []any{"a", "b", "c"},
		},
	}
}

func TestEval_References(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want any
	}{
		{"flow runDate", `$flow.runDate`, time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC)},
		{"flow params dot", `$flow.params.period`, "2026-07"},
		{"flow params bracket", `$flow.params["period"]`, "2026-07"},
		{"vars dot", `$vars.foo`, "bar"},
		{"vars bracket", `$vars["foo"]`, "bar"},
		{"vars nested dot", `$vars.nested.deep`, "value"},
		{"vars list index", `$vars.list[1]`, "b"},
		{"node meta rowCount", `$node["Query Payroll"].meta.rowCount`, 3},
		{"node meta bracket", `$node["Query Payroll"].meta["source"]`, "pg"},
		{"node items index field", `$node["Query Payroll"].items[0].name`, "alice"},
		{"node items whole", `$node["Query Payroll"].items[2].salary`, 300},
		{"empty node items", `$node["Empty Node"].meta.rowCount`, 0},
		{"whitespace tolerated", `  $vars.foo  `, "bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if !equalVal(got, tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_StringAndNumberLiterals(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want any
	}{
		{"double quoted string", `"hello"`, "hello"},
		{"string with escaped quote", `"he said \"hi\""`, `he said "hi"`},
		{"string with escaped backslash", `"a\\b"`, `a\b`},
		{"int literal", `7`, float64(7)},
		{"negative int", `-3`, float64(-3)},
		{"float literal", `3.5`, float64(3.5)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if !equalVal(got, tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_PipeUpperLower(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"upper", `$vars.foo | upper`, "BAR"},
		{"lower", `"HELLO" | lower`, "hello"},
		{"chained", `$vars.foo | upper | lower`, "bar"},
		{"upper on number coerces", `$vars.n | upper`, "42"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if got != any(tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_PipeFormatAndTz(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"format date", `$flow.runDate | format:"2006-01-02"`, "2026-07-14"},
		{"format datetime", `$flow.runDate | format:"2006-01-02 15:04"`, "2026-07-14 09:30"},
		{"tz then format", `$flow.runDate | tz:"Asia/Bangkok" | format:"2006-01-02 15:04"`, "2026-07-14 16:30"},
		{"addDays then format", `$flow.runDate | addDays:1 | format:"2006-01-02"`, "2026-07-15"},
		{"addDays negative", `$flow.runDate | addDays:-2 | format:"2006-01-02"`, "2026-07-12"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if got != any(tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_FormatOnStringDate(t *testing.T) {
	// format should parse an RFC3339 string then re-format.
	ec := EvalContext{Vars: map[string]any{"d": "2026-07-14T09:30:00Z"}}
	got, err := Eval(`$vars.d | format:"2006-01-02"`, ec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != any("2026-07-14") {
		t.Fatalf("got %#v", got)
	}
}

func TestEval_PipeNumber(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"number with 2 decimals", `$vars.n | number:2`, "42.00"},
		{"number zero decimals", `$vars.n | number:0`, "42"},
		{"number from string", `"3.14159" | number:2`, "3.14"},
		{"number default decimals", `$vars.n | number`, "42"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if got != any(tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_PipePad(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"padLeft zeros", `$vars.n | padLeft:5:"0"`, "00042"},
		{"padRight spaces", `$vars.foo | padRight:5:"x"`, "barxx"},
		{"padLeft no-op when long enough", `"abcdef" | padLeft:3:"0"`, "abcdef"},
		{"padLeft default space", `$vars.foo | padLeft:5`, "  bar"},
		{"padRight default space", `$vars.foo | padRight:5`, "bar  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if got != any(tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

// TestEval_PadWidthCapped proves the sandbox DoS guard: an attacker-controlled
// oversized pad width is rejected fast, not looped/allocated over.
func TestEval_PadWidthCapped(t *testing.T) {
	ec := baseCtx()
	for _, body := range []string{
		`$vars.foo | padLeft:500000000:"0"`,
		`$vars.foo | padRight:70000`,
	} {
		start := time.Now()
		_, err := Eval(body, ec)
		if err == nil {
			t.Fatalf("Eval(%q) = nil error, want width-cap error", body)
		}
		if !errors.Is(err, ErrEval) {
			t.Fatalf("Eval(%q) error = %v, want wraps ErrEval", body, err)
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Fatalf("Eval(%q) took %v, want fast rejection within sandbox budget", body, elapsed)
		}
	}
}

func TestEval_PipeAggregations(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want any
	}{
		{"sum salary", `$node["Query Payroll"].items | sum:"salary"`, float64(600)},
		{"avg salary", `$node["Query Payroll"].items | avg:"salary"`, float64(200)},
		{"count items", `$node["Query Payroll"].items | count`, 3},
		{"count empty", `$node["Empty Node"].items | count`, 0},
		{"sum of plain number slice", `$vars.nums | sum`, float64(6)},
		{"avg of plain number slice", `$vars.nums | avg`, float64(2)},
	}
	ec.Vars["nums"] = []any{1, 2, 3}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if err != nil {
				t.Fatalf("Eval(%q) unexpected error: %v", tc.body, err)
			}
			if !equalVal(got, tc.want) {
				t.Fatalf("Eval(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestEval_AvgEmptyIsZero(t *testing.T) {
	ec := baseCtx()
	got, err := Eval(`$node["Empty Node"].items | avg:"salary"`, ec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != any(float64(0)) {
		t.Fatalf("avg over empty = %#v, want 0", got)
	}
}

func TestEvalString(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"string passthrough", `$vars.foo`, "bar"},
		{"int coerced", `$vars.n`, "42"},
		{"int64 coerced", `$flow.params.count`, "5"},
		{"float whole coerced", `7`, "7"},
		{"float fractional coerced", `3.5`, "3.5"},
		{"time coerced RFC3339", `$flow.runDate`, "2026-07-14T09:30:00Z"},
		{"bool coerced", `$vars.b`, "true"},
		{"pipe result", `$flow.runDate | format:"2006"`, "2026"},
	}
	ec.Vars["b"] = true
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvalString(tc.body, ec)
			if err != nil {
				t.Fatalf("EvalString(%q) unexpected error: %v", tc.body, err)
			}
			if got != tc.want {
				t.Fatalf("EvalString(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestEvalString_MapAndSliceCoercion(t *testing.T) {
	ec := baseCtx()
	// A map/slice value has no natural scalar form; coerce via fmt fallback.
	got, err := EvalString(`$vars.nested`, ec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "deep") {
		t.Fatalf("map coercion = %q, want to contain 'deep'", got)
	}
}

func TestEvalString_Error(t *testing.T) {
	ec := baseCtx()
	if _, err := EvalString(`$vars.missing`, ec); err == nil {
		t.Fatalf("expected error for missing var")
	}
}

func TestEval_Errors(t *testing.T) {
	ec := baseCtx()
	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{"empty body", ``, "empty expression"},
		{"only whitespace", `   `, "empty expression"},
		{"unknown root", `$bogus.foo`, "unknown reference"},
		{"unknown var", `$vars.missing`, "unknown"},
		{"unknown node", `$node["Nope"].items`, "unknown node"},
		{"unknown flow field", `$flow.bogus`, "unknown"},
		{"unknown node field", `$node["Query Payroll"].bogus`, "unknown"},
		{"unknown pipe fn", `$vars.foo | frobnicate`, "unknown function"},
		{"bad arity format", `$flow.runDate | format`, "expects"},
		{"bad arity too many", `$vars.foo | upper:"x"`, "expects"},
		{"format on non-time", `$vars.foo | format:"2006"`, "format"},
		{"tz bad zone", `$flow.runDate | tz:"Not/AZone" | format:"2006"`, "timezone"},
		{"addDays on non-time", `$vars.foo | addDays:1`, "addDays"},
		{"addDays bad arg type", `$flow.runDate | addDays:"x"`, "addDays"},
		{"number on non-numeric", `$vars.foo | number:2`, "number"},
		{"number bad decimals arg", `$vars.n | number:"x"`, "number"},
		{"padLeft bad width", `$vars.foo | padLeft:"x"`, "padLeft"},
		{"padLeft bad pad type", `$vars.foo | padLeft:5:3`, "pad"},
		{"sum on non-slice", `$vars.foo | sum`, "sum"},
		{"sum missing field value", `$node["Query Payroll"].items | sum:"nope"`, "field"},
		{"sum non-numeric field", `$node["Query Payroll"].items | sum:"name"`, "numeric"},
		{"count on non-slice", `$vars.foo | count`, "count"},
		{"index out of range", `$vars.list[9]`, "out of range"},
		{"negative index", `$vars.list[-1]`, "out of range"},
		{"index into scalar", `$vars.foo.bar`, "cannot"},
		{"index into scalar bracket", `$vars.foo[0]`, "cannot"},
		{"missing map key", `$vars.nested.absent`, "unknown"},
		{"unbalanced bracket", `$node["Query Payroll".items`, "expected"},
		{"unterminated string", `"abc`, "unterminated"},
		{"unterminated node string", `$node["abc`, "unterminated"},
		{"bad number literal", `1.2.3`, "number"},
		{"trailing pipe", `$vars.foo |`, "expected function"},
		{"leading pipe", `| upper`, "unexpected"},
		{"unexpected char", `@`, "unexpected"},
		{"node without bracket", `$node.items`, "expected"},
		{"node bracket not string", `$node[0]`, "expected"},
		{"dot without name", `$vars.`, "expected"},
		{"pipe arg unterminated string", `$vars.foo | padLeft:5:"x`, "unterminated"},
		{"bracket empty", `$vars.list[]`, "expected"},
		{"trailing junk after expr", `$vars.foo bar`, "unexpected"},
		{"pipe colon then junk", `$vars.n | number:`, "expected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Eval(tc.body, ec)
			if err == nil {
				t.Fatalf("Eval(%q) expected error containing %q, got nil", tc.body, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Eval(%q) error = %q, want substring %q", tc.body, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEval_NilMaps(t *testing.T) {
	// All context maps nil — references must error cleanly, never panic.
	ec := EvalContext{}
	for _, body := range []string{
		`$vars.foo`,
		`$flow.params.period`,
		`$node["X"].items`,
	} {
		if _, err := Eval(body, ec); err == nil {
			t.Fatalf("Eval(%q) with nil context expected error", body)
		}
	}
}

func TestEval_ErrorsAreWrapped(t *testing.T) {
	ec := baseCtx()
	_, err := Eval(`$vars.missing`, ec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrEval) {
		t.Fatalf("error %v is not wrapping ErrEval", err)
	}
}

func TestEval_Timeout(t *testing.T) {
	// Force the deadline to be already exceeded via the step budget: an
	// expression with more pipe stages than the step budget must abort.
	ec := baseCtx()
	saved := maxSteps
	maxSteps = 2
	defer func() { maxSteps = saved }()

	body := `$vars.foo | upper | lower | upper | lower | upper`
	_, err := Eval(body, ec)
	if err == nil {
		t.Fatal("expected timeout/step-budget error")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected timeout/budget error, got %v", err)
	}
}

func TestEval_TimeoutDeadline(t *testing.T) {
	// The wall-clock deadline path: a zero timeout must trip immediately.
	ec := baseCtx()
	saved := evalTimeout
	evalTimeout = 0
	defer func() { evalTimeout = saved }()

	_, err := Eval(`$vars.foo | upper`, ec)
	if err == nil {
		t.Fatal("expected deadline error with zero timeout")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestEval_NoPanicRecover(t *testing.T) {
	// Register a pipe fn that panics, ensure recover converts to error.
	ec := baseCtx()
	pipeFuncs["boom"] = func(v any, args []any) (any, error) {
		panic("kaboom")
	}
	defer delete(pipeFuncs, "boom")

	_, err := Eval(`$vars.foo | boom`, ec)
	if err == nil {
		t.Fatal("expected recovered panic to become error")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic-wrapped error, got %v", err)
	}
}

func TestEval_NumberCoercionVariants(t *testing.T) {
	// Exercise toFloat across the numeric kinds through sum over a slice.
	ec := EvalContext{Vars: map[string]any{
		"ints":    []any{int(1), int8(2), int16(3), int32(4), int64(5)},
		"uints":   []any{uint(1), uint8(2), uint16(3), uint32(4), uint64(5)},
		"floats":  []any{float32(1.5), float64(2.5)},
		"strs":    []any{"1", "2", "3"},
		"badstr":  []any{"x"},
		"boolbad": []any{true},
	}}
	cases := []struct {
		body string
		want float64
	}{
		{`$vars.ints | sum`, 15},
		{`$vars.uints | sum`, 15},
		{`$vars.floats | sum`, 4},
		{`$vars.strs | sum`, 6},
	}
	for _, c := range cases {
		got, err := Eval(c.body, ec)
		if err != nil {
			t.Fatalf("Eval(%q) error: %v", c.body, err)
		}
		if got != any(c.want) {
			t.Fatalf("Eval(%q)=%#v want %v", c.body, got, c.want)
		}
	}
	if _, err := Eval(`$vars.badstr | sum`, ec); err == nil {
		t.Fatal("expected error summing non-numeric string")
	}
	if _, err := Eval(`$vars.boolbad | sum`, ec); err == nil {
		t.Fatal("expected error summing bool")
	}
}

// equalVal compares expected vs actual, treating any int-ish/float-ish
// numbers by value so tests can specify plain int literals.
func equalVal(got, want any) bool {
	if gt, ok := got.(time.Time); ok {
		if wt, ok := want.(time.Time); ok {
			return gt.Equal(wt)
		}
		return false
	}
	gf, gok := asFloat(got)
	wf, wok := asFloat(want)
	if gok && wok {
		return math.Abs(gf-wf) < 1e-9
	}
	return got == want
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return float64(n), true
	default:
		return 0, false
	}
}

func TestCoerceString_NumericTypes(t *testing.T) {
	// Drive coerceString through EvalString for every scalar kind a $vars
	// value can hold. Each value is a real reference, coerced to string.
	cases := []struct {
		name string
		val  any
		want string
	}{
		{"float32", float32(1.5), "1.5"},
		{"int", int(7), "7"},
		{"int8", int8(-8), "-8"},
		{"int16", int16(16), "16"},
		{"int32", int32(32), "32"},
		{"int64", int64(64), "64"},
		{"uint", uint(1), "1"},
		{"uint8", uint8(8), "8"},
		{"uint16", uint16(16), "16"},
		{"uint32", uint32(32), "32"},
		{"uint64", uint64(64), "64"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ec := EvalContext{Vars: map[string]any{"x": c.val}}
			got, err := EvalString(`$vars.x`, ec)
			if err != nil {
				t.Fatalf("EvalString error: %v", err)
			}
			if got != c.want {
				t.Fatalf("coerce %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestEval_FlowBareAndIndex(t *testing.T) {
	ec := baseCtx()
	// $flow with no accessor.
	if _, err := Eval(`$flow`, ec); err == nil ||
		!strings.Contains(err.Error(), "expected .runDate") {
		t.Fatalf("$flow bare: got err=%v", err)
	}
	// Indexing into $flow with [...] is not allowed.
	if _, err := Eval(`$flow["runDate"]`, ec); err == nil ||
		!strings.Contains(err.Error(), "cannot index into $flow") {
		t.Fatalf("$flow index: got err=%v", err)
	}
}

func TestEval_NilNodeMeta(t *testing.T) {
	// A node with nil Meta and a nil inner item map must resolve via mapAny,
	// not panic. Accessing a missing meta key yields an error, not a crash.
	ec := EvalContext{Nodes: map[string]NodeResult{
		"N": {Items: []map[string]any{nil}, Meta: nil},
	}}
	// .meta is an empty map -> unknown field.
	if _, err := Eval(`$node["N"].meta.rowCount`, ec); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("nil meta: got err=%v", err)
	}
	// items[0] is an empty map -> unknown field.
	if _, err := Eval(`$node["N"].items[0].x`, ec); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("nil item: got err=%v", err)
	}
}

func TestEval_WalkStepBudget(t *testing.T) {
	// A deep accessor chain must abort when the step budget runs out mid-walk.
	ec := EvalContext{Vars: map[string]any{
		"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": "x"}}},
	}}
	saved := maxSteps
	maxSteps = 2
	defer func() { maxSteps = saved }()
	if _, err := Eval(`$vars.a.b.c.d`, ec); err == nil ||
		!strings.Contains(err.Error(), "budget") {
		t.Fatalf("walk budget: got err=%v", err)
	}
}

func TestEval_FormatOnNumber(t *testing.T) {
	// format on a numeric (non-time, non-string) value hits the asTime default.
	ec := EvalContext{Vars: map[string]any{"n": 5}}
	if _, err := Eval(`$vars.n | format:"2006"`, ec); err == nil ||
		!strings.Contains(err.Error(), "not a time") {
		t.Fatalf("format on number: got err=%v", err)
	}
}

func TestEval_MoreParseAndPipeErrors(t *testing.T) {
	ec := baseCtx()
	ec.Vars["nums"] = []any{1, 2, 3}
	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{"dollar then non-ident", `$.foo`, "expected reference name"},
		{"node accessor malformed", `$node["Query Payroll"].`, "expected field name"},
		{"non-integer index", `$vars.list[1.5]`, "must be an integer"},
		{"index not closed", `$vars.list[0`, "expected ']'"},
		{"trailing string junk", `$vars.foo "x"`, "unexpected token"},
		{"lower arity", `"x" | lower:"y"`, "expects"},
		{"format layout not string", `$flow.runDate | format:2`, "layout must be a string"},
		{"tz arity zero", `$flow.runDate | tz`, "expects"},
		{"tz arg not string", `$flow.runDate | tz:2`, "must be a string"},
		{"addDays arity zero", `$flow.runDate | addDays`, "expects"},
		{"number arity too many", `$vars.n | number:1:2`, "expects"},
		{"pad arity zero", `$vars.foo | padLeft`, "expects"},
		{"pad arity three", `$vars.foo | padLeft:5:"0":"x"`, "expects"},
		{"count arity", `$vars.nums | count:1`, "expects"},
		{"reduce arity too many", `$vars.nums | sum:"a":"b"`, "expects"},
		{"sum field arg not string", `$vars.nums | sum:2`, "must be a string"},
		{"sum field over non-objects", `$vars.nums | sum:"x"`, "object items"},
		{"avg non-slice", `$vars.foo | avg`, "avg"},
		{"escape default", `"a\zb" | upper`, ""}, // not an error; asserts value below
		{"trailing backslash string", `"abc\`, "unterminated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.body, ec)
			if tc.name == "escape default" {
				if err != nil {
					t.Fatalf("escape default unexpected error: %v", err)
				}
				if got != any(`A\ZB`) {
					t.Fatalf("escape default = %#v, want %q", got, `A\ZB`)
				}
				return
			}
			if err == nil {
				t.Fatalf("Eval(%q) expected error containing %q, got nil", tc.body, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Eval(%q) error = %q, want substring %q", tc.body, err.Error(), tc.wantSub)
			}
		})
	}
}

func FuzzEval(f *testing.F) {
	seeds := []string{
		`$vars.foo`,
		`$flow.runDate | format:"2006-01-02"`,
		`$node["X"].items | sum:"salary"`,
		`"literal" | upper | padLeft:8:"0"`,
		`$vars.list[0]`,
		`123 | number:2`,
		``,
		`$flow.params.period | tz:"Asia/Bangkok"`,
		`| | | :::`,
		`$node[`,
		`"unterminated`,
		`$vars.n | number:"x"`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	ec := baseCtx()
	ec.Vars["nums"] = []any{1, 2, 3}
	f.Fuzz(func(t *testing.T, body string) {
		// Must never panic regardless of input. Errors are fine.
		_, _ = Eval(body, ec)
		_, _ = EvalString(body, ec)
	})
}
