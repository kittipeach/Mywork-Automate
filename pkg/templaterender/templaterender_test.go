package templaterender

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mywork/automate/pkg/expression"
)

// baseCtx builds an EvalContext exercised by the render tests: a couple of
// vars, a flow run date + params, and a node "Q" with items and meta so the
// header/trailer/aggregate cases have real data to evaluate against.
func baseCtx() expression.EvalContext {
	return expression.EvalContext{
		Vars: map[string]any{
			"x":       "XVAL",
			"empty":   "",
			"account": "0123456789",
		},
		Flow: expression.FlowContext{
			// A fixed date so format layouts are deterministic.
			RunDate: time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC),
			Params: map[string]any{
				"period": "2026-07",
			},
		},
		Nodes: map[string]expression.NodeResult{
			"Q": {
				Items: []map[string]any{
					{"amount": 100.0},
					{"amount": 250.5},
				},
				Meta: map[string]any{
					"rowCount": 2,
				},
			},
		},
	}
}

func TestRender_Success(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "pure literal no placeholders",
			tmpl: "just some literal text.",
			want: "just some literal text.",
		},
		{
			name: "empty template",
			tmpl: "",
			want: "",
		},
		{
			name: "single placeholder",
			tmpl: "hello {{ $vars.x }}!",
			want: "hello XVAL!",
		},
		{
			name: "placeholder is the whole template",
			tmpl: "{{$vars.x}}",
			want: "XVAL",
		},
		{
			name: "placeholder at start",
			tmpl: "{{ $vars.x }} trailing",
			want: "XVAL trailing",
		},
		{
			name: "placeholder at end",
			tmpl: "leading {{ $vars.x }}",
			want: "leading XVAL",
		},
		{
			name: "multiple placeholders with literal between",
			tmpl: "a {{ $vars.x }} b {{ $flow.params.period }} c",
			want: "a XVAL b 2026-07 c",
		},
		{
			name: "adjacent placeholders no separator",
			tmpl: "{{ $vars.x }}{{ $flow.params.period }}",
			want: "XVAL2026-07",
		},
		{
			name: "whitespace inside braces is trimmed",
			tmpl: "[{{   $vars.x   }}]",
			want: "[XVAL]",
		},
		{
			name: "no-inner-space form equals spaced form",
			tmpl: "[{{$vars.x}}]",
			want: "[XVAL]",
		},
		{
			name: "placeholder evaluating to empty string",
			tmpl: "pre{{ $vars.empty }}post",
			want: "prepost",
		},
		{
			name: "escaped open braces emit literal braces",
			tmpl: `use \{{ to open`,
			want: "use {{ to open",
		},
		{
			name: "escaped close braces emit literal braces",
			tmpl: `close with \}\}`,
			want: "close with }}",
		},
		{
			name: "escaped braces around a real placeholder",
			tmpl: `\{{ raw \}} then {{ $vars.x }}`,
			want: "{{ raw }} then XVAL",
		},
		{
			name: "escaped backslash then placeholder",
			tmpl: `path\\{{ $vars.x }}`,
			want: `path\XVAL`,
		},
		{
			name: "lone backslash is literal",
			tmpl: `a\b`,
			want: `a\b`,
		},
		{
			name: "trailing backslash is literal",
			tmpl: `tail\`,
			want: `tail\`,
		},
		{
			name: "dynamic filename with format pipe",
			tmpl: `payroll_{{ $flow.runDate | format:"20060102" }}.xlsx`,
			want: "payroll_20260714.xlsx",
		},
		{
			name: "header record with node meta rowCount",
			tmpl: `H{{ $node["Q"].meta.rowCount }}`,
			want: "H2",
		},
		{
			name: "trailer record with sum aggregate over items",
			tmpl: `T{{ $node["Q"].items | sum:"amount" }}`,
			want: "T350.5",
		},
		{
			name: "multi-line header block with expression",
			tmpl: "company\nperiod: {{ $flow.params.period }}\n",
			want: "company\nperiod: 2026-07\n",
		},
	}

	ec := baseCtx()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, ec)
			if err != nil {
				t.Fatalf("Render(%q) unexpected error: %v", tt.tmpl, err)
			}
			if got != tt.want {
				t.Fatalf("Render(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestRender_Errors(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		// wantIs is the sentinel the returned error must match via errors.Is.
		wantIs error
		// wantSubstr, if non-empty, must appear in the error message so we know
		// the context (offset / which placeholder) is propagated.
		wantSubstr []string
	}{
		{
			name:       "unterminated placeholder",
			tmpl:       "before {{ $vars.x",
			wantIs:     ErrUnterminated,
			wantSubstr: []string{"offset 7"},
		},
		{
			name:       "unterminated placeholder at very start",
			tmpl:       "{{ nope",
			wantIs:     ErrUnterminated,
			wantSubstr: []string{"offset 0"},
		},
		{
			name:       "empty placeholder braces",
			tmpl:       "a{{}}b",
			wantIs:     ErrEmptyPlaceholder,
			wantSubstr: []string{"offset 1"},
		},
		{
			name:       "whitespace-only placeholder",
			tmpl:       "a{{    }}b",
			wantIs:     ErrEmptyPlaceholder,
			wantSubstr: []string{"offset 1"},
		},
		{
			name:       "evaluation error propagated with offset and body",
			tmpl:       "x {{ $vars.missing }} y",
			wantIs:     ErrEval,
			wantSubstr: []string{"offset 2", "$vars.missing"},
		},
		{
			name:       "evaluation error unknown node",
			tmpl:       `{{ $node["Nope"].items | count }}`,
			wantIs:     ErrEval,
			wantSubstr: []string{"offset 0"},
		},
	}

	ec := baseCtx()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, ec)
			if err == nil {
				t.Fatalf("Render(%q) = %q, want error", tt.tmpl, got)
			}
			if got != "" {
				t.Fatalf("Render(%q) returned %q on error, want empty string", tt.tmpl, got)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("Render(%q) error = %v, want errors.Is %v", tt.tmpl, err, tt.wantIs)
			}
			for _, sub := range tt.wantSubstr {
				if !strings.Contains(err.Error(), sub) {
					t.Fatalf("Render(%q) error = %q, want it to contain %q", tt.tmpl, err.Error(), sub)
				}
			}
		})
	}
}

// TestRender_EvalErrorWrapsExpressionSentinel proves the propagated evaluation
// error still matches the expression package's own sentinel, so callers can
// classify it either way.
func TestRender_EvalErrorWrapsExpressionSentinel(t *testing.T) {
	_, err := Render("{{ $vars.missing }}", baseCtx())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, expression.ErrEval) {
		t.Fatalf("error %v does not wrap expression.ErrEval", err)
	}
}

// FuzzRender asserts the single hard invariant of a security-critical parser:
// no input, however malformed, may ever panic. The seed corpus covers the
// tricky brace/backslash boundaries.
func FuzzRender(f *testing.F) {
	seeds := []string{
		"",
		"literal",
		"{{ $vars.x }}",
		"{{$vars.x}}{{$flow.params.period}}",
		"{{",
		"}}",
		"{{}}",
		"{{   }}",
		"{{ unterminated",
		`\{{`,
		`\}`,
		`\\`,
		`\`,
		`a\`,
		"{{{{}}}}",
		`{{ $node["Q"].items | sum:"amount" }}`,
		"{",
		"}",
		"{{ {{ }} }}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	ec := baseCtx()
	f.Fuzz(func(t *testing.T, tmpl string) {
		// Must not panic. The error value itself is irrelevant here.
		_, _ = Render(tmpl, ec)
	})
}
