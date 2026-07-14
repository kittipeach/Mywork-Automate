package expression

import (
	"fmt"
)

// AST node types.

// exprNode is the parsed expression: a primary value followed by zero or more
// pipe stages.
type exprNode struct {
	primary primaryNode
	pipes   []pipeNode
}

// primaryNode is either a reference ($...) or a literal.
type primaryNode struct {
	isRef   bool
	ref     refNode // valid when isRef
	literal any     // string or float64 when !isRef
}

// refNode is a rooted reference with a chain of accessors.
type refNode struct {
	root      string // "node", "flow", "vars"
	nodeName  string // for root == "node": the selected node name
	accessors []accessor
}

// accessor is a single ".name" or "[key]" step.
type accessor struct {
	isIndex bool // true for [key], false for .name
	name    string
	// index key when isIndex: exactly one of keyStr/keyInt is meaningful.
	keyStr   string
	keyInt   int
	keyIsInt bool
}

// pipeNode is one pipe stage: a function name and its literal arguments.
type pipeNode struct {
	fn   string
	args []any // string or float64 literals
}

// parser is a hand-written recursive-descent parser over the token slice.
type parser struct {
	toks []token
	pos  int
}

// parse tokenizes and parses a full expression body.
func parse(body string) (exprNode, error) {
	toks, err := tokenize(body)
	if err != nil {
		return exprNode{}, err
	}
	p := &parser{toks: toks}
	// Reject an entirely empty body (only EOF token).
	if p.peek().kind == tokEOF {
		return exprNode{}, fmt.Errorf("%w: empty expression", ErrEval)
	}
	node, err := p.parseExpr()
	if err != nil {
		return exprNode{}, err
	}
	if p.peek().kind != tokEOF {
		t := p.peek()
		return exprNode{}, fmt.Errorf("%w: unexpected token %q at position %d", ErrEval, tokenDesc(t), t.pos)
	}
	return node, nil
}

func (p *parser) peek() token { return p.toks[p.pos] }

func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// parseExpr := primary ( '|' pipe )*
func (p *parser) parseExpr() (exprNode, error) {
	prim, err := p.parsePrimary()
	if err != nil {
		return exprNode{}, err
	}
	node := exprNode{primary: prim}
	for p.peek().kind == tokPipe {
		p.advance() // consume '|'
		pipe, err := p.parsePipe()
		if err != nil {
			return exprNode{}, err
		}
		node.pipes = append(node.pipes, pipe)
	}
	return node, nil
}

// parsePrimary := reference | stringLit | numberLit
func (p *parser) parsePrimary() (primaryNode, error) {
	t := p.peek()
	switch t.kind {
	case tokDollar:
		ref, err := p.parseRef()
		if err != nil {
			return primaryNode{}, err
		}
		return primaryNode{isRef: true, ref: ref}, nil
	case tokString:
		p.advance()
		return primaryNode{literal: t.val}, nil
	case tokNumber:
		p.advance()
		return primaryNode{literal: t.num}, nil
	default:
		return primaryNode{}, fmt.Errorf("%w: unexpected token %q at position %d, expected a value", ErrEval, tokenDesc(t), t.pos)
	}
}

// parseRef := '$' ident accessors
func (p *parser) parseRef() (refNode, error) {
	p.advance() // consume '$'
	head := p.peek()
	if head.kind != tokIdent {
		return refNode{}, fmt.Errorf("%w: expected reference name after '$' at position %d", ErrEval, head.pos)
	}
	p.advance()

	switch head.val {
	case "node":
		return p.parseNodeRef()
	case "flow", "vars":
		ref := refNode{root: head.val}
		acc, err := p.parseAccessors()
		if err != nil {
			return refNode{}, err
		}
		ref.accessors = acc
		return ref, nil
	default:
		return refNode{}, fmt.Errorf("%w: unknown reference root %q at position %d", ErrEval, "$"+head.val, head.pos)
	}
}

// parseNodeRef := '$node' '[' string ']' accessors
func (p *parser) parseNodeRef() (refNode, error) {
	if p.peek().kind != tokLBrack {
		return refNode{}, fmt.Errorf("%w: expected '[' after $node at position %d", ErrEval, p.peek().pos)
	}
	p.advance() // '['
	if p.peek().kind != tokString {
		return refNode{}, fmt.Errorf("%w: expected node name string in $node[...] at position %d", ErrEval, p.peek().pos)
	}
	name := p.advance().val
	if p.peek().kind != tokRBrack {
		return refNode{}, fmt.Errorf("%w: expected ']' to close $node[...] at position %d", ErrEval, p.peek().pos)
	}
	p.advance() // ']'
	ref := refNode{root: "node", nodeName: name}
	acc, err := p.parseAccessors()
	if err != nil {
		return refNode{}, err
	}
	ref.accessors = acc
	return ref, nil
}

// parseAccessors := ( '.' ident | '[' (string|number) ']' )*
func (p *parser) parseAccessors() ([]accessor, error) {
	var accs []accessor
	for {
		switch p.peek().kind {
		case tokDot:
			p.advance()
			if p.peek().kind != tokIdent {
				return nil, fmt.Errorf("%w: expected field name after '.' at position %d", ErrEval, p.peek().pos)
			}
			accs = append(accs, accessor{name: p.advance().val})
		case tokLBrack:
			p.advance()
			acc, err := p.parseIndex()
			if err != nil {
				return nil, err
			}
			accs = append(accs, acc)
		default:
			return accs, nil
		}
	}
}

// parseIndex parses the inside of an accessor '[' ... ']'.
func (p *parser) parseIndex() (accessor, error) {
	t := p.peek()
	var acc accessor
	acc.isIndex = true
	switch t.kind {
	case tokString:
		p.advance()
		acc.keyStr = t.val
	case tokNumber:
		p.advance()
		if t.num != float64(int(t.num)) {
			return accessor{}, fmt.Errorf("%w: array index %q must be an integer at position %d", ErrEval, t.val, t.pos)
		}
		acc.keyInt = int(t.num)
		acc.keyIsInt = true
	default:
		return accessor{}, fmt.Errorf("%w: expected string or integer index at position %d", ErrEval, t.pos)
	}
	if p.peek().kind != tokRBrack {
		return accessor{}, fmt.Errorf("%w: expected ']' to close index at position %d", ErrEval, p.peek().pos)
	}
	p.advance() // ']'
	return acc, nil
}

// parsePipe := ident ( ':' arg )*
func (p *parser) parsePipe() (pipeNode, error) {
	if p.peek().kind != tokIdent {
		return pipeNode{}, fmt.Errorf("%w: expected function name after '|' at position %d", ErrEval, p.peek().pos)
	}
	pn := pipeNode{fn: p.advance().val}
	for p.peek().kind == tokColon {
		p.advance() // ':'
		arg, err := p.parsePipeArg()
		if err != nil {
			return pipeNode{}, err
		}
		pn.args = append(pn.args, arg)
	}
	return pn, nil
}

// parsePipeArg parses one literal argument (string or number).
func (p *parser) parsePipeArg() (any, error) {
	t := p.peek()
	switch t.kind {
	case tokString:
		p.advance()
		return t.val, nil
	case tokNumber:
		p.advance()
		return t.num, nil
	default:
		return nil, fmt.Errorf("%w: expected string or number argument after ':' at position %d", ErrEval, t.pos)
	}
}

// tokenDesc renders a token for error messages. Punctuation, ident and number
// tokens carry their literal in val; strings are re-quoted for clarity.
func tokenDesc(t token) string {
	if t.kind == tokString {
		return `"` + t.val + `"`
	}
	return t.val
}
