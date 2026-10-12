package bindgen

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

func intLit(v *big.Int) jen.Code {
	if v.Sign() >= 0 && v.Cmp(big.NewInt(0x100)) >= 0 {
		return jen.Id("0x" + strings.ToUpper(v.Text(16)))
	}
	return jen.Id(v.String())
}

func (g *gen) emitEnums() {
	for _, e := range g.api.Enums {
		f := e.File
		if f == "" {
			if td, ok := g.typedefs[e.Name]; ok {
				f = td.File
			}
		}
		if f == "" || len(e.Members) == 0 {
			continue
		}
		fb := g.file(base(f))
		typeName, typed := g.enumName[e]
		if typed {
			fb.types = append(fb.types, start(g.docText(e.Doc)).Type().Id(typeName).Id(intName(e.Size, e.Signed)))
		}
		var items []jen.Code
		for _, m := range e.Members {
			n := goName(m.Name)
			if !g.claim(n, m.Name) {
				continue
			}
			s := start(g.docText(m.Doc)).Id(n)
			if typed {
				s.Id(typeName)
			}
			items = append(items, s.Op("=").Add(intLit(m.Value)))
		}
		if len(items) > 0 {
			fb.consts = append(fb.consts, jen.Const().Defs(items...))
		}
	}
}

var skipConst = []string{"SDL_PRI", "SDL_SCN"}

func (g *gen) emitConsts() {
	byFile := map[string][]jen.Code{}
	var order []string
	for _, k := range g.api.Consts {
		skip := false
		for _, p := range skipConst {
			if hasPrefix(k.Name, p) {
				skip = true
			}
		}
		if skip {
			continue
		}
		n := goName(k.Name)
		if prev, ok := g.claimed[n]; ok {
			if prev != k.Name {
				g.skip(k.Name, "Go name "+n+" already used by "+prev)
			}
			continue
		}
		g.claim(n, k.Name)
		var v jen.Code
		typeName := ""
		switch k.Kind {
		case ir.ConstInt:
			v = intLit(k.Int)
			typeName = g.constType(k)
		case ir.ConstFloat:
			v = jen.Lit(k.Float)
		default:
			v = jen.Lit(k.Str)
		}
		if _, ok := byFile[k.File]; !ok {
			order = append(order, k.File)
		}
		stmt := start(g.docText(k.Doc)).Id(n)
		if typeName != "" {
			stmt.Id(typeName)
			g.typedConsts++
		}
		byFile[k.File] = append(byFile[k.File], stmt.Op("=").Add(v))
	}
	for _, f := range order {
		g.file(f).consts = append(g.file(f).consts, jen.Const().Defs(byFile[f]...))
	}
}

type argPlan struct {
	hidden bool // not part of the public signature (slice result count)
	name   string
	pub    *jen.Statement
	raw    *jen.Statement
	expr   jen.Code
}

func (g *gen) emitFuncs() {
	for _, f := range g.api.Funcs {
		if why := g.bindFunc(f); why != "" {
			g.skip(f.Name, why)
		}
	}
}

func isFmtParam(p ir.Param) bool {
	return p.Name == "fmt" && isCString(p.Type) && p.Type.Resolve().Const
}

// bindFunc emits the wrapper for f and returns a reason when it is skipped.
func (g *gen) bindFunc(f *ir.Function) string {
	if f.File == "SDL_stdinc.h" && !keptStdinc.MatchString(f.Name) {
		return "libc replacement"
	}
	sig := f.Sig
	printf := false
	if sig.Variadic {
		if n := len(sig.Params); n == 0 || !isFmtParam(sig.Params[n-1]) {
			return "C variadic function"
		}
		printf = true
	}
	var sr *sliceRes
	if !handWritten[f.Name] {
		sr = g.sliceResult(f)
	}
	var plans []argPlan
	var notes []string
	for i, p := range sig.Params {
		name := paramName(p.Name, i)
		if sr != nil && i == sr.idx {
			plans = append(plans, argPlan{hidden: true, name: name, raw: jen.Op("*").Add(sr.cnt), expr: jen.Op("&").Id("n")})
			continue
		}
		if ov, ok := paramTypeOverrides[goName(f.Name)+"."+p.Name]; ok {
			plans = append(plans, argPlan{name: name, pub: jen.Id(ov), raw: jen.Id(ov), expr: jen.Id(name)})
			continue
		}
		if printf && i == len(sig.Params)-1 {
			plans = append(plans, argPlan{name: name, raw: jen.Op("*").Id("byte"),
				expr: jen.Id("cprintf").Call(jen.Id("format"), jen.Id("args"))})
			continue
		}
		if isCString(p.Type) && p.Type.Resolve().Const {
			if np, ok := nullParams[f.Name+"."+p.Name]; ok && np.distinct {
				notes = append(notes, "A nil "+name+" is passed as NULL, which "+np.effect+"; an empty string is passed as an empty C string.")
				plans = append(plans, argPlan{name: name, pub: jen.Op("*").String(), raw: jen.Op("*").Id("byte"),
					expr: jen.Id("cstrPtr").Call(jen.Id(name))})
				continue
			}
			fn := "cstr"
			if nullable(p.Doc) {
				fn = "cstrNull"
				notes = append(notes, "An empty "+name+" is passed as NULL.")
			}
			plans = append(plans, argPlan{name: name, pub: jen.String(), raw: jen.Op("*").Id("byte"),
				expr: jen.Id(fn).Call(jen.Id(name))})
			continue
		}
		if isWcharString(p.Type) {
			notes = append(notes, "An empty "+name+" is passed as NULL.")
			plans = append(plans, argPlan{name: name, pub: jen.String(), raw: jen.Op("*").Id("Wchar"),
				expr: jen.Id("wcstrNull").Call(jen.Id(name))})
			continue
		}
		t, err := g.typ(p.Type)
		if err != nil {
			return "parameter " + name + ": " + err.Error()
		}
		if isGUID(p.Type) {
			plans = append(plans, argPlan{name: name, pub: t, raw: jen.Id("guidABI"), expr: jen.Id("guidToABI").Call(jen.Id(name))})
			continue
		}
		plans = append(plans, argPlan{name: name, pub: t, raw: t, expr: jen.Id(name)})
	}
	var rawRet, pubRet *jen.Statement
	retString, retFree, guidRet := false, false, false
	if sr != nil {
		rawRet, pubRet = sr.raw, sr.pub
	} else if rr := sig.Ret.Resolve(); rr.Kind != ir.Void {
		if isCString(sig.Ret) {
			rawRet, pubRet, retString = jen.Op("*").Id("byte"), jen.String(), true
			retFree = !rr.Const && freedStringRe.MatchString(f.Doc)
		} else {
			t, err := g.typ(sig.Ret)
			if err != nil {
				return "result: " + err.Error()
			}
			rawRet, pubRet = t, t
			if isGUID(sig.Ret) {
				rawRet, guidRet = jen.Id("guidABI"), true
			}
		}
	}
	n := goName(f.Name)
	if !g.claim(n, f.Name) {
		return ""
	}
	fnVar := "fn" + n
	var rawParams, pubParams, args []jen.Code
	for i, p := range plans {
		rawParams = append(rawParams, jen.Id(p.name).Add(p.raw))
		args = append(args, p.expr)
		if p.hidden {
			continue
		}
		if printf && i == len(plans)-1 {
			pubParams = append(pubParams, jen.Id("format").String(), jen.Id("args").Op("...").Any())
			continue
		}
		pubParams = append(pubParams, jen.Id(p.name).Add(p.pub))
	}
	rawFn := jen.Func().Params(rawParams...)
	if rawRet != nil {
		rawFn = rawFn.Add(rawRet)
	}
	fb := g.file(f.File)
	fb.funcs = append(fb.funcs, jen.Var().Id(fnVar).Add(rawFn))
	g.binds = append(g.binds, [2]string{fnVar, f.Name})
	if handWritten[f.Name] {
		return ""
	}

	doc := g.docText(f.Doc)
	if printf {
		notes = append(notes, "The message is formatted with fmt.Sprintf when args are given and used verbatim otherwise.")
	}
	if n := keepAliveNote(f); n != "" {
		notes = append(notes, n)
	}
	if len(f.Targets) < 3 {
		notes = append(notes, "Only declared by the SDL headers for "+strings.Join(f.Targets, ", ")+".")
	}
	_, okVariant := nullDistinctResults[f.Name]
	okVariant = okVariant && retString && !retFree
	okName := n + "OK"
	if okVariant {
		notes = append(notes, "NULL is returned as an empty string; use "+okName+" to tell it from an empty value.")
	}
	if len(notes) > 0 {
		doc = append(doc, "")
		doc = append(doc, notes...)
	}
	call := jen.Id(fnVar).Call(args...)
	var body []jen.Code
	switch {
	case sr != nil:
		doc = append(doc, "", "The SDL allocation is copied into the returned slice and released.")
		if sr.idx >= 0 {
			body = []jen.Code{jen.Var().Id("n").Add(sr.cnt)}
		}
		body = append(body, sr.finish(call)...)
	case rawRet == nil:
		body = []jen.Code{call}
	case retFree:
		body = []jen.Code{
			jen.Id("p").Op(":=").Add(call),
			jen.If(jen.Id("p").Op("==").Nil()).Block(jen.Return(jen.Lit(""))),
			jen.Id("s").Op(":=").Id("GoString").Call(jen.Id("p")),
			jen.Id("Free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("p"))),
			jen.Return(jen.Id("s")),
		}
	case retString:
		body = []jen.Code{jen.Return(jen.Id("GoString").Call(call))}
	case guidRet:
		body = []jen.Code{jen.Return(call.Dot("guid").Call())}
	default:
		body = []jen.Code{jen.Return(call)}
	}
	w := start(doc).Func().Id(n).Params(pubParams...)
	if pubRet != nil {
		w = w.Add(pubRet)
	}
	fb.funcs = append(fb.funcs, w.Block(body...))
	if okVariant && g.claim(okName, f.Name+" (OK variant)") {
		okDoc := append(g.docText(f.Doc), "", "The boolean is false when SDL returned NULL, which is different from an empty value.")
		if len(f.Targets) < 3 {
			okDoc = append(okDoc, "Only declared by the SDL headers for "+strings.Join(f.Targets, ", ")+".")
		}
		okBody := []jen.Code{
			jen.Id("p").Op(":=").Add(jen.Id(fnVar).Call(args...)),
			jen.If(jen.Id("p").Op("==").Nil()).Block(jen.Return(jen.Lit(""), jen.False())),
			jen.Return(jen.Id("GoString").Call(jen.Id("p")), jen.True()),
		}
		fb.funcs = append(fb.funcs, start(okDoc).Func().Id(okName).Params(pubParams...).Params(jen.String(), jen.Bool()).Block(okBody...))
	}
	_ = fmt.Sprint
	return ""
}

// isGUID reports whether t is SDL_GUID. purego mis-classifies 16-byte structs
// whose only field is an array on amd64, so GUIDs cross the C boundary as the
// equivalent guidABI (two uint64 words) and are converted in the wrapper.
func isGUID(t *ir.Type) bool { return t.Kind == ir.Named && t.Name == "SDL_GUID" }

// sliceRes describes a function whose result is an SDL_free-able array with
// an out-parameter holding its length.
type sliceRes struct {
	idx      int
	cnt      *jen.Statement // type of the count variable
	raw, pub *jen.Statement
	finish   func(call *jen.Statement) []jen.Code
}

var countNames = map[string]bool{"count": true, "num_mime_types": true, "datasize": true, "size": true}

// deepCopy lists the structs that SDL returns in an array of pointers whose
// pointees live in the same allocation as the array.
var deepCopy = map[string]bool{"SDL_CameraSpec": true, "SDL_DisplayMode": true, "SDL_GamepadBinding": true, "SDL_Finger": true}

func (g *gen) sliceResult(f *ir.Function) *sliceRes {
	rt := f.Sig.Ret.Resolve()
	if rt.Kind != ir.Pointer || rt.Const || isCString(f.Sig.Ret) || !freedStringRe.MatchString(f.Doc) {
		return nil
	}
	idx := -1
	for i, p := range f.Sig.Params {
		pr := p.Type.Resolve()
		if pr.Kind == ir.Pointer && countNames[p.Name] && pr.Elem.Resolve().Kind == ir.Int {
			idx = i
		}
	}
	elem := rt.Elem.Resolve()
	raw, err := g.typ(f.Sig.Ret)
	if err != nil {
		return nil
	}
	free := func(helper string, extra ...jen.Code) func(*jen.Statement) []jen.Code {
		return func(call *jen.Statement) []jen.Code {
			return []jen.Code{jen.Return(jen.Id(helper).Call(append([]jen.Code{call}, extra...)...))}
		}
	}
	n := jen.Int().Call(jen.Id("n"))
	_ = n
	count := func() jen.Code { return jen.Int().Call(jen.Id("n")) }
	if idx < 0 {
		// NULL-terminated array of strings
		if elem.Kind == ir.Pointer && isCString(rt.Elem) {
			return &sliceRes{idx: -1, raw: raw, pub: jen.Index().String(), cnt: jen.Int(), finish: func(call *jen.Statement) []jen.Code {
				return []jen.Code{jen.Return(jen.Id("copyFreeStrings0").Call(call))}
			}}
		}
		return nil
	}
	cnt, err := g.typ(f.Sig.Params[idx].Type.Resolve().Elem)
	if err != nil {
		return nil
	}
	sr := &sliceRes{idx: idx, cnt: cnt, raw: raw}
	switch {
	case elem.Kind == ir.Void:
		sr.pub = jen.Index().Byte()
		sr.finish = func(call *jen.Statement) []jen.Code {
			return []jen.Code{jen.Id("p").Op(":=").Add(call), jen.Return(jen.Id("copyFreeBytes").Call(jen.Id("p"), count()))}
		}
	case elem.Kind == ir.Pointer && isCString(rt.Elem):
		sr.pub = jen.Index().String()
		sr.finish = func(call *jen.Statement) []jen.Code {
			return []jen.Code{jen.Id("p").Op(":=").Add(call), jen.Return(jen.Id("copyFreeStrings").Call(jen.Id("p"), count()))}
		}
	case elem.Kind == ir.Pointer:
		inner := elem.Elem.Resolve()
		et, err := g.typ(rt.Elem.Resolve().Elem)
		if err != nil {
			return nil
		}
		helper := "copyFree"
		pubElem := jen.Op("*").Add(et)
		if (inner.Kind == ir.Struct || inner.Kind == ir.Union) && deepCopy[inner.Rec.Name] {
			helper, pubElem = "copyFreeDeref", et
		}
		sr.pub = jen.Index().Add(pubElem)
		sr.finish = func(call *jen.Statement) []jen.Code {
			return []jen.Code{jen.Id("p").Op(":=").Add(call), jen.Return(jen.Id(helper).Call(jen.Id("p"), count()))}
		}
	default:
		et, err := g.typ(rt.Elem)
		if err != nil {
			return nil
		}
		sr.pub = jen.Index().Add(et)
		sr.finish = func(call *jen.Statement) []jen.Code {
			return []jen.Code{jen.Id("p").Op(":=").Add(call), jen.Return(jen.Id("copyFree").Call(jen.Id("p"), count()))}
		}
	}
	_ = free
	return sr
}

// retainedRe matches doc text saying that SDL keeps using a pointer argument
// after the call returns instead of copying what it points to.
var retainedRe = regexp.MustCompile(`(?i)not copied|NoCopy|must (?:remain|stay) valid|keeps? a reference|remain valid[^.]{0,40}until the [^.]{0,40}(?:destroyed|closed)`)

// keepAliveNote returns the "Keep alive:" doc note of f, or "" when neither
// the function doc nor a parameter doc says that SDL retains a pointer
// argument. Go's garbage collector does not know about the reference SDL
// holds, so the caller has to keep the memory reachable.
func keepAliveNote(f *ir.Function) string {
	retained := retainedRe.MatchString(f.Doc) || retainedRe.MatchString(f.Name)
	hasPtr, hasString := false, false
	for _, p := range f.Sig.Params {
		retained = retained || retainedRe.MatchString(p.Doc)
		if r := p.Type.Resolve(); r.Kind == ir.Pointer {
			hasPtr = true
			hasString = hasString || isCString(p.Type)
		}
	}
	if !retained || !hasPtr {
		return storedPointerNote(f)
	}
	n := "Keep alive: SDL keeps the pointer arguments of this function without copying what they point to; keep that Go memory reachable (for example in a package-level variable) for as long as SDL may use it."
	if hasString {
		n += " String arguments are converted to a temporary C string per call and are not kept alive by this wrapper."
	}
	return n
}

// storedPointerFuncs are functions that store a void * argument without any
// doc text saying so.
var storedPointerFuncs = map[string]string{
	"SDL_SetPointerProperty":            "value",
	"SDL_SetPointerPropertyWithCleanup": "value",
	"SDL_SetTLS":                        "value",
}

// syncUserdataRe matches the functions whose userdata argument is only used
// during the call, or is a lookup key, so no reference outlives the call.
var syncUserdataRe = regexp.MustCompile(`^SDL_(Enumerate|FilterEvents|Remove)`)

// storedPointerNote returns the "Keep alive:" note for the functions that
// hand a void * (the userdata of a callback registration, or a property or
// TLS value) to SDL, which stores it and passes it back later. The Go garbage
// collector cannot see that reference.
func storedPointerNote(f *ir.Function) string {
	arg := storedPointerFuncs[f.Name]
	if arg == "" && !syncUserdataRe.MatchString(f.Name) {
		for _, p := range f.Sig.Params {
			if p.Name == "userdata" && p.Type.Resolve().Kind == ir.Pointer {
				arg = "userdata"
			}
		}
	}
	if arg == "" {
		return ""
	}
	return "Keep alive: SDL stores the " + arg + " argument and may use it after this call returns; Go's garbage collector does not see that reference, so keep the Go memory it points to reachable (for example in a package-level variable) for as long as SDL may use it."
}
