// Package billingexpr implements the billing expression engine for tiered /
// dynamic per-model pricing. The design constraints (pure, deterministic,
// length/depth/step limits, function whitelist) are documented in expr.md
// and enforced by this package.
package billingexpr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const (
	// MaxExprLength bounds a single expression (see expr.md).
	MaxExprLength = 512
	// MaxExprDepth bounds parser recursion.
	MaxExprDepth = 64
	// MaxExprSteps bounds node visits per evaluation.
	MaxExprSteps = 10000
)

// Validate parses an expression without evaluating it and checks the call
// nodes against the function whitelist. It is the gate used when an admin
// stores a new pricing expression: anything rejected here never reaches the
// request path.
func Validate(expr string) error {
	root, err := parse(expr)
	if err != nil {
		return err
	}
	return walkCalls(root)
}

func walkCalls(n *node) error {
	if n == nil {
		return nil
	}
	if n.kind == ndCall {
		if !allowedFunc(n.ident) {
			return fmt.Errorf("function %q is not allowed", n.ident)
		}
		switch n.ident {
		case "min", "max":
			if len(n.args) < 1 {
				return fmt.Errorf("%s() needs at least one argument", n.ident)
			}
		default:
			if len(n.args) != 1 {
				return fmt.Errorf("%s() needs exactly one argument", n.ident)
			}
		}
	}
	for _, a := range n.args {
		if err := walkCalls(a); err != nil {
			return err
		}
	}
	return nil
}

// Eval parses and evaluates the expression against the given variables.
func Eval(expr string, vars map[string]float64) (float64, error) {
	root, err := parse(expr)
	if err != nil {
		return 0, err
	}
	ev := &evaluator{vars: vars}
	v, err := ev.eval(root)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// ---------------------------------------------------------------------------
// AST
// ---------------------------------------------------------------------------

type nodeKind int

const (
	ndNumber nodeKind = iota
	ndVar
	ndUnary
	ndBinary
	ndTernary
	ndCall
)

type node struct {
	kind  nodeKind
	num   float64
	ident string
	op    string
	args  []*node // unary: 1, binary: 2, ternary: 3, call: n
}

// ---------------------------------------------------------------------------
// Lexer
// ---------------------------------------------------------------------------

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkNumber
	tkIdent
	tkOp // operators and punctuation
)

type token struct {
	kind tokenKind
	text string
	num  float64
}

func lex(expr string) ([]token, error) {
	var tokens []token
	rs := []rune(expr)
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			i++
		case unicode.IsDigit(r) || r == '.':
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			text := string(rs[i:j])
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q at offset %d", text, i)
			}
			tokens = append(tokens, token{kind: tkNumber, text: text, num: v})
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			tokens = append(tokens, token{kind: tkIdent, text: string(rs[i:j])})
			i = j
		case strings.ContainsRune("+-*/%<>=!?:(),&|", r):
			// two-character operators first
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "<=", ">=", "==", "!=", "&&", "||":
				tokens = append(tokens, token{kind: tkOp, text: two})
				i += 2
			default:
				tokens = append(tokens, token{kind: tkOp, text: string(r)})
				i++
			}
		default:
			return nil, fmt.Errorf("unexpected character %q at offset %d", r, i)
		}
	}
	tokens = append(tokens, token{kind: tkEOF})
	return tokens, nil
}

// ---------------------------------------------------------------------------
// Parser (recursive descent, depth-limited)
// ---------------------------------------------------------------------------

type parser struct {
	tokens []token
	pos    int
	depth  int
}

func parse(expr string) (*node, error) {
	if len(expr) > MaxExprLength {
		return nil, fmt.Errorf("expression exceeds %d characters", MaxExprLength)
	}
	tokens, err := lex(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	n, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tkEOF {
		return nil, fmt.Errorf("unexpected trailing input %q", p.peek().text)
	}
	return n, nil
}

func (p *parser) peek() token { return p.tokens[p.pos] }

func (p *parser) next() token {
	t := p.tokens[p.pos]
	if t.kind != tkEOF {
		p.pos++
	}
	return t
}

func (p *parser) acceptOp(ops ...string) (string, bool) {
	t := p.peek()
	if t.kind != tkOp {
		return "", false
	}
	for _, op := range ops {
		if t.text == op {
			p.pos++
			return op, true
		}
	}
	return "", false
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > MaxExprDepth {
		return fmt.Errorf("expression nesting exceeds depth %d", MaxExprDepth)
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

func (p *parser) parseTernary() (*node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if _, ok := p.acceptOp("?"); !ok {
		return cond, nil
	}
	yes, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if _, ok := p.acceptOp(":"); !ok {
		return nil, fmt.Errorf("expected ':' in ternary expression")
	}
	no, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	return &node{kind: ndTernary, args: []*node{cond, yes, no}}, nil
}

func (p *parser) parseOr() (*node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("||")
		if !ok {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &node{kind: ndBinary, op: op, args: []*node{left, right}}
	}
}

func (p *parser) parseAnd() (*node, error) {
	left, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("&&")
		if !ok {
			return left, nil
		}
		right, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		left = &node{kind: ndBinary, op: op, args: []*node{left, right}}
	}
}

func (p *parser) parseCmp() (*node, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("<", ">", "<=", ">=", "==", "!=")
		if !ok {
			return left, nil
		}
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		left = &node{kind: ndBinary, op: op, args: []*node{left, right}}
	}
}

func (p *parser) parseAdd() (*node, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("+", "-")
		if !ok {
			return left, nil
		}
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = &node{kind: ndBinary, op: op, args: []*node{left, right}}
	}
}

func (p *parser) parseMul() (*node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("*", "/", "%")
		if !ok {
			return left, nil
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &node{kind: ndBinary, op: op, args: []*node{left, right}}
	}
}

func (p *parser) parseUnary() (*node, error) {
	if op, ok := p.acceptOp("-", "!"); ok {
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &node{kind: ndUnary, op: op, args: []*node{operand}}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	t := p.next()
	switch t.kind {
	case tkNumber:
		return &node{kind: ndNumber, num: t.num}, nil
	case tkIdent:
		if _, ok := p.acceptOp("("); ok {
			call := &node{kind: ndCall, ident: t.text}
			if _, isClose := p.acceptOp(")"); !isClose {
				for {
					arg, err := p.parseTernary()
					if err != nil {
						return nil, err
					}
					call.args = append(call.args, arg)
					if _, done := p.acceptOp(","); !done {
						break
					}
				}
				if _, ok := p.acceptOp(")"); !ok {
					return nil, fmt.Errorf("expected ')' after arguments of %s()", t.text)
				}
			}
			return call, nil
		}
		return &node{kind: ndVar, ident: t.text}, nil
	case tkOp:
		if t.text != "(" {
			return nil, fmt.Errorf("unexpected %q", t.text)
		}
		inner, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		if _, ok := p.acceptOp(")"); !ok {
			return nil, fmt.Errorf("expected ')'")
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("unexpected end of expression")
	}
}

// ---------------------------------------------------------------------------
// Evaluator (whitelist + step budget)
// ---------------------------------------------------------------------------

type evaluator struct {
	vars  map[string]float64
	steps int
}

func (e *evaluator) eval(n *node) (float64, error) {
	e.steps++
	if e.steps > MaxExprSteps {
		return 0, fmt.Errorf("expression evaluation exceeds %d steps", MaxExprSteps)
	}
	switch n.kind {
	case ndNumber:
		return n.num, nil
	case ndVar:
		switch n.ident {
		case "true":
			return 1, nil
		case "false":
			return 0, nil
		}
		v, ok := e.vars[n.ident]
		if !ok {
			return 0, fmt.Errorf("unknown variable %q", n.ident)
		}
		return v, nil
	case ndUnary:
		v, err := e.eval(n.args[0])
		if err != nil {
			return 0, err
		}
		if n.op == "-" {
			return -v, nil
		}
		return boolToFloat(v == 0), nil
	case ndBinary:
		left, err := e.eval(n.args[0])
		if err != nil {
			return 0, err
		}
		right, err := e.eval(n.args[1])
		if err != nil {
			return 0, err
		}
		return e.applyBinary(n.op, left, right)
	case ndTernary:
		cond, err := e.eval(n.args[0])
		if err != nil {
			return 0, err
		}
		if cond != 0 {
			return e.eval(n.args[1])
		}
		return e.eval(n.args[2])
	case ndCall:
		return e.evalCall(n)
	default:
		return 0, fmt.Errorf("unknown node kind %d", n.kind)
	}
}

func (e *evaluator) applyBinary(op string, left, right float64) (float64, error) {
	switch op {
	case "+":
		return left + right, nil
	case "-":
		return left - right, nil
	case "*":
		return left * right, nil
	case "/":
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	case "%":
		if right == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return math.Mod(left, right), nil
	case "<":
		return boolToFloat(left < right), nil
	case ">":
		return boolToFloat(left > right), nil
	case "<=":
		return boolToFloat(left <= right), nil
	case ">=":
		return boolToFloat(left >= right), nil
	case "==":
		return boolToFloat(left == right), nil
	case "!=":
		return boolToFloat(left != right), nil
	case "&&":
		return boolToFloat(left != 0 && right != 0), nil
	case "||":
		return boolToFloat(left != 0 || right != 0), nil
	default:
		return 0, fmt.Errorf("unknown operator %q", op)
	}
}

func (e *evaluator) evalCall(n *node) (float64, error) {
	if !allowedFunc(n.ident) {
		return 0, fmt.Errorf("function %q is not allowed", n.ident)
	}
	args := make([]float64, 0, len(n.args))
	for _, a := range n.args {
		v, err := e.eval(a)
		if err != nil {
			return 0, err
		}
		args = append(args, v)
	}
	switch n.ident {
	case "min":
		if len(args) < 1 {
			return 0, fmt.Errorf("min() needs at least one argument")
		}
		v := args[0]
		for _, a := range args[1:] {
			v = math.Min(v, a)
		}
		return v, nil
	case "max":
		if len(args) < 1 {
			return 0, fmt.Errorf("max() needs at least one argument")
		}
		v := args[0]
		for _, a := range args[1:] {
			v = math.Max(v, a)
		}
		return v, nil
	case "floor":
		if len(args) != 1 {
			return 0, fmt.Errorf("floor() needs exactly one argument")
		}
		return math.Floor(args[0]), nil
	case "ceil":
		if len(args) != 1 {
			return 0, fmt.Errorf("ceil() needs exactly one argument")
		}
		return math.Ceil(args[0]), nil
	case "round":
		if len(args) != 1 {
			return 0, fmt.Errorf("round() needs exactly one argument")
		}
		return math.Round(args[0]), nil
	case "abs":
		if len(args) != 1 {
			return 0, fmt.Errorf("abs() needs exactly one argument")
		}
		return math.Abs(args[0]), nil
	}
	return 0, fmt.Errorf("function %q is not allowed", n.ident)
}

var funcWhitelist = map[string]bool{
	"min": true, "max": true, "floor": true, "ceil": true, "round": true, "abs": true,
}

func allowedFunc(name string) bool { return funcWhitelist[name] }

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
