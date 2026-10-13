package bindgen

import (
	"regexp"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// byteLenRe matches the names SDL gives the byte-length parameter that follows
// a void pointer to a buffer.
var byteLenRe = regexp.MustCompile(`^(len|length|size|buflen|datalen|num_bytes)$`)

// bytesFormExcluded lists functions with a (void *, length) pair that must keep
// only the raw form. The memory-backed IOStream constructors store the pointer,
// the asynchronous functions use the buffer after returning, and the SaveFile
// functions legitimately write an empty file for an empty buffer, which a
// no-op for empty slices would not do.
var bytesFormExcluded = map[string]bool{
	"SDL_IOFromMem":      true,
	"SDL_IOFromConstMem": true,
	"SDL_ReadAsyncIO":    true,
	"SDL_WriteAsyncIO":   true,
	"SDL_LoadFileAsync":  true,
	"SDL_SaveFile":       true,
	"SDL_SaveFile_IO":    true,
}

// bytesEmptyCalls lists functions whose []byte form must call SDL for an empty
// slice (with a NULL pointer and a zero length) instead of returning early:
// an empty file is a legitimate argument, and the length of a read must match
// the file exactly. Their "true if ... or false on failure" result is the
// standard success/failure result, so it is accepted without the usual
// "true on success" wording.
var bytesEmptyCalls = map[string]bool{
	"SDL_ReadStorageFile":  true,
	"SDL_WriteStorageFile": true,
}

// bytesForm describes a function with a (void *ptr, size) pair that also gets
// a []byte form.
type bytesForm struct {
	ptr, cnt int  // parameter indexes; cnt == ptr+1
	wide     bool // the length is a 64-bit unsigned integer rather than an int32
	call     bool // an empty slice still calls SDL with NULL and 0
}

// bytesFormOf returns the []byte form of f, or nil when it does not match. The
// pair is a pointer to void (const or not) immediately followed by an integer
// with a byte-length name. Functions that SDL keeps the buffer of, and the
// explicit exclusions, are skipped.
func bytesFormOf(f *ir.Function, plans []argPlan) *bytesForm {
	if handWritten[f.Name] || bytesFormExcluded[f.Name] || f.Sig.Variadic || keepAliveNote(f) != "" {
		return nil
	}
	ret := f.Sig.Ret.Resolve()
	switch ret.Kind {
	case ir.Void, ir.Int:
	case ir.Bool:
		if !successRe.MatchString(f.Doc) && !bytesEmptyCalls[f.Name] {
			return nil
		}
	default:
		return nil
	}
	ps := f.Sig.Params
	for i := 0; i+1 < len(ps); i++ {
		pt := ps[i].Type.Resolve()
		if pt.Kind != ir.Pointer || pt.Elem.Resolve().Kind != ir.Void {
			continue
		}
		ct := ps[i+1].Type.Resolve()
		if ct.Kind != ir.Int || !byteLenRe.MatchString(ps[i+1].Name) {
			continue
		}
		wide := ct.Size == 8 && !ct.Signed
		if !wide && !(ct.Size == 4 && ct.Signed) {
			continue
		}
		if nullable(ps[i].Doc) || plans[i].hidden || plans[i].pub == nil || plans[i+1].pub == nil {
			return nil
		}
		for j := range ps {
			if j != i && j != i+1 && ps[j].Type.Resolve().Kind == ir.Pointer && ps[j].Type.Resolve().Elem.Resolve().Kind == ir.Void {
				return nil // a second raw buffer: leave it to the raw wrapper
			}
		}
		return &bytesForm{ptr: i, cnt: i + 1, wide: wide, call: bytesEmptyCalls[f.Name]}
	}
	return nil
}

// bytesNote is the doc note of the raw wrapper that points at its []byte form.
func bytesNote(name string) string {
	return bytesName(name) + " takes a []byte instead of a pointer and length."
}

// bytesName is the exported name of the []byte form of the Go function name:
// a trailing "Data" is dropped, so PutAudioStreamData becomes
// PutAudioStreamBytes.
func bytesName(name string) string {
	return strings.TrimSuffix(name, "Data") + "Bytes"
}

// typedName is the exported name of the typed-slice form of the Go function
// name: PutAudioStreamData becomes PutAudioStreamSlice.
func typedName(name string) string {
	return strings.TrimSuffix(name, "Data") + "Slice"
}

// emitBytes emits <name>Bytes for f, which takes the buffer as a []byte and
// derives the length from it, and the generic <name>Slice form that takes a
// slice of any fixed-size numeric element type.
func (g *gen) emitBytes(f *ir.Function, bf *bytesForm, plans []argPlan, name, fnVar string, fb *fileBuf) {
	g.emitBytesForm(f, bf, plans, name, fnVar, fb, false)
	g.emitBytesForm(f, bf, plans, name, fnVar, fb, true)
}

// emitBytesForm emits the []byte form of f, or with typed the generic form
// whose slice elements are measured in bytes by their size.
func (g *gen) emitBytesForm(f *ir.Function, bf *bytesForm, plans []argPlan, name, fnVar string, fb *fileBuf, typed bool) {
	bytesName := bytesName(name)
	if typed {
		bytesName = typedName(name)
	}
	if !g.claim(bytesName, f.Name+" (bytes form)") {
		return
	}
	ps := f.Sig.Params
	b := "b"
	for _, p := range plans {
		if p.name == b {
			b = "data"
		}
	}
	call := bf.call
	var params, args []jen.Code
	for i, p := range plans {
		switch i {
		case bf.cnt:
			var n jen.Code = jen.Len(jen.Id(b))
			if typed || call {
				n = jen.Id("n")
			}
			args = append(args, jen.Add(p.pub.Clone()).Call(n))
			continue
		case bf.ptr:
			if typed {
				params = append(params, jen.Id(b).Index().Id("T"))
			} else {
				params = append(params, jen.Id(b).Index().Byte())
			}
			if call {
				args = append(args, jen.Id("ptr"))
			} else {
				args = append(args, jen.Qual("unsafe", "Pointer").Call(jen.Op("&").Id(b).Index(jen.Lit(0))))
			}
			continue
		}
		params = append(params, jen.Id(p.name).Add(p.pub))
		args = append(args, p.expr)
	}
	ret := f.Sig.Ret.Resolve()
	var retT *jen.Statement
	var empty, fail []jen.Code
	emptyDoc, failDoc := "returns without calling SDL", "returns"
	switch ret.Kind {
	case ir.Bool:
		retT = jen.Bool()
		empty, fail = []jen.Code{jen.True()}, []jen.Code{jen.False()}
		emptyDoc, failDoc = "returns true without calling SDL", "returns false"
	case ir.Int:
		t, err := g.typ(f.Sig.Ret)
		if err != nil {
			return
		}
		retT = t
		empty, fail = []jen.Code{jen.Lit(0)}, []jen.Code{jen.Lit(0)}
		emptyDoc, failDoc = "returns zero without calling SDL", "returns zero"
	}
	s := jen.Id(b)
	invoke := jen.Id(fnVar).Call(args...)
	var body []jen.Code
	byteLen := func() jen.Code {
		if typed {
			return jen.Id("n").Op(":=").Len(s).Op("*").Int().Call(jen.Qual("unsafe", "Sizeof").Call(jen.Op("*").Id("new").Call(jen.Id("T"))))
		}
		return jen.Id("n").Op(":=").Len(s)
	}
	if call {
		body = append(body, byteLen(), jen.Var().Id("ptr").Qual("unsafe", "Pointer"),
			jen.If(jen.Id("n").Op(">").Lit(0)).Block(jen.Id("ptr").Op("=").Qual("unsafe", "Pointer").Call(jen.Op("&").Id(b).Index(jen.Lit(0)))))
	} else {
		body = append(body, jen.If(jen.Len(s).Op("==").Lit(0)).Block(jen.Return(empty...)))
		if typed {
			body = append(body, byteLen())
		}
	}
	if !bf.wide {
		if typed || call {
			body = append(body, jen.If(jen.Op("!").Id("fitsInt32").Call(jen.Id("n"), jen.Lit(bytesName))).Block(jen.Return(fail...)))
		} else {
			body = append(body, jen.If(jen.Op("!").Id("fitsInt32").Call(jen.Len(s), jen.Lit(bytesName))).Block(jen.Return(fail...)))
		}
	}
	if retT == nil {
		body = append(body, invoke, jen.Qual("runtime", "KeepAlive").Call(s))
	} else {
		body = append(body, jen.Id("ret").Op(":=").Add(invoke), jen.Qual("runtime", "KeepAlive").Call(s), jen.Return(jen.Id("ret")))
	}
	what := "[]byte form"
	if typed {
		what = "typed-slice form"
	}
	doc := []string{
		bytesName + " is the " + what + " of " + name + ": it takes " + b + " instead of the " + ps[bf.ptr].Name + " pointer and derives the " + ps[bf.cnt].Name + " argument from its length.",
	}
	if typed {
		doc[0] += " The length is the size of the slice in bytes, so T must be a fixed-size numeric type (see Number) and its in-memory layout is what SDL sees."
	}
	if call {
		doc = append(doc, "", "An empty or nil slice is passed to SDL as a NULL pointer with a zero length.")
	} else {
		doc = append(doc, "", "An empty or nil slice is a no-op that "+emptyDoc+".")
	}
	if !bf.wide {
		doc[len(doc)-1] += " A slice longer than MaxInt32 bytes sets the SDL error and " + failDoc + "."
	}
	if !ps[bf.ptr].Type.Resolve().Const {
		if ret.Kind == ir.Int {
			doc = append(doc, "", "SDL fills "+b+" in place; the result is the number of bytes it wrote or read, as for "+name+".")
		} else {
			doc = append(doc, "", "SDL fills "+b+" in place.")
		}
	}
	doc = append(doc, "", "See also: "+name)
	w := start(doc).Func().Id(bytesName)
	if typed {
		w = w.Types(jen.Id("T").Id("Number"))
	}
	w = w.Params(params...)
	if retT != nil {
		w = w.Add(retT)
	}
	fb.funcs = append(fb.funcs, w.Block(body...))
}
