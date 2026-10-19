package bindgen

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// intLit renders v as a Go integer literal, in hex from 0x100 upward.
func intLit(v *big.Int) jen.Code {
	if v.Sign() >= 0 && v.Cmp(big.NewInt(0x100)) >= 0 {
		return jen.Id("0x" + strings.ToUpper(v.Text(16)))
	}
	return jen.Id(v.String())
}

// emitEnums generates a named integer type and constants for every enum.
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

// skipConst lists the name prefixes of printf-style format macros that are not bound.
var skipConst = []string{"SDL_PRI", "SDL_SCN"}

// emitConsts generates the macro constants into the buffer of their header.
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

// argPlan describes how one C parameter appears in the public wrapper and in
// the raw call.
type argPlan struct {
	hidden bool // not part of the public signature (slice result count)
	name   string
	pub    *jen.Statement
	raw    *jen.Statement
	expr   jen.Code
	// cstr names the conversion ("cstr" or "cstrNull") of a string
	// parameter that the main wrapper can make through a cstrScratch.
	cstr string
}

// emitFuncs generates a wrapper for every function and records the reason for
// each one that is skipped.
func (g *gen) emitFuncs() {
	g.emitMemStream()
	for _, f := range g.api.Funcs {
		if why := g.bindFunc(f); why != "" {
			g.skip(f.Name, why)
		}
	}
}

// isFmtParam reports whether p is the const char *fmt parameter of a printf-style function.
func isFmtParam(p ir.Param) bool {
	return p.Name == "fmt" && isCString(p.Type) && p.Type.Resolve().Const
}

// bindFunc emits the wrapper for f and returns a reason when it is skipped.
func (g *gen) bindFunc(f *ir.Function) string {
	if f.Name == "SDL_main" {
		return "defined by the application, not exported by libSDL3"
	}
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
		if isCString(p.Type) && p.Type.Resolve().Const && retainedParam(f, p) {
			plans = append(plans, argPlan{name: name, pub: jen.Op("*").Id("byte"), raw: jen.Op("*").Id("byte"), expr: jen.Id(name)})
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
				expr: jen.Id(fn).Call(jen.Id(name)), cstr: fn})
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
	var vf *nullParam
	vfIdx := -1
	for i, p := range sig.Params {
		if np, ok := nullParams[f.Name+"."+p.Name]; ok && np.distinct && np.valueForm && !plans[i].hidden {
			vf, vfIdx = &np, i
		}
	}
	var sf *sliceForm
	if sr == nil && !printf {
		sf = sliceFormOf(f, plans)
	}
	var pl *planarForm
	if sr == nil && !printf {
		pl = planarFormOf(f, plans)
	}
	var bf *bytesForm
	if sr == nil && !printf {
		bf = bytesFormOf(f, plans)
	}
	var tu *textureUpdate
	if sr == nil && !printf {
		tu = textureUpdateOf(f, plans)
	}
	var ol *ownedLoad
	if sr == nil && !printf {
		ol = ownedLoadOf(f)
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
	fb := g.file(f.File)
	if directCall(f) {
		addrVar := "addr" + n
		regVar := g.emitDirect(fb, f, fnVar, addrVar, plans, rawParams, rawRet)
		g.binds = append(g.binds, binding{goVar: addrVar, cName: f.Name, direct: true, fallback: regVar})
	} else {
		rawFn := jen.Func().Params(rawParams...)
		if rawRet != nil {
			rawFn = rawFn.Add(rawRet)
		}
		fb.funcs = append(fb.funcs, jen.Var().Id(fnVar).Add(rawFn))
		g.binds = append(g.binds, binding{goVar: fnVar, cName: f.Name})
	}
	if handWritten[f.Name] {
		return ""
	}

	doc := g.docText(f.Doc)
	if printf {
		notes = append(notes, "The message is formatted with fmt.Sprintf when args are given and used verbatim otherwise. Verbs follow Go fmt, not C printf: write %d instead of %u.")
	}
	if f.Name == "SDL_GetError" {
		notes = append(notes, "SDL error state belongs to the calling OS thread. If this goroutine may move between OS threads, use runtime.LockOSThread across the failing SDL call and this call.")
	}
	if n := keepAliveNote(f); n != "" {
		notes = append(notes, n)
	}
	if ol == nil && sf == nil {
		if n := freedBufferNote(f); n != "" {
			notes = append(notes, n)
		}
	}
	if sf != nil {
		notes = append(notes, sliceNote(n))
	}
	if pl != nil {
		notes = append(notes, planarNote(pl))
	}
	if bf != nil {
		notes = append(notes, bytesNote(n))
	}
	if tu != nil {
		notes = append(notes, textureUpdateNote(tu))
	}
	if ol != nil && g.claimed["IOFromBytes"] != "" {
		notes = append(notes, ownedLoadNote(n))
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
	// String parameters go through a cstrScratch released when the wrapper
	// returns, after any result string has been copied.
	scName := freshName("sc", plans)
	var scDecl []jen.Code
	for i, p := range plans {
		if p.cstr != "" {
			if scDecl == nil {
				scDecl = []jen.Code{
					jen.Var().Id(scName).Id("cstrScratch"),
					jen.Defer().Id(scName).Dot("release").Call(),
				}
			}
			args[i] = jen.Id(scName).Dot(p.cstr).Call(jen.Id(p.name))
		}
	}
	call := jen.Id(fnVar).Call(args...)
	var body []jen.Code
	switch {
	case sr != nil:
		doc = append(doc, "", "The SDL allocation is copied into the returned slice and released. A nil result means SDL reported failure (see GetError); a non-nil empty slice means success with no items.")
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
	body = append(scDecl, body...)
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
		okBody := append(scDecl,
			jen.Id("p").Op(":=").Add(jen.Id(fnVar).Call(args...)),
			jen.If(jen.Id("p").Op("==").Nil()).Block(jen.Return(jen.Lit(""), jen.False())),
			jen.Return(jen.Id("GoString").Call(jen.Id("p")), jen.True()),
		)
		fb.funcs = append(fb.funcs, start(okDoc).Func().Id(okName).Params(pubParams...).Params(jen.String(), jen.Bool()).Block(okBody...))
	}
	if vf != nil {
		g.emitValueForm(f, vf, vfIdx, plans, n, fb)
	}
	if sf != nil {
		g.emitSlice(f, sf, plans, n, fnVar, fb, doc)
	}
	if bf != nil {
		g.emitBytes(f, bf, plans, n, fnVar, fb)
	}
	if ol != nil {
		g.emitOwnedLoad(f, ol, n)
	}
	if pl != nil {
		g.emitPlanar(f, pl, plans, n, fb)
	}
	if tu != nil {
		g.emitTextureUpdate(f, tu, plans, n, fb)
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

// countNames are the parameter names SDL uses for the out-count of a returned array.
var countNames = map[string]bool{"count": true, "num_mime_types": true, "datasize": true, "size": true}

// deepCopy lists the structs that SDL returns in an array of pointers whose
// pointees live in the same allocation as the array.
var deepCopy = map[string]bool{"SDL_CameraSpec": true, "SDL_DisplayMode": true, "SDL_GamepadBinding": true, "SDL_Finger": true}

// sliceResult returns the plan for converting an SDL-allocated array result
// into a Go slice that is copied and freed, or nil when f does not return one.
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
var retainedRe = regexp.MustCompile(`(?i)not copied|NoCopy|must (?:remain|stay) (?:valid|available)|keeps? a reference|remain valid[^.]{0,40}until the [^.]{0,40}(?:destroyed|closed)`)

// keepAliveNote returns the "Keep alive:" doc note for pointers SDL retains.
// Go's garbage collector does not know about the reference SDL holds, so the
// caller has to keep that memory reachable.
func keepAliveNote(f *ir.Function) string {
	retainedParams := retainedPointerNames(f)
	stored := storedPointerNote(f)
	if len(retainedParams) == 0 {
		return stored
	}
	n := "Keep alive: SDL retains the " + strings.Join(retainedParams, ", ") + " pointer argument(s); keep that Go memory reachable (for example in a package-level variable) for as long as SDL may use it."
	if name := taskBufferParam(f); name != "" {
		n = "Keep alive: " + name + " must stay alive and unmoved until the task completes, and SDL may access it at any time until then; pass memory that Go keeps reachable and does not move (not a stack variable) and do not release it before the outcome is reported."
	}
	if stored != "" {
		n += " " + strings.TrimPrefix(stored, "Keep alive: ")
	}
	return n
}

// retainedParam reports whether SDL's function or parameter docs say that p
// remains in use after the call returns.
func retainedParam(f *ir.Function, p ir.Param) bool {
	for _, name := range retainedPointerNames(f) {
		if name == p.Name {
			return true
		}
	}
	return false
}

// retainedPointerNames prefers parameter docs when they identify a specific
// retained pointer, since function docs also contain the parameter sections.
func retainedPointerNames(f *ir.Function) []string {
	var pointers, documented []string
	for _, p := range f.Sig.Params {
		if p.Type.Resolve().Kind != ir.Pointer {
			continue
		}
		pointers = append(pointers, p.Name)
		if retainedRe.MatchString(p.Doc) {
			documented = append(documented, p.Name)
		}
	}
	if len(documented) > 0 {
		return documented
	}
	if retainedRe.MatchString(f.Doc) || retainedRe.MatchString(f.Name) {
		return pointers
	}
	return nil
}

// taskBufferRe matches doc text that ties the lifetime of a buffer argument
// to the completion of an asynchronous task.
var taskBufferRe = regexp.MustCompile("`(\\w+)`[^.]*must (?:remain|stay) (?:valid|available) until[^.]*(?:work|task)")

// taskBufferParam returns the pointer parameter that f's doc says must stay
// valid until the asynchronous work is done, or "".
func taskBufferParam(f *ir.Function) string {
	for _, m := range taskBufferRe.FindAllStringSubmatch(f.Doc, -1) {
		for _, p := range f.Sig.Params {
			if p.Name == m[1] && p.Type.Resolve().Kind == ir.Pointer {
				return p.Name
			}
		}
	}
	return ""
}

// freedBufferNote returns the doc note of a function that hands out a buffer
// allocated by SDL: a non-const double-pointer out parameter whose
// documentation tells the caller to use SDL_free.
func freedBufferNote(f *ir.Function) string {
	docs := f.Doc
	out := false
	for _, p := range f.Sig.Params {
		docs += " " + p.Doc
		if r := p.Type.Resolve(); r.Kind == ir.Pointer && !r.Const {
			if e := r.Elem.Resolve(); e != nil && e.Kind == ir.Pointer && !isCString(r.Elem) {
				out = true
			}
		}
	}
	if !out || !freedStringRe.MatchString(docs) {
		return ""
	}
	return "The buffer returned through the pointer parameters is SDL-allocated and must be released with Free."
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
	note := "Keep alive: SDL stores the " + arg + " argument and may use it after this call returns; Go's garbage collector does not see that reference, so keep the Go memory it points to reachable (for example in a package-level variable) for as long as SDL may use it."
	if asyncUserdataRe.MatchString(f.Name) {
		note += " " + asyncUserdataSentence
	}
	return note
}

// emitValueForm emits <Name>Value, which takes the nullable parameter vi as a
// plain string, and the optional clear helper that drops it and passes nil.
func (g *gen) emitValueForm(f *ir.Function, np *nullParam, vi int, plans []argPlan, n string, fb *fileBuf) {
	vname := plans[vi].name
	ret := f.Sig.Ret.Resolve().Kind != ir.Void
	var full, clear []jen.Code
	var withValue, withNil []jen.Code
	for i, p := range plans {
		if p.hidden {
			continue
		}
		if i == vi {
			full = append(full, jen.Id(p.name).String())
			withValue = append(withValue, jen.Op("&").Id(p.name))
			withNil = append(withNil, jen.Nil())
			continue
		}
		full = append(full, jen.Id(p.name).Add(p.pub))
		clear = append(clear, jen.Id(p.name).Add(p.pub))
		withValue = append(withValue, jen.Id(p.name))
		withNil = append(withNil, jen.Id(p.name))
	}
	body := func(args []jen.Code) jen.Code {
		c := jen.Id(n).Call(args...)
		if ret {
			return jen.Return(c)
		}
		return c
	}
	retT := func(w *jen.Statement) *jen.Statement {
		if ret {
			t, err := g.typ(f.Sig.Ret)
			if err == nil {
				return w.Add(t)
			}
		}
		return w
	}
	name := n + "Value"
	if g.claim(name, f.Name+" (value form)") {
		doc := []string{n + "Value is " + n + " with the " + vname + " parameter as a plain string instead of a *string.", "Use the clear helper to pass NULL."}
		if np.clearForm == "" {
			doc[1] = "Use " + n + " with a nil " + vname + " to pass NULL."
		} else {
			doc[1] = "Use " + np.clearForm + " to pass NULL."
		}
		fb.funcs = append(fb.funcs, retT(start(doc).Func().Id(name).Params(full...)).Block(body(withValue)))
	}
	if np.clearForm != "" && g.claim(np.clearForm, f.Name+" (clear form)") {
		doc := []string{np.clearForm + " is " + n + " with a nil " + vname + ", which " + np.effect + "."}
		fb.funcs = append(fb.funcs, retT(start(doc).Func().Id(np.clearForm).Params(clear...)).Block(body(withNil)))
	}
}
