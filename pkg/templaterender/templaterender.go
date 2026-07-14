// Package templaterender is the multi-placeholder string layer for MyWork
// Automate file generation (spec 08 E7-S1, spec 05 §3). It scans a template
// string for `{{ ... }}` placeholders, evaluates the body of each one with the
// sandboxed pkg/expression engine, and splices the results back into the
// surrounding literal text.
//
// It is used for dynamic filenames (`payroll_{{ $flow.runDate | format:"20060102" }}.xlsx`),
// header/trailer records (`H{{ $node["Q"].meta.rowCount }}`, `T{{ $node["Q"].items | sum:"amount" }}`)
// and any static+expression row in a template.
//
// # Escaping rule
//
// A backslash (`\`) is the escape character:
//   - `\{` renders a literal `{`
//   - `\}` renders a literal `}`
//   - `\\` renders a literal `\`
//
// An escaped brace is emitted as a literal and is NEVER treated as part of a
// `{{` / `}}` placeholder delimiter, so `\{{` renders `{{` verbatim. A
// backslash before any other byte (or a trailing backslash at end of input) is
// emitted literally, followed by that byte. Escaping applies only to literal
// text: the interior of a placeholder is handed verbatim to the expression
// engine, which owns its own string/quoting rules.
//
// The package performs no IO, is deterministic, and never panics on any input
// (see FuzzRender). Errors always wrap one of the sentinels below and carry the
// byte offset of the offending placeholder for diagnostics.
package templaterender

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mywork/automate/pkg/expression"
)

// Sentinel errors. Callers can classify failures with errors.Is; Render wraps
// these with fmt.Errorf %w to add the placeholder offset and body.
var (
	// ErrUnterminated is returned when a `{{` has no matching `}}`.
	ErrUnterminated = errors.New("templaterender: unterminated placeholder")
	// ErrEmptyPlaceholder is returned for `{{}}` or a whitespace-only body.
	ErrEmptyPlaceholder = errors.New("templaterender: empty placeholder")
)

// ErrEval is re-exported from pkg/expression so callers can classify an
// evaluation failure via errors.Is(err, templaterender.ErrEval) without
// importing the expression package directly. A propagated evaluation error
// wraps this value.
var ErrEval = expression.ErrEval

const (
	openDelim  = "{{"
	closeDelim = "}}"
)

// Render scans tmpl for `{{ ... }}` placeholders, evaluates each body via
// expression.EvalString against ec, and splices the results into the
// surrounding literal text. On any error it returns an empty string and a
// wrapped error identifying the offending placeholder by byte offset.
func Render(tmpl string, ec expression.EvalContext) (string, error) {
	var b strings.Builder
	// The rendered output is never longer than the template plus expansions;
	// pre-sizing to len(tmpl) avoids the initial reallocations for the common
	// literal-heavy case.
	b.Grow(len(tmpl))

	i := 0
	n := len(tmpl)
	for i < n {
		c := tmpl[i]

		// Escape handling in literal text.
		if c == '\\' {
			// A trailing backslash (nothing follows) is a literal backslash.
			if i+1 >= n {
				b.WriteByte('\\')
				i++
				continue
			}
			next := tmpl[i+1]
			switch next {
			case '{', '}', '\\':
				// \{ \} \\ -> emit the escaped byte literally, consume both.
				b.WriteByte(next)
			default:
				// Backslash before anything else is not an escape: emit both
				// bytes verbatim.
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			i += 2
			continue
		}

		// Placeholder start: an unescaped "{{".
		if strings.HasPrefix(tmpl[i:], openDelim) {
			rendered, consumed, err := renderPlaceholder(tmpl, i, ec)
			if err != nil {
				return "", err
			}
			b.WriteString(rendered)
			i += consumed
			continue
		}

		// Ordinary literal byte (including a lone '}' or a single '{').
		b.WriteByte(c)
		i++
	}

	return b.String(), nil
}

// renderPlaceholder handles the placeholder that begins at start (where
// tmpl[start:] starts with "{{"). It returns the evaluated replacement text and
// the number of bytes consumed (delimiters included), or a wrapped error tagged
// with the placeholder's byte offset.
func renderPlaceholder(tmpl string, start int, ec expression.EvalContext) (string, int, error) {
	// Locate the closing "}}" after the opening delimiter.
	rest := tmpl[start+len(openDelim):]
	end := strings.Index(rest, closeDelim)
	if end < 0 {
		return "", 0, fmt.Errorf("%w at offset %d", ErrUnterminated, start)
	}

	body := rest[:end]
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", 0, fmt.Errorf("%w at offset %d", ErrEmptyPlaceholder, start)
	}

	out, err := expression.EvalString(trimmed, ec)
	if err != nil {
		return "", 0, fmt.Errorf("placeholder {{%s}} at offset %d: %w", body, start, err)
	}

	consumed := len(openDelim) + end + len(closeDelim)
	return out, consumed, nil
}
