package bindgen

import (
	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// countedPtr pairs a pointer field of a struct with the sibling integer field
// that holds its element count. The headers do not state the pairing, so it is
// listed here by hand.
type countedPtr struct {
	ptr, count string // C field names
	method     string // Go method name
	owner      string // doc text: when the memory stops being valid
	extra      []string
	// stride, when set, names a second integer field: the slice length is
	// count*stride bytes (a pixel buffer is h rows of pitch bytes).
	stride string
}

// countedPtrs lists the counted pointer fields by C record name.
var countedPtrs = map[string]countedPtr{
	"SDL_Palette": {
		ptr: "colors", count: "ncolors", method: "ColorSlice",
		owner: "DestroyPalette",
		extra: []string{"Do not write to it directly; use SetPaletteColors.", "Copy it (for example with slices.Clone) if it must outlive the palette."},
	},
	"SDL_AsyncIOOutcome": {
		ptr: "buffer", count: "bytes_transferred", method: "Bytes",
		owner: "its backing buffer is released",
		extra: []string{"For a LoadFileAsync result that is when Free is called on Buffer; for ReadAsyncIO or WriteAsyncIO it is when the caller's own buffer is released."},
	},
	"SDL_Surface": {
		ptr: "pixels", count: "h", stride: "pitch", method: "PixelBytes",
		owner: "the surface is destroyed or resized",
		extra: []string{"The slice covers Pitch*H bytes, including row padding.", "A surface that needs locking (MUSTLOCK) must be locked with LockSurface while it is used."},
	},
}

// eventRecords returns the structs that are members of the SDL_Event union.
func (g *gen) eventRecords() map[*ir.Record]bool {
	out := map[*ir.Record]bool{}
	for r := range g.recName {
		if !r.Union || (r.Name != "SDL_Event" && r.Tag != "SDL_Event") {
			continue
		}
		for _, f := range r.Fields {
			if ft := f.Type.Resolve(); ft != nil && ft.Kind == ir.Struct {
				out[ft.Rec] = true
			}
		}
	}
	return out
}

// isCharPtr reports whether t is a pointer to a char (a C string).
func isCharPtr(t *ir.Type) bool {
	r := t.Resolve()
	if r == nil || r.Kind != ir.Pointer {
		return false
	}
	e := r.Elem.Resolve()
	return e != nil && e.Kind == ir.Int && e.Char
}

// emitPtrAccessors adds the methods that view raw pointer fields of r safely:
// a slice for a counted pointer and a copying string for a C string field of
// an event.
func (g *gen) emitPtrAccessors(r *ir.Record, n string, fb *fileBuf) {
	if r.Union || r.Incomplete {
		return
	}
	cname := r.Name
	if cname == "" {
		cname = r.Tag
	}
	fields := map[string]ir.Field{}
	goField := map[string]string{}
	for i, f := range r.Fields {
		fields[f.Name] = f
		nm := f.Name
		if nm == "" {
			nm = "anon" + string(rune('0'+i%10))
		}
		goField[f.Name] = fieldName(nm)
	}
	recv := jen.Id("p").Op("*").Id(n)
	if cp, ok := countedPtrs[cname]; ok {
		pf, okp := fields[cp.ptr]
		cf, okc := fields[cp.count]
		if okp && okc && cf.Type.Resolve().Kind == ir.Int && g.claim(n+"."+cp.method, cname+"."+cp.ptr) {
			elem := jen.Byte()
			if e := pf.Type.Resolve().Elem.Resolve(); e.Kind != ir.Void {
				t, err := g.typ(pf.Type.Resolve().Elem)
				if err != nil {
					g.skip(n+"."+cp.method, err.Error())
					return
				}
				elem = t
			}
			pn, cn := goField[cp.ptr], goField[cp.count]
			ptr := jen.Id("p").Dot(pn)
			if pf.Type.Resolve().Elem.Resolve().Kind == ir.Void {
				ptr = jen.Parens(jen.Op("*").Byte()).Call(ptr)
			}
			length := jen.Int().Call(jen.Id("p").Dot(cn))
			guard := jen.Id("p").Dot(cn).Op("<=").Lit(0)
			what, bad := cn+" elements", "a non-positive "+cn
			if cp.stride != "" {
				sf, oks := fields[cp.stride]
				if !oks || sf.Type.Resolve().Kind != ir.Int {
					g.skip(n+"."+cp.method, "stride field "+cp.stride+" is not an integer")
					return
				}
				sn := goField[cp.stride]
				length = jen.Int().Call(jen.Id("p").Dot(cn)).Op("*").Int().Call(jen.Id("p").Dot(sn))
				guard = guard.Op("||").Id("p").Dot(sn).Op("<=").Lit(0)
				what, bad = cn+"*"+sn+" bytes", "a non-positive "+cn+" or "+sn
			}
			doc := []string{
				cp.method + " returns " + pn + " as a slice of " + what + " without copying. It",
				"returns nil for a nil receiver, a nil " + pn + " pointer or " + bad + ".",
				"",
				"The slice aliases memory owned by SDL: do not retain it after " + cp.owner + ".",
			}
			doc = append(doc, cp.extra...)
			fb.types = append(fb.types, start(doc).Func().Params(recv).Id(cp.method).Params().Index().Add(elem).Block(
				jen.If(jen.Id("p").Op("==").Nil().Op("||").Id("p").Dot(pn).Op("==").Nil().Op("||").Add(guard)).Block(jen.Return(jen.Nil())),
				jen.Return(jen.Qual("unsafe", "Slice").Call(ptr, length)),
			))
		}
	}
	if !g.eventRecords()[r] {
		return
	}
	for _, f := range r.Fields {
		if f.Name == "" || !isCharPtr(f.Type) {
			continue
		}
		fn := goField[f.Name]
		if t, err := g.fieldType(n, f); err != nil || t.GoString() != "*byte" {
			continue
		}
		m := fn + "String"
		if !g.claim(n+"."+m, cname+"."+f.Name) {
			continue
		}
		doc := []string{
			m + " returns a copy of the NUL-terminated " + fn + " string, or \"\" for a nil receiver",
			"or a NULL pointer.",
			"",
			"SDL owns the " + fn + " memory and it is valid only while the event is being handled;",
			"the returned string is an independent Go copy and stays valid.",
		}
		fb.types = append(fb.types, start(doc).Func().Params(recv).Id(m).Params().String().Block(
			jen.If(jen.Id("p").Op("==").Nil()).Block(jen.Return(jen.Lit(""))),
			jen.Return(jen.Id("GoString").Call(jen.Id("p").Dot(fn))),
		))
	}
}
