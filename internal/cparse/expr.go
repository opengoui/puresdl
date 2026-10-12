package cparse

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// VKind is the dynamic type of an evaluated constant.
type VKind int

const (
	VInt VKind = iota
	VFloat
	VString
)

// Value is the result of evaluating a constant expression.
type Value struct {
	Kind  VKind
	Int   *big.Int
	Float float64
	Str   string
}

func intVal(i *big.Int) Value { return Value{Kind: VInt, Int: i} }
func intVal64(i int64) Value  { return intVal(big.NewInt(i)) }

func (v Value) truth() bool {
	switch v.Kind {
	case VInt:
		return v.Int.Sign() != 0
	case VFloat:
		return v.Float != 0
	}
	return v.Str != ""
}

func (v Value) float() float64 {
	if v.Kind == VFloat {
		return v.Float
	}
	f, _ := new(big.Float).SetInt(v.Int).Float64()
	return f
}

// Macro is a recorded #define.
type Macro struct {
	Name     string
	FuncLike bool
	Params   []string
	Body     []Token
	Doc      string
	File     string
	Line     int
}

// CastInfo describes an integer type for C cast evaluation.
type CastInfo struct {
	Bits   int
	Signed bool
}

// Env supplies symbols to the constant expression evaluator.
type Env struct {
	Macros map[string]*Macro
	// Syms holds enumerator values by C name.
	Syms map[string]Value
	// CastType resolves a type name to its integer width, if known.
	CastType func(name string) (CastInfo, bool)
	// CondMode makes unknown identifiers evaluate to 0, as in #if.
	CondMode bool
}

var builtinCasts = map[string]CastInfo{
	"int": {32, true}, "unsigned": {32, false}, "long": {64, true}, "char": {8, true},
	"short": {16, true}, "size_t": {64, false}, "float": {0, true}, "double": {0, true},
}

type exprParser struct {
	env   *Env
	toks  []Token
	pos   int
	depth int
}

// Eval evaluates a constant expression made of tokens.
func (e *Env) Eval(toks []Token) (v Value, err error) {
	return e.eval(toks, 0)
}

func (e *Env) eval(toks []Token, depth int) (v Value, err error) {
	if depth > 40 {
		return v, fmt.Errorf("macro recursion too deep")
	}
	p := &exprParser{env: e, toks: toks, depth: depth}
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(evalError); ok {
				err = pe
				return
			}
			panic(r)
		}
	}()
	v = p.ternary()
	if p.pos != len(p.toks) {
		p.fail("unexpected %q", p.toks[p.pos].Text)
	}
	return v, nil
}

type evalError string

func (e evalError) Error() string { return string(e) }

func (p *exprParser) fail(format string, a ...any) {
	panic(evalError(fmt.Sprintf(format, a...)))
}

func (p *exprParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos].Text
	}
	return ""
}

func (p *exprParser) accept(s string) bool {
	if p.pos < len(p.toks) && p.toks[p.pos].Kind == Punct && p.toks[p.pos].Text == s {
		p.pos++
		return true
	}
	return false
}

func (p *exprParser) ternary() Value {
	c := p.binary(0)
	if p.accept("?") {
		a := p.ternary()
		if !p.accept(":") {
			p.fail("expected ':'")
		}
		b := p.ternary()
		if c.truth() {
			return a
		}
		return b
	}
	return c
}

var binPrec = [][]string{
	{"||"}, {"&&"}, {"|"}, {"^"}, {"&"}, {"==", "!="}, {"<", ">", "<=", ">="}, {"<<", ">>"}, {"+", "-"}, {"*", "/", "%"},
}

func (p *exprParser) binary(level int) Value {
	if level == len(binPrec) {
		return p.unary()
	}
	l := p.binary(level + 1)
	for {
		op := ""
		for _, o := range binPrec[level] {
			if p.pos < len(p.toks) && p.toks[p.pos].Kind == Punct && p.toks[p.pos].Text == o {
				op = o
			}
		}
		if op == "" {
			return l
		}
		p.pos++
		r := p.binary(level + 1)
		l = p.apply(op, l, r)
	}
}

func b2v(b bool) Value {
	if b {
		return intVal64(1)
	}
	return intVal64(0)
}

func (p *exprParser) apply(op string, l, r Value) Value {
	switch op {
	case "||":
		return b2v(l.truth() || r.truth())
	case "&&":
		return b2v(l.truth() && r.truth())
	}
	if l.Kind == VString || r.Kind == VString {
		if l.Kind == r.Kind && (op == "==" || op == "!=") {
			return b2v((l.Str == r.Str) == (op == "=="))
		}
		p.fail("operator %s on string", op)
	}
	if l.Kind == VFloat || r.Kind == VFloat {
		a, b := l.float(), r.float()
		switch op {
		case "+":
			return Value{Kind: VFloat, Float: a + b}
		case "-":
			return Value{Kind: VFloat, Float: a - b}
		case "*":
			return Value{Kind: VFloat, Float: a * b}
		case "/":
			return Value{Kind: VFloat, Float: a / b}
		case "==":
			return b2v(a == b)
		case "!=":
			return b2v(a != b)
		case "<":
			return b2v(a < b)
		case ">":
			return b2v(a > b)
		case "<=":
			return b2v(a <= b)
		case ">=":
			return b2v(a >= b)
		}
		p.fail("operator %s on float", op)
	}
	a, b := l.Int, r.Int
	z := new(big.Int)
	switch op {
	case "|":
		return intVal(z.Or(a, b))
	case "^":
		return intVal(z.Xor(a, b))
	case "&":
		return intVal(z.And(a, b))
	case "==":
		return b2v(a.Cmp(b) == 0)
	case "!=":
		return b2v(a.Cmp(b) != 0)
	case "<":
		return b2v(a.Cmp(b) < 0)
	case ">":
		return b2v(a.Cmp(b) > 0)
	case "<=":
		return b2v(a.Cmp(b) <= 0)
	case ">=":
		return b2v(a.Cmp(b) >= 0)
	case "<<":
		return intVal(z.Lsh(a, uint(b.Int64())))
	case ">>":
		return intVal(z.Rsh(a, uint(b.Int64())))
	case "+":
		return intVal(z.Add(a, b))
	case "-":
		return intVal(z.Sub(a, b))
	case "*":
		return intVal(z.Mul(a, b))
	case "/", "%":
		if b.Sign() == 0 {
			p.fail("division by zero")
		}
		if op == "/" {
			return intVal(z.Quo(a, b))
		}
		return intVal(z.Rem(a, b))
	}
	p.fail("bad operator %s", op)
	return Value{}
}

func (p *exprParser) castInfo(name string) (CastInfo, bool) {
	if ci, ok := builtinCasts[name]; ok {
		return ci, true
	}
	if p.env.CastType != nil {
		return p.env.CastType(name)
	}
	return CastInfo{}, false
}

func (p *exprParser) unary() Value {
	switch {
	case p.accept("-"):
		v := p.unary()
		if v.Kind == VFloat {
			return Value{Kind: VFloat, Float: -v.Float}
		}
		if v.Kind != VInt {
			p.fail("unary - on string")
		}
		return intVal(new(big.Int).Neg(v.Int))
	case p.accept("+"):
		return p.unary()
	case p.accept("~"):
		v := p.unary()
		if v.Kind != VInt {
			p.fail("~ on non-int")
		}
		return intVal(new(big.Int).Not(v.Int))
	case p.accept("!"):
		return b2v(!p.unary().truth())
	}
	// cast: ( type ) operand
	if p.peek() == "(" && p.pos+2 < len(p.toks) && p.toks[p.pos+1].Kind == Ident && p.toks[p.pos+2].Text == ")" {
		name := p.toks[p.pos+1].Text
		if ci, ok := p.castInfo(name); ok {
			p.pos += 3
			return p.cast(ci, p.unary())
		}
	}
	// cast with two keywords, e.g. (unsigned int)
	if p.peek() == "(" && p.pos+3 < len(p.toks) && p.toks[p.pos+1].Kind == Ident && p.toks[p.pos+2].Kind == Ident && p.toks[p.pos+3].Text == ")" {
		if _, ok := p.castInfo(p.toks[p.pos+2].Text); ok {
			ci, _ := p.castInfo(p.toks[p.pos+1].Text)
			if p.toks[p.pos+1].Text == "unsigned" {
				ci.Signed = false
			}
			if _, ok2 := p.castInfo(p.toks[p.pos+1].Text); ok2 {
				p.pos += 4
				return p.cast(ci, p.unary())
			}
		}
	}
	return p.primary()
}

func (p *exprParser) cast(ci CastInfo, v Value) Value {
	if ci.Bits == 0 { // float/double
		return Value{Kind: VFloat, Float: v.float()}
	}
	if v.Kind == VFloat {
		i, _ := new(big.Float).SetFloat64(v.Float).Int(nil)
		v = intVal(i)
	}
	if v.Kind != VInt {
		p.fail("cast of string")
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(ci.Bits))
	r := new(big.Int).Mod(v.Int, mod)
	if ci.Signed && r.Cmp(new(big.Int).Rsh(mod, 1)) >= 0 {
		r.Sub(r, mod)
	}
	return intVal(r)
}

func parseNumber(s string) (Value, error) {
	ls := strings.ToLower(s)
	isHex := strings.HasPrefix(ls, "0x")
	if !isHex && (strings.ContainsAny(ls, ".") || strings.Contains(ls, "e")) {
		t := strings.TrimRight(ls, "fl")
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: VFloat, Float: f}, nil
	}
	t := strings.TrimRight(ls, "ul")
	if !isHex && strings.HasSuffix(t, "f") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(t, "f"), 64)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: VFloat, Float: f}, nil
	}
	i := new(big.Int)
	base := 0
	if len(t) > 1 && t[0] == '0' && !isHex && !strings.HasPrefix(t, "0b") {
		base = 8
	}
	if _, ok := i.SetString(t, base); !ok {
		return Value{}, fmt.Errorf("bad number %q", s)
	}
	return intVal(i), nil
}

func unquote(tok string) (string, error) {
	return strconv.Unquote(tok)
}

func (p *exprParser) primary() Value {
	if p.pos >= len(p.toks) {
		p.fail("unexpected end of expression")
	}
	t := p.toks[p.pos]
	switch t.Kind {
	case Number:
		p.pos++
		v, err := parseNumber(t.Text)
		if err != nil {
			p.fail("%v", err)
		}
		return v
	case Char:
		p.pos++
		s, err := strconv.Unquote(t.Text)
		if err != nil || len(s) == 0 {
			p.fail("bad char literal %s", t.Text)
		}
		return intVal64(int64(s[0]))
	case String:
		var sb strings.Builder
		for p.pos < len(p.toks) && p.toks[p.pos].Kind == String {
			s, err := unquote(p.toks[p.pos].Text)
			if err != nil {
				p.fail("bad string %s", p.toks[p.pos].Text)
			}
			sb.WriteString(s)
			p.pos++
		}
		return Value{Kind: VString, Str: sb.String()}
	case Punct:
		if t.Text == "(" {
			p.pos++
			v := p.ternary()
			if !p.accept(")") {
				p.fail("expected ')'")
			}
			return v
		}
		p.fail("unexpected %q", t.Text)
	case Ident:
		p.pos++
		return p.ident(t)
	}
	p.fail("unexpected token %v", t)
	return Value{}
}

func (p *exprParser) ident(t Token) Value {
	name := t.Text
	if name == "defined" {
		paren := p.accept("(")
		if p.pos >= len(p.toks) {
			p.fail("bad defined")
		}
		n := p.toks[p.pos].Text
		p.pos++
		if paren && !p.accept(")") {
			p.fail("bad defined")
		}
		_, ok := p.env.Macros[n]
		return b2v(ok)
	}
	switch name {
	case "SDL_UINT64_C", "SDL_SINT64_C", "SDL_UINT32_C", "SDL_SINT32_C":
		return p.callBuiltinIdentity()
	case "true":
		return intVal64(1)
	case "false":
		return intVal64(0)
	}
	if m, ok := p.env.Macros[name]; ok {
		if !m.FuncLike {
			v, err := p.env.eval(m.Body, p.depth+1)
			if err != nil {
				p.fail("%s: %v", name, err)
			}
			return v
		}
		if p.peek() == "(" {
			args := p.collectArgs()
			return p.expand(m, args)
		}
	}
	if v, ok := p.env.Syms[name]; ok {
		return v
	}
	if p.env.CondMode {
		return intVal64(0)
	}
	p.fail("unknown identifier %s", name)
	return Value{}
}

func (p *exprParser) callBuiltinIdentity() Value {
	if !p.accept("(") {
		p.fail("expected '('")
	}
	v := p.ternary()
	if !p.accept(")") {
		p.fail("expected ')'")
	}
	return v
}

func (p *exprParser) collectArgs() [][]Token {
	p.pos++ // (
	var args [][]Token
	var cur []Token
	depth := 0
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		p.pos++
		if t.Kind == Punct {
			switch t.Text {
			case "(":
				depth++
			case ")":
				if depth == 0 {
					if len(cur) > 0 || len(args) > 0 {
						args = append(args, cur)
					}
					return args
				}
				depth--
			case ",":
				if depth == 0 {
					args = append(args, cur)
					cur = nil
					continue
				}
			}
		}
		cur = append(cur, t)
	}
	p.fail("unterminated macro call")
	return nil
}

func (p *exprParser) expand(m *Macro, args [][]Token) Value {
	if len(args) != len(m.Params) {
		p.fail("macro %s: want %d args, got %d", m.Name, len(m.Params), len(args))
	}
	var body []Token
	for _, bt := range m.Body {
		if bt.Kind == Ident {
			replaced := false
			for i, pn := range m.Params {
				if pn == bt.Text {
					body = append(body, Token{Kind: Punct, Text: "("})
					body = append(body, args[i]...)
					body = append(body, Token{Kind: Punct, Text: ")"})
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
		}
		body = append(body, bt)
	}
	v, err := p.env.eval(body, p.depth+1)
	if err != nil {
		p.fail("%s: %v", m.Name, err)
	}
	return v
}
