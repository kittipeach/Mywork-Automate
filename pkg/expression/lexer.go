package expression

import (
	"fmt"
	"strconv"
	"strings"
)

// tokenKind enumerates the lexical token types of the expression language.
type tokenKind int

const (
	tokEOF    tokenKind = iota
	tokDollar           // $
	tokIdent            // node, flow, vars, items, meta, upper, ...
	tokDot              // .
	tokLBrack           // [
	tokRBrack           // ]
	tokPipe             // |
	tokColon            // :
	tokString           // "..."
	tokNumber           // 123, -3, 3.5
)

// token is a single lexical unit with its source value.
type token struct {
	kind tokenKind
	val  string  // for tokIdent/tokString (unquoted)/tokNumber (raw)
	num  float64 // parsed value for tokNumber (validated at lex time)
	pos  int     // byte offset in source, for error messages
}

// lexer turns the expression body into a slice of tokens. All scanning is
// single-pass and bounded by the input length; there are no unbounded loops.
type lexer struct {
	src string
	pos int
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// tokenize scans the whole input into tokens, returning an error on any
// malformed lexeme (e.g. unterminated string, stray character).
func tokenize(src string) ([]token, error) {
	l := &lexer{src: src}
	var toks []token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == tokEOF {
			return toks, nil
		}
	}
}

// next returns the next token, skipping insignificant whitespace.
func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) && isSpace(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, pos: l.pos}, nil
	}
	start := l.pos
	b := l.src[l.pos]
	switch {
	case b == '$':
		l.pos++
		return token{kind: tokDollar, val: "$", pos: start}, nil
	case b == '.':
		l.pos++
		return token{kind: tokDot, val: ".", pos: start}, nil
	case b == '[':
		l.pos++
		return token{kind: tokLBrack, val: "[", pos: start}, nil
	case b == ']':
		l.pos++
		return token{kind: tokRBrack, val: "]", pos: start}, nil
	case b == '|':
		l.pos++
		return token{kind: tokPipe, val: "|", pos: start}, nil
	case b == ':':
		l.pos++
		return token{kind: tokColon, val: ":", pos: start}, nil
	case b == '"':
		return l.scanString()
	case isDigit(b) || (b == '-' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1])):
		return l.scanNumber()
	case isIdentStart(b):
		return l.scanIdent()
	default:
		return token{}, fmt.Errorf("%w: unexpected character %q at position %d", ErrEval, string(b), start)
	}
}

// scanString reads a double-quoted string with \" and \\ escapes.
func (l *lexer) scanString() (token, error) {
	start := l.pos
	l.pos++ // consume opening quote
	var sb strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' {
			if l.pos+1 >= len(l.src) {
				break // trailing backslash -> unterminated
			}
			esc := l.src[l.pos+1]
			switch esc {
			case '"', '\\':
				sb.WriteByte(esc)
			default:
				sb.WriteByte('\\')
				sb.WriteByte(esc)
			}
			l.pos += 2
			continue
		}
		if c == '"' {
			l.pos++ // consume closing quote
			return token{kind: tokString, val: sb.String(), pos: start}, nil
		}
		sb.WriteByte(c)
		l.pos++
	}
	return token{}, fmt.Errorf("%w: unterminated string literal at position %d", ErrEval, start)
}

// scanNumber reads an integer or float literal (optionally negative).
func (l *lexer) scanNumber() (token, error) {
	start := l.pos
	if l.src[l.pos] == '-' {
		l.pos++
	}
	dots := 0
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if isDigit(c) {
			l.pos++
			continue
		}
		if c == '.' {
			dots++
			l.pos++
			continue
		}
		break
	}
	raw := l.src[start:l.pos]
	if dots > 1 {
		return token{}, fmt.Errorf("%w: malformed number literal %q at position %d", ErrEval, raw, start)
	}
	// The scan guarantees at least one digit and at most one dot, so the raw
	// text is always a valid float64 literal (a lone "-"/"." cannot reach here
	// because the caller only enters scanNumber on a digit or '-'+digit).
	f, _ := strconv.ParseFloat(raw, 64)
	return token{kind: tokNumber, val: raw, num: f, pos: start}, nil
}

// scanIdent reads an identifier: [A-Za-z_][A-Za-z0-9_]*
func (l *lexer) scanIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}
	return token{kind: tokIdent, val: l.src[start:l.pos], pos: start}, nil
}
