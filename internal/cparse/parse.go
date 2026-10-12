package cparse

import (
	"fmt"
	"strings"
)

var stripIdents = setOf(
	"SDL_DECLSPEC", "SDLCALL", "SDL_MALLOC", "SDL_ANALYZER_NORETURN", "SDL_PRINTF_FORMAT_STRING",
	"SDL_SCANF_FORMAT_STRING", "SDL_FORCE_INLINE", "SDL_INLINE", "extern", "static", "inline",
	"SDL_NORETURN", "SDL_DEPRECATED", "SDL_UNUSED", "SDL_NODISCARD", "volatile", "register", "restrict",
	"SDL_NOTHROW", "SDLMAIN_DECLSPEC", "SDL_VOLATILE",
)

var stripCalls = setOf(
	"SDL_ALLOC_SIZE", "SDL_ALLOC_SIZE2", "SDL_OUT_Z_CAP", "SDL_OUT_CAP", "SDL_OUT_BYTECAP", "SDL_IN_BYTECAP",
	"SDL_INOUT_Z_CAP", "SDL_OUT_Z_BYTECAP", "SDL_PRINTF_VARARG_FUNC", "SDL_PRINTF_VARARG_FUNCV",
	"SDL_WPRINTF_VARARG_FUNC", "SDL_WPRINTF_VARARG_FUNCV", "SDL_SCANF_VARARG_FUNC", "SDL_SCANF_VARARG_FUNCV",
	"SDL_ACQUIRE", "SDL_RELEASE", "SDL_TRY_ACQUIRE", "SDL_ACQUIRE_SHARED", "SDL_TRY_ACQUIRE_SHARED",
	"SDL_RELEASE_SHARED", "SDL_RELEASE_GENERIC", "SDL_ELF_NOTE_DLOPEN",
)

var builtinWords = setOf("unsigned", "signed", "int", "char", "short", "long", "float", "double", "void")

func setOf(s ...string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

// Parser accumulates declarations from preprocessed headers.
type Parser struct {
	Unit Unit
	file string
}

// ParseTokens parses the preprocessed tokens of one header.
func (p *Parser) ParseTokens(file string, toks []Token) {
	p.file = file
	p.Unit.Files = append(p.Unit.Files, file)
	var stmt []Token
	var doc string
	paren, brace := 0, 0
	var openPrev Token // token preceding the outermost '{'
	flush := func(body bool) {
		if len(stmt) > 0 {
			p.statement(stmt, doc, body)
		}
		stmt, doc = nil, ""
	}
	for _, t := range toks {
		if t.Kind == Doc || t.Kind == Post {
			if brace == 0 && paren == 0 {
				if t.Kind == Doc && len(stmt) == 0 && !ignoredDoc(t.Text) {
					doc = t.Text
				}
				continue
			}
			stmt = append(stmt, t)
			continue
		}
		if t.Kind == Punct {
			switch t.Text {
			case "(":
				paren++
			case ")":
				paren--
			case "{":
				if brace == 0 && paren == 0 {
					if len(stmt) > 0 {
						openPrev = stmt[len(stmt)-1]
					}
				}
				brace++
			case "}":
				brace--
				stmt = append(stmt, t)
				if brace == 0 && paren == 0 && openPrev.Text == ")" {
					flush(true)
					continue
				}
				if brace < 0 { // stray closing brace (extern "C" {})
					brace = 0
					stmt = nil
				}
				continue
			case ";":
				if brace == 0 && paren == 0 {
					stmt = append(stmt, t)
					flush(false)
					continue
				}
			}
		}
		if brace == 0 && paren == 0 && t.Kind == Punct && t.Text == "{" && len(stmt) > 0 && stmt[0].Text == "extern" && len(stmt) == 2 && stmt[1].Kind == String {
			// extern "C" {
			stmt, brace = nil, 0
			continue
		}
		stmt = append(stmt, t)
	}
	flush(false)
}

func ignoredDoc(d string) bool {
	return strings.Contains(d, "\\file") || strings.Contains(d, "\\name ") || strings.HasPrefix(d, "\\defgroup") ||
		strings.HasPrefix(d, "\\cond") || strings.HasPrefix(d, "\\endcond")
}

func (p *Parser) skip(toks []Token, why string) {
	if len(toks) == 0 {
		return
	}
	n := len(toks)
	if n > 12 {
		n = 12
	}
	p.Unit.Skipped = append(p.Unit.Skipped, fmt.Sprintf("%s:%d: %s: %s", toks[0].File, toks[0].Line, why, joinTokens(toks[:n])))
}

// stripAnnotations removes the SDL attribute macros from a declaration.
func stripAnnotations(toks []Token) []Token {
	out := make([]Token, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == Ident {
			if stripIdents[t.Text] {
				continue
			}
			if stripCalls[t.Text] && i+1 < len(toks) && toks[i+1].Text == "(" {
				depth := 0
				j := i + 1
				for ; j < len(toks); j++ {
					if toks[j].Text == "(" {
						depth++
					} else if toks[j].Text == ")" {
						depth--
						if depth == 0 {
							break
						}
					}
				}
				i = j
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func (p *Parser) statement(raw []Token, doc string, body bool) {
	toks := stripAnnotations(raw)
	// drop the trailing ';'
	if n := len(toks); n > 0 && toks[n-1].Text == ";" {
		toks = toks[:n-1]
	}
	if len(toks) == 0 {
		return
	}
	switch {
	case toks[0].Text == "typedef":
		p.typedef(toks[1:], doc)
	case body:
		if name := funcName(toks); name != "" {
			p.Unit.Inline = append(p.Unit.Inline, name)
		}
	case (toks[0].Text == "struct" || toks[0].Text == "union") && indexTok(toks, "{") > 0:
		p.aggregateDef(toks, doc)
	case toks[0].Text == "enum" && indexTok(toks, "{") > 0:
		p.enumDef(toks, doc)
	case toks[0].Kind == Ident && (strings.HasSuffix(toks[0].Text, "_ASSERT") || strings.HasSuffix(toks[0].Text, "_DEFINE_HANDLE") || strings.HasPrefix(toks[0].Text, "VK_DEFINE_")):
		// compile-time checks and opaque external handle macros
	case indexTok(toks, "(") > 0:
		p.function(toks, doc)
	default:
		p.skip(toks, "unrecognised statement")
	}
}

func funcName(toks []Token) string {
	k := indexTok(toks, "(")
	if k > 0 && toks[k-1].Kind == Ident {
		return toks[k-1].Text
	}
	return ""
}

func indexTok(toks []Token, s string) int {
	for i, t := range toks {
		if t.Kind == Punct && t.Text == s {
			return i
		}
	}
	return -1
}

// matching returns the index of the token closing the bracket opened at i.
func matching(toks []Token, i int) int {
	open := toks[i].Text
	closeS := map[string]string{"(": ")", "{": "}", "[": "]"}[open]
	depth := 0
	for j := i; j < len(toks); j++ {
		if toks[j].Kind != Punct {
			continue
		}
		switch toks[j].Text {
		case open:
			depth++
		case closeS:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func (p *Parser) function(toks []Token, doc string) {
	k := indexTok(toks, "(")
	if k < 2 || toks[k-1].Kind != Ident {
		p.skip(toks, "function without return type")
		return
	}
	end := matching(toks, k)
	if end < 0 {
		p.skip(toks, "unbalanced function declaration")
		return
	}
	ret, _, err := parseDeclarator(toks[:k-1])
	if err != nil {
		p.skip(toks, "return type: "+err.Error())
		return
	}
	sig, err := parseParams(toks[k+1 : end])
	if err != nil {
		p.skip(toks, "params: "+err.Error())
		return
	}
	sig.Ret = ret
	f := &Func{Name: toks[k-1].Text, FuncSig: *sig, Doc: doc, File: toks[0].File, Line: toks[0].Line}
	p.Unit.Funcs = append(p.Unit.Funcs, f)
}

// splitTop splits toks at commas of nesting depth zero.
func splitTop(toks []Token, sep string) [][]Token {
	var parts [][]Token
	var cur []Token
	depth := 0
	for _, t := range toks {
		if t.Kind == Punct {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case sep:
				if depth == 0 {
					parts = append(parts, cur)
					cur = nil
					continue
				}
			}
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 || len(parts) > 0 {
		parts = append(parts, cur)
	}
	return parts
}

func parseParams(toks []Token) (*FuncSig, error) {
	sig := &FuncSig{}
	if len(toks) == 0 || len(toks) == 1 && toks[0].Text == "void" {
		return sig, nil
	}
	for _, part := range splitTop(toks, ",") {
		if len(part) == 1 && part[0].Text == "..." {
			sig.Variadic = true
			continue
		}
		ct, name, err := parseDeclarator(part)
		if err != nil {
			return nil, err
		}
		if ct.IsArray {
			ct.Ptr++
			ct.IsArray, ct.Array = false, nil
		}
		sig.Params = append(sig.Params, Param{Name: name, Type: ct})
	}
	return sig, nil
}

// parseDeclarator parses "specifiers *... [name] [array]" into a type and
// declarator name.
func parseDeclarator(toks []Token) (ct CType, name string, err error) {
	var words []string
	seenStar := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.Kind == Doc || t.Kind == Post:
		case t.Text == "const":
			if !seenStar {
				ct.Const = true
			}
		case t.Text == "struct" || t.Text == "union" || t.Text == "enum":
			ct.Tag = t.Text
		case t.Text == "*":
			ct.Ptr++
			seenStar = true
		case t.Text == "[":
			e := matching(toks, i)
			if e < 0 {
				return ct, "", fmt.Errorf("unbalanced [")
			}
			ct.IsArray, ct.Array = true, toks[i+1:e]
			i = e
		case t.Kind == Ident:
			words = append(words, t.Text)
		default:
			return ct, "", fmt.Errorf("unexpected %q in declarator", t.Text)
		}
	}
	if len(words) == 0 {
		return ct, "", fmt.Errorf("empty type")
	}
	if ct.Tag != "" {
		ct.Name = words[0]
		if len(words) > 1 {
			name = words[1]
		}
		return ct, name, nil
	}
	n := 0
	for n < len(words) && builtinWords[words[n]] {
		n++
	}
	if n > 0 {
		ct.Name = normalizeBuiltin(words[:n])
		if n < len(words) {
			name = words[n]
		}
		return ct, name, nil
	}
	ct.Name = words[0]
	if len(words) > 1 {
		name = words[1]
	}
	if len(words) > 2 {
		return ct, name, fmt.Errorf("too many words: %v", words)
	}
	return ct, name, nil
}

func normalizeBuiltin(w []string) string {
	has := map[string]int{}
	for _, x := range w {
		has[x]++
	}
	switch {
	case has["double"] > 0:
		return "double"
	case has["float"] > 0:
		return "float"
	case has["void"] > 0:
		return "void"
	case has["char"] > 0:
		if has["unsigned"] > 0 {
			return "unsigned char"
		}
		if has["signed"] > 0 {
			return "signed char"
		}
		return "char"
	}
	u := has["unsigned"] > 0
	var base string
	switch {
	case has["long"] >= 2:
		base = "long long"
	case has["long"] == 1:
		base = "long"
	case has["short"] > 0:
		base = "short"
	default:
		base = "int"
	}
	if u {
		return "unsigned " + base
	}
	return base
}

// funcPtr recognises "ret ( * name ) ( params )" and returns its parts.
func funcPtr(toks []Token) (ret []Token, name string, params []Token, ok bool) {
	k := indexTok(toks, "(")
	if k < 1 || k+3 >= len(toks) || toks[k+1].Text != "*" || toks[k+2].Kind != Ident || toks[k+3].Text != ")" {
		return
	}
	if k+4 >= len(toks) || toks[k+4].Text != "(" {
		return
	}
	end := matching(toks, k+4)
	if end < 0 {
		return
	}
	return toks[:k], toks[k+2].Text, toks[k+5 : end], true
}

func (p *Parser) typedef(toks []Token, doc string) {
	if len(toks) == 0 {
		return
	}
	if ret, name, params, ok := funcPtr(toks); ok && indexTok(toks, "{") < 0 {
		r, _, err := parseDeclarator(ret)
		if err != nil {
			p.skip(toks, "callback return: "+err.Error())
			return
		}
		sig, err := parseParams(params)
		if err != nil {
			p.skip(toks, "callback params: "+err.Error())
			return
		}
		sig.Ret = r
		p.Unit.Typedefs = append(p.Unit.Typedefs, &Typedef{Name: name, Func: sig, Doc: doc, File: toks[0].File, Line: toks[0].Line})
		return
	}
	if b := indexTok(toks, "{"); b > 0 {
		e := matching(toks, b)
		if e < 0 {
			p.skip(toks, "unbalanced typedef body")
			return
		}
		name := ""
		for _, t := range toks[e+1:] {
			if t.Kind == Ident {
				name = t.Text
			}
		}
		if name == "" {
			p.skip(toks, "typedef declarator")
			return
		}
		td := &Typedef{Name: name, Doc: doc, File: toks[0].File, Line: toks[0].Line}
		head := toks[:b]
		switch head[0].Text {
		case "enum":
			en := p.parseEnumBody(toks[b+1 : e])
			en.Name, en.Doc, en.File, en.Line = name, doc, td.File, td.Line
			td.Enum = en
			td.Type = CType{Tag: "enum", Name: name}
			p.Unit.Enums = append(p.Unit.Enums, en)
		case "struct", "union":
			tag := ""
			if len(head) > 1 {
				tag = head[1].Text
			}
			ag := p.parseAggBody(head[0].Text, tag, toks[b+1:e])
			ag.Doc, ag.File, ag.Line = doc, td.File, td.Line
			td.Type = CType{Tag: head[0].Text, Name: tag, Agg: ag}
		default:
			p.skip(toks, "typedef of unknown aggregate")
			return
		}
		p.Unit.Typedefs = append(p.Unit.Typedefs, td)
		return
	}
	ct, name, err := parseDeclarator(toks)
	if err != nil || name == "" {
		p.skip(toks, "typedef")
		return
	}
	p.Unit.Typedefs = append(p.Unit.Typedefs, &Typedef{Name: name, Type: ct, Doc: doc, File: toks[0].File, Line: toks[0].Line})
}

func (p *Parser) aggregateDef(toks []Token, doc string) {
	b := indexTok(toks, "{")
	e := matching(toks, b)
	if e < 0 {
		p.skip(toks, "unbalanced aggregate")
		return
	}
	tag := ""
	if b > 1 {
		tag = toks[1].Text
	}
	ag := p.parseAggBody(toks[0].Text, tag, toks[b+1:e])
	ag.Doc, ag.File, ag.Line = doc, toks[0].File, toks[0].Line
	p.Unit.Aggs = append(p.Unit.Aggs, ag)
}

func (p *Parser) enumDef(toks []Token, doc string) {
	b := indexTok(toks, "{")
	e := matching(toks, b)
	if e < 0 {
		p.skip(toks, "unbalanced enum")
		return
	}
	en := p.parseEnumBody(toks[b+1 : e])
	if b > 1 {
		en.Name = toks[1].Text
	}
	en.Doc, en.File, en.Line = doc, toks[0].File, toks[0].Line
	p.Unit.Enums = append(p.Unit.Enums, en)
}

func (p *Parser) parseEnumBody(toks []Token) *Enum {
	en := &Enum{}
	var doc string
	for _, part := range splitMembers(toks) {
		var core []Token
		var post string
		for _, t := range part {
			switch t.Kind {
			case Doc:
				doc = t.Text
			case Post:
				post = t.Text
			default:
				core = append(core, t)
			}
		}
		if len(core) == 0 {
			if post != "" && len(en.Members) > 0 && en.Members[len(en.Members)-1].Doc == "" {
				en.Members[len(en.Members)-1].Doc = post
			}
			continue
		}
		m := EnumMember{Name: core[0].Text, Doc: doc}
		if m.Doc == "" {
			m.Doc = post
		}
		doc = ""
		if len(core) > 2 && core[1].Text == "=" {
			m.Expr = core[2:]
		}
		en.Members = append(en.Members, m)
	}
	return en
}

// splitMembers splits an enum body at top-level commas, keeping a trailing
// Post doc comment together with the member that precedes the comma.
func splitMembers(toks []Token) [][]Token {
	var parts [][]Token
	var cur []Token
	depth := 0
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == Punct {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					if i+1 < len(toks) && toks[i+1].Kind == Post {
						cur = append(cur, toks[i+1])
						i++
					}
					parts = append(parts, cur)
					cur = nil
					continue
				}
			}
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts
}

func (p *Parser) parseAggBody(kind, tag string, toks []Token) *Aggregate {
	ag := &Aggregate{Kind: kind, Tag: tag}
	var doc string
	var cur []Token
	depth := 0
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == Doc && depth == 0 && len(cur) == 0 {
			doc = t.Text
			continue
		}
		if t.Kind == Post && depth == 0 && len(cur) == 0 {
			if n := len(ag.Fields); n > 0 && ag.Fields[n-1].Doc == "" {
				ag.Fields[n-1].Doc = t.Text
			}
			continue
		}
		if t.Kind == Punct {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ";":
				if depth == 0 {
					p.parseField(ag, cur, doc)
					cur, doc = nil, ""
					continue
				}
			}
		}
		if t.Kind != Doc && t.Kind != Post {
			cur = append(cur, t)
		}
	}
	return ag
}

func (p *Parser) parseField(ag *Aggregate, toks []Token, doc string) {
	toks = stripAnnotations(toks)
	if len(toks) == 0 {
		return
	}
	if ret, name, params, ok := funcPtr(toks); ok {
		r, _, err := parseDeclarator(ret)
		sig, err2 := parseParams(params)
		if err != nil || err2 != nil {
			p.skip(toks, "function pointer field")
			return
		}
		sig.Ret = r
		ag.Fields = append(ag.Fields, Field{Name: name, Func: sig, Doc: doc})
		return
	}
	if b := indexTok(toks, "{"); b >= 0 && (toks[0].Text == "struct" || toks[0].Text == "union") {
		e := matching(toks, b)
		if e < 0 {
			p.skip(toks, "unbalanced nested aggregate")
			return
		}
		tag := ""
		if b > 1 {
			tag = toks[1].Text
		}
		inner := p.parseAggBody(toks[0].Text, tag, toks[b+1:e])
		for _, d := range splitTop(toks[e+1:], ",") {
			ct := CType{Tag: toks[0].Text, Name: tag, Agg: inner}
			name := ""
			for _, t := range d {
				switch {
				case t.Text == "*":
					ct.Ptr++
				case t.Kind == Ident:
					name = t.Text
				}
			}
			ag.Fields = append(ag.Fields, Field{Name: name, Type: ct, Doc: doc})
			if len(d) == 0 {
				break
			}
		}
		return
	}
	// a, b declarators share the base type.
	decls := splitTop(toks, ",")
	first, name, err := parseDeclarator(decls[0])
	if err != nil {
		p.skip(toks, "field: "+err.Error())
		return
	}
	ag.Fields = append(ag.Fields, Field{Name: name, Type: first, Doc: doc})
	for _, d := range decls[1:] {
		ct := CType{Tag: first.Tag, Name: first.Name, Const: first.Const}
		var nm string
		for i := 0; i < len(d); i++ {
			t := d[i]
			switch {
			case t.Text == "*":
				ct.Ptr++
			case t.Text == "[":
				if e := matching(d, i); e > 0 {
					ct.IsArray, ct.Array = true, d[i+1:e]
					i = e
				}
			case t.Kind == Ident:
				nm = t.Text
			}
		}
		ag.Fields = append(ag.Fields, Field{Name: nm, Type: ct, Doc: doc})
	}
}
