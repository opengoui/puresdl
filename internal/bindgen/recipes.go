package bindgen

import (
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// This file holds the recipes: wrappers that compose several SDL calls and
// therefore cannot come from a single-function rule. Each recipe is keyed by
// the C function that makes it necessary and is emitted once.

// memStreamKeys are the functions whose memory-backed IOStream stores the
// caller's pointer. Their presence triggers the copying IOFromBytes recipe.
var memStreamKeys = map[string]bool{"SDL_IOFromConstMem": true, "SDL_IOFromMem": true}

// memStreamDeps are the SDL functions and constants the IOFromBytes recipe
// calls; the recipe is skipped when any of them is not in the API.
var memStreamDeps = []string{"SDL_IOFromDynamicMem", "SDL_WriteIO", "SDL_SeekIO", "SDL_CloseIO"}

// emitMemStream emits IOFromBytes, which builds a readable, writable and
// seekable stream over a copy of a Go slice. The copy lives in SDL dynamic
// memory, so unlike IOFromMem/IOFromConstMem nothing has to be kept alive.
func (g *gen) emitMemStream() {
	var key *ir.Function
	have := map[string]*ir.Function{}
	for _, f := range g.api.Funcs {
		have[f.Name] = f
		if memStreamKeys[f.Name] && key == nil {
			key = f
		}
	}
	if key == nil {
		return
	}
	for _, d := range memStreamDeps {
		if have[d] == nil {
			return
		}
	}
	if !g.hasConst("SDL_IO_SEEK_SET") {
		return
	}
	ret, err := g.typ(have["SDL_IOFromDynamicMem"].Sig.Ret)
	if err != nil || !g.claim("IOFromBytes", key.Name+" (copying memory stream)") {
		return
	}
	doc := []string{
		"IOFromBytes returns a readable, writable and seekable IOStream (the dynamic memory kind from IOFromDynamicMem) positioned at 0 over a copy of b.",
		"",
		"Writes go to that copy, never to b. The copy is owned by SDL and released by CloseIO, so b may be reused or collected right away and nothing has to be kept alive.",
		"It returns nil for an empty slice or when SDL fails (see GetError).",
		"",
		"See also: " + goName(key.Name) + ", IOFromDynamicMem",
	}
	body := []jen.Code{
		jen.If(jen.Len(jen.Id("b")).Op("==").Lit(0)).Block(jen.Return(jen.Nil())),
		jen.Id("s").Op(":=").Id("IOFromDynamicMem").Call(),
		jen.If(jen.Id("s").Op("==").Nil()).Block(jen.Return(jen.Nil())),
		jen.Id("n").Op(":=").Uint().Call(jen.Len(jen.Id("b"))),
		jen.If(jen.Id("WriteIO").Call(jen.Id("s"), jen.Qual("unsafe", "Pointer").Call(jen.Op("&").Id("b").Index(jen.Lit(0))), jen.Id("n")).Op("!=").Id("n").
			Op("||").Id("SeekIO").Call(jen.Id("s"), jen.Lit(0), jen.Id(goName("SDL_IO_SEEK_SET"))).Op("!=").Lit(0)).Block(
			jen.Id("CloseIO").Call(jen.Id("s")),
			jen.Return(jen.Nil()),
		),
		jen.Qual("runtime", "KeepAlive").Call(jen.Id("b")),
		jen.Return(jen.Id("s")),
	}
	fb := g.file(key.File)
	fb.funcs = append(fb.funcs, start(doc).Func().Id("IOFromBytes").Params(jen.Id("b").Index().Byte()).Add(ret).Block(body...))
}

// hasConst reports whether name is a macro constant or enum member of the API.
func (g *gen) hasConst(name string) bool {
	for _, k := range g.api.Consts {
		if k.Name == name {
			return true
		}
	}
	for _, e := range g.api.Enums {
		for _, m := range e.Members {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}

// ownedLoad describes a loader of the shape
//
//	bool Load(SDL_IOStream *src, bool closeio, T *out..., Uint8 **buf, Uint32 *len)
//
// whose buffer the caller must release with SDL_free (SDL_LoadWAV_IO).
type ownedLoad struct {
	outs   []int // indexes of the pointer-to-struct out parameters, returned by value
	buf    int
	length int
}

// ownedLoadOf returns the owned-buffer loader shape of f, or nil. The
// documentation must tell the caller to free the buffer with SDL_free; that is
// what makes copying it and calling Free the right ownership transfer.
func ownedLoadOf(f *ir.Function) *ownedLoad {
	if handWritten[f.Name] || f.Sig.Variadic || f.Sig.Ret.Resolve().Kind != ir.Bool || !freedStringRe.MatchString(f.Doc) {
		return nil
	}
	ps := f.Sig.Params
	if len(ps) < 4 {
		return nil
	}
	src := ps[0].Type.Resolve()
	if src.Kind != ir.Pointer || ps[0].Name != "src" || ps[1].Name != "closeio" || ps[1].Type.Resolve().Kind != ir.Bool {
		return nil
	}
	if e := src.Elem.Resolve(); e.Kind != ir.Struct && e.Kind != ir.Union {
		return nil
	}
	ol := &ownedLoad{buf: len(ps) - 2, length: len(ps) - 1}
	bt := ps[ol.buf].Type.Resolve()
	if bt.Kind != ir.Pointer || bt.Elem.Resolve().Kind != ir.Pointer {
		return nil
	}
	if be := bt.Elem.Resolve().Elem.Resolve(); be.Kind != ir.Int || be.Size != 1 || be.Signed {
		return nil
	}
	lt := ps[ol.length].Type.Resolve()
	if lt.Kind != ir.Pointer || lt.Elem.Resolve().Kind != ir.Int || lt.Elem.Resolve().Size != 4 || lt.Elem.Resolve().Signed {
		return nil
	}
	for i := 2; i < ol.buf; i++ {
		t := ps[i].Type.Resolve()
		if t.Kind != ir.Pointer || t.Const {
			return nil
		}
		if k := t.Elem.Resolve().Kind; k != ir.Struct {
			return nil
		}
		ol.outs = append(ol.outs, i)
	}
	return ol
}

// emitOwnedLoad emits <Name>Bytes (without the _IO suffix) for the loader f:
// it takes the file contents as a []byte, runs f over a copying memory stream
// that f closes, and returns the out structs, a Go copy of the buffer (the SDL
// allocation is released with Free) and an error.
func (g *gen) emitOwnedLoad(f *ir.Function, ol *ownedLoad, name string) {
	if g.claimed["IOFromBytes"] == "" {
		return
	}
	bytesName := strings.TrimSuffix(name, "_IO") + "Bytes"
	emptyVar := "err" + strings.TrimSuffix(bytesName, "Bytes") + "Empty"
	ps := f.Sig.Params
	var outTypes []*jen.Statement
	for _, i := range ol.outs {
		t, err := g.typ(ps[i].Type.Resolve().Elem)
		if err != nil {
			return
		}
		outTypes = append(outTypes, t)
	}
	bufT, err := g.typ(ps[ol.buf].Type.Resolve().Elem)
	if err != nil {
		return
	}
	lenT, err := g.typ(ps[ol.length].Type.Resolve().Elem)
	if err != nil {
		return
	}
	if !g.claim(bytesName, f.Name+" (owned buffer, bytes form)") || !g.claim(emptyVar, f.Name+" (empty data error)") {
		return
	}
	bufName, lenName := paramName(ps[ol.buf].Name, ol.buf), paramName(ps[ol.length].Name, ol.length)
	res := []jen.Code{}
	for _, i := range ol.outs {
		res = append(res, jen.Id(paramName(ps[i].Name, i)))
	}
	zeros := func(tail ...jen.Code) []jen.Code {
		var r []jen.Code
		for _, t := range outTypes {
			r = append(r, jen.Add(t).Values())
		}
		return append(r, tail...)
	}
	call := []jen.Code{jen.Id("src"), jen.True()}
	var decls []jen.Code
	var results []jen.Code
	for k, i := range ol.outs {
		n := paramName(ps[i].Name, i)
		decls = append(decls, jen.Var().Id(n).Add(outTypes[k]))
		call = append(call, jen.Op("&").Id(n))
		results = append(results, jen.Add(outTypes[k]))
	}
	call = append(call, jen.Op("&").Id(bufName), jen.Op("&").Id(lenName))
	results = append(results, jen.Index().Byte(), jen.Error())
	fail := func(format string, args ...jen.Code) jen.Code {
		return jen.Return(zeros(jen.Nil(), jen.Qual("fmt", "Errorf").Call(append([]jen.Code{jen.Lit(format)}, args...)...))...)
	}
	body := append([]jen.Code{}, decls...)
	body = append(body,
		jen.If(jen.Len(jen.Id("data")).Op("==").Lit(0)).Block(jen.Return(append(append([]jen.Code{}, res...), jen.Nil(), jen.Id(emptyVar))...)),
		jen.Id("src").Op(":=").Id("IOFromBytes").Call(jen.Id("data")),
		jen.If(jen.Id("src").Op("==").Nil()).Block(fail("sdl: "+bytesName+": cannot open data: %s", jen.Id("GetError").Call())),
		jen.Var().Id(bufName).Add(bufT),
		jen.Var().Id(lenName).Add(lenT),
		jen.If(jen.Op("!").Id(name).Call(call...)).Block(fail("sdl: "+bytesName+": %s", jen.Id("GetError").Call())),
		jen.Id("out").Op(":=").Make(jen.Index().Byte(), jen.Id(lenName)),
		jen.If(jen.Id(lenName).Op(">").Lit(0)).Block(
			jen.Copy(jen.Id("out"), jen.Qual("unsafe", "Slice").Call(jen.Id(bufName), jen.Id(lenName)))),
		jen.Id("Free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id(bufName))),
		jen.Return(append(append([]jen.Code{}, res...), jen.Id("out"), jen.Nil())...),
	)
	doc := []string{
		bytesName + " is the []byte form of " + name + ": it loads from data, a copy of the file contents, instead of an IOStream.",
		"",
		"The returned audio is a Go copy; the SDL-allocated buffer is released with Free before returning. Empty data returns the fixed error " + emptyVar + ". The stream is closed by " + name + ".",
		"",
		"See also: " + name + ", IOFromBytes",
	}
	fb := g.file(f.File)
	fb.funcs = append(fb.funcs,
		jen.Var().Id(emptyVar).Op("=").Qual("errors", "New").Call(jen.Lit("sdl: "+bytesName+": empty data")),
		start(doc).Func().Id(bytesName).Params(jen.Id("data").Index().Byte()).Params(results...).Block(body...))
}

// ownedLoadNote is the doc note of the raw loader that points at its []byte form.
func ownedLoadNote(name string) string {
	return strings.TrimSuffix(name, "_IO") + "Bytes loads from a []byte, copies the buffer and releases it; the buffer returned here is SDL-allocated and must be released with Free."
}
