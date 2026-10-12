package cparse

import (
	"strings"
)

// Preprocessor evaluates #if/#define directives and emits the tokens of the
// active regions. Conditions that depend on the target platform
// (SDL_PLATFORM_*) activate every branch of the chain so that the generated
// API is the union over all platforms; symbols that do not exist on the host
// are resolved lazily at run time.
type Preprocessor struct {
	Env    *Env
	locked map[string]bool
}

// NewPreprocessor returns a preprocessor with the endianness macros that the
// SDL headers branch on predefined for a little-endian target.
func NewPreprocessor() *Preprocessor {
	pp := &Preprocessor{Env: &Env{Macros: map[string]*Macro{}, Syms: map[string]Value{}}, locked: map[string]bool{}}
	for name, val := range map[string]string{
		"SDL_LIL_ENDIAN":     "1234",
		"SDL_BIG_ENDIAN":     "4321",
		"SDL_BYTEORDER":      "1234",
		"SDL_FLOATWORDORDER": "1234",
	} {
		pp.Env.Macros[name] = &Macro{Name: name, Body: lexFragment(val, "<predef>", 0)}
		pp.locked[name] = true
	}
	return pp
}

type condFrame struct {
	parent bool // enclosing region active
	taken  bool // some branch of the chain already selected
	active bool
	union  bool // chain depends on the platform: every branch is active
}

type scanState struct {
	pp      *Preprocessor
	file    string
	src     string
	i, line int
	out     []Token
	conds   []condFrame
	lastDoc string // doc comment pending for the next directive
}

func (s *scanState) active() bool {
	return len(s.conds) == 0 || s.conds[len(s.conds)-1].active
}

// Process preprocesses one header and returns its active tokens.
func (pp *Preprocessor) Process(file, src string) []Token {
	s := &scanState{pp: pp, file: file, src: src, line: 1}
	s.run()
	return s.out
}

func (s *scanState) run() {
	src := s.src
	lineStart := true
	for s.i < len(src) {
		c := src[s.i]
		switch {
		case c == '\n':
			s.line++
			s.i++
			lineStart = true
		case c == ' ' || c == '\t' || c == '\r':
			s.i++
		case c == '/' && s.i+1 < len(src) && src[s.i+1] == '*':
			s.comment()
		case c == '/' && s.i+1 < len(src) && src[s.i+1] == '/':
			for s.i < len(src) && src[s.i] != '\n' {
				s.i++
			}
		case c == '#' && lineStart:
			s.directive()
			lineStart = true
		default:
			lineStart = false
			s.code()
		}
	}
}

func (s *scanState) comment() {
	src := s.src
	end := strings.Index(src[s.i+2:], "*/")
	var body string
	if end < 0 {
		body = src[s.i+2:]
		end = len(src) - s.i - 2
	} else {
		body = src[s.i+2 : s.i+2+end]
	}
	startLine := s.line
	s.line += strings.Count(body, "\n")
	s.i += end + 4
	if s.i > len(src) {
		s.i = len(src)
	}
	if !s.active() {
		return
	}
	switch {
	case strings.HasPrefix(body, "*<"), strings.HasPrefix(body, "!<"):
		s.out = append(s.out, Token{Post, cleanDoc(body[2:]), s.file, startLine})
	case strings.HasPrefix(body, "*") && len(body) > 1, strings.HasPrefix(body, "!"):
		d := cleanDoc(body[1:])
		s.out = append(s.out, Token{Doc, d, s.file, startLine})
		s.lastDoc = d
	}
}

// code lexes the rest of the current line up to a comment start.
func (s *scanState) code() {
	src := s.src
	j := s.i
	for j < len(src) && src[j] != '\n' {
		if src[j] == '"' || src[j] == '\'' {
			q := src[j]
			j++
			for j < len(src) && src[j] != q && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
		} else if src[j] == '/' && j+1 < len(src) && (src[j+1] == '*' || src[j+1] == '/') {
			break
		}
		j++
	}
	if j > len(src) {
		j = len(src)
	}
	frag := src[s.i:j]
	s.i = j
	if s.active() {
		s.out = append(s.out, lexFragment(frag, s.file, s.line)...)
		s.lastDoc = ""
	}
}

// directive consumes a preprocessor directive line.
func (s *scanState) directive() {
	src := s.src
	startLine := s.line
	s.i++ // '#'
	var b strings.Builder
	post := ""
	for s.i < len(src) {
		c := src[s.i]
		if c == '\\' && s.i+1 < len(src) && (src[s.i+1] == '\n' || src[s.i+1] == '\r' && s.i+2 < len(src) && src[s.i+2] == '\n') {
			if src[s.i+1] == '\r' {
				s.i++
			}
			s.i += 2
			s.line++
			b.WriteByte(' ')
			continue
		}
		if c == '\n' {
			break
		}
		if c == '/' && s.i+1 < len(src) && src[s.i+1] == '*' {
			end := strings.Index(src[s.i+2:], "*/")
			if end < 0 {
				s.i = len(src)
				break
			}
			body := src[s.i+2 : s.i+2+end]
			if strings.HasPrefix(body, "*<") {
				post = cleanDoc(body[2:])
			}
			s.line += strings.Count(body, "\n")
			s.i += end + 4
			b.WriteByte(' ')
			continue
		}
		if c == '/' && s.i+1 < len(src) && src[s.i+1] == '/' {
			for s.i < len(src) && src[s.i] != '\n' {
				s.i++
			}
			break
		}
		b.WriteByte(c)
		s.i++
	}
	text := strings.TrimSpace(b.String())
	name, rest := text, ""
	for k := 0; k < len(text); k++ {
		if !isIdentStart(text[k]) && !isDigit(text[k]) {
			name, rest = text[:k], strings.TrimSpace(text[k:])
			break
		}
	}
	toks := lexFragment(rest, s.file, startLine)
	pp := s.pp
	switch name {
	case "if", "ifdef", "ifndef":
		s.openCond(name, toks)
	case "elif":
		s.elifCond(toks)
	case "else":
		if n := len(s.conds); n > 0 {
			f := &s.conds[n-1]
			f.active = f.parent && (f.union || !f.taken)
			f.taken = true
		}
	case "endif":
		if n := len(s.conds); n > 0 {
			s.conds = s.conds[:n-1]
		}
	case "define":
		if s.active() {
			doc := s.lastDoc
			if doc == "" {
				doc = post
			}
			pp.define(toks, rest, doc, s.file, startLine)
		}
	case "undef":
		if s.active() && len(toks) > 0 && !pp.locked[toks[0].Text] {
			delete(pp.Env.Macros, toks[0].Text)
		}
	}
	s.lastDoc = ""
}

func (pp *Preprocessor) define(toks []Token, rest, doc, file string, line int) {
	if len(toks) == 0 || toks[0].Kind != Ident {
		return
	}
	name := toks[0].Text
	if pp.locked[name] {
		return
	}
	if _, dup := pp.Env.Macros[name]; dup {
		return // first definition wins: platform branches are unioned
	}
	m := &Macro{Name: name, Doc: doc, File: file, Line: line}
	body := toks[1:]
	// A function-like macro has '(' immediately after the name.
	if len(body) > 0 && body[0].Text == "(" && strings.HasPrefix(rest, name+"(") {
		m.FuncLike = true
		k := 1
		for k < len(body) && body[k].Text != ")" {
			if body[k].Kind == Ident {
				m.Params = append(m.Params, body[k].Text)
			}
			k++
		}
		if k < len(body) {
			body = body[k+1:]
		}
	}
	m.Body = body
	pp.Env.Macros[name] = m
}

func isPlatformy(toks []Token) bool {
	for _, t := range toks {
		if t.Kind == Ident && strings.HasPrefix(t.Text, "SDL_PLATFORM_") {
			return true
		}
	}
	return false
}

func containsIdent(toks []Token, name string) bool {
	for _, t := range toks {
		if t.Kind == Ident && t.Text == name {
			return true
		}
	}
	return false
}

func (s *scanState) openCond(kind string, toks []Token) {
	parent := s.active()
	f := condFrame{parent: parent}
	if parent {
		switch {
		case containsIdent(toks, "SDL_WIKI_DOCUMENTATION_SECTION"):
			// documentation-only declarations are not the real ABI
			f.active = kind == "ifndef"
		case isPlatformy(toks):
			f.active, f.union = true, true
		case kind == "ifdef":
			f.active = s.defined(toks)
		case kind == "ifndef":
			f.active = !s.defined(toks)
		default:
			f.active = s.evalCond(toks)
		}
	}
	f.taken = f.active
	s.conds = append(s.conds, f)
}

func (s *scanState) defined(toks []Token) bool {
	if len(toks) == 0 {
		return false
	}
	_, ok := s.pp.Env.Macros[toks[0].Text]
	return ok
}

func (s *scanState) evalCond(toks []Token) bool {
	env := *s.pp.Env
	env.CondMode = true
	v, err := env.Eval(toks)
	return err == nil && v.truth()
}

func (s *scanState) elifCond(toks []Token) {
	n := len(s.conds)
	if n == 0 {
		return
	}
	f := &s.conds[n-1]
	switch {
	case !f.parent:
		f.active = false
	case isPlatformy(toks):
		f.active, f.union = true, true
	case f.union:
		f.active = true
	case f.taken:
		f.active = false
	default:
		f.active = s.evalCond(toks)
	}
	f.taken = f.taken || f.active
}
