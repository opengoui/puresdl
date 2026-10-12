package bindgen

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

func base(path string) string { return filepath.Base(path) }

// index assigns Go names to every type the package declares.
func (g *gen) index() {
	for _, td := range g.api.Typedefs {
		g.typedefs[td.Name] = td
	}
	// typedef-named types
	for _, td := range g.api.Typedefs {
		if td.Std || unsupportedNames[td.Name] {
			continue
		}
		if _, ok := builtinNames[td.Name]; ok {
			continue
		}
		n := goName(td.Name)
		if !g.claim(n, td.Name) {
			continue
		}
		g.tdGo[td.Name] = n
		res := td.Type.Under.Resolve()
		switch res.Kind {
		case ir.Struct, ir.Union:
			if res.Rec.Name == td.Name {
				g.recName[res.Rec] = n
			}
		case ir.Enum:
			if res.Enum.Name == td.Name {
				g.enumName[res.Enum] = n
			}
		}
	}
	// tag-named records and enums without a typedef
	for _, r := range g.api.Records {
		if _, ok := g.recName[r]; ok || r.Tag == "" || !hasPrefix(r.Tag, "SDL_") {
			continue
		}
		if g.claim(goName(r.Tag), r.Tag) {
			g.recName[r] = goName(r.Tag)
		}
	}
	for _, e := range g.api.Enums {
		if _, ok := g.enumName[e]; ok || e.Tag == "" || !hasPrefix(e.Tag, "SDL_") {
			continue
		}
		if g.claim(goName(e.Tag), e.Tag) {
			g.enumName[e] = goName(e.Tag)
		}
	}
	// anonymous nested records are named after their parent field
	for changed := true; changed; {
		changed = false
		for _, r := range g.sortedRecords() {
			pn := g.recName[r]
			for i, f := range r.Fields {
				ft := f.Type.Resolve()
				for ft != nil && ft.Kind == ir.Array {
					ft = ft.Elem.Resolve()
				}
				if ft == nil || (ft.Kind != ir.Struct && ft.Kind != ir.Union) {
					continue
				}
				if _, ok := g.recName[ft.Rec]; ok {
					continue
				}
				fn := f.Name
				if fn == "" {
					fn = fmt.Sprintf("anon%d", i)
				}
				n := pn + "_" + fieldName(fn)
				if g.claim(n, pn+"."+fn) {
					g.recName[ft.Rec] = n
					changed = true
				}
			}
		}
	}
	for _, f := range g.api.Funcs {
		g.docNames[f.Name] = goName(f.Name)
	}
	for _, k := range g.api.Consts {
		g.docNames[k.Name] = goName(k.Name)
	}
	for _, e := range g.api.Enums {
		for _, m := range e.Members {
			g.docNames[m.Name] = goName(m.Name)
		}
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func (g *gen) sortedRecords() []*ir.Record {
	var rs []*ir.Record
	for r := range g.recName {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return g.recName[rs[i]] < g.recName[rs[j]] })
	return rs
}

// recFile finds the header a record belongs to.
func (g *gen) recFile(r *ir.Record) string {
	if td, ok := g.typedefs[r.Name]; ok && r.Name != "" {
		return base(td.File)
	}
	if r.File != "" {
		return base(r.File)
	}
	if r.Tag != "" {
		if td, ok := g.typedefs[r.Tag]; ok {
			return base(td.File)
		}
	}
	return ""
}

func (g *gen) emitTypes() {
	// parents give anonymous records their header
	parentFile := map[*ir.Record]string{}
	for _, r := range g.sortedRecords() {
		f := g.recFile(r)
		if f == "" {
			continue
		}
		for _, fl := range r.Fields {
			ft := fl.Type.Resolve()
			for ft != nil && ft.Kind == ir.Array {
				ft = ft.Elem.Resolve()
			}
			if ft != nil && (ft.Kind == ir.Struct || ft.Kind == ir.Union) {
				if _, ok := parentFile[ft.Rec]; !ok {
					parentFile[ft.Rec] = f
				}
			}
		}
	}
	for _, r := range g.sortedRecords() {
		f := g.recFile(r)
		if f == "" {
			f = parentFile[r]
		}
		if f == "" {
			f = "SDL_stdinc.h"
		}
		g.emitRecord(r, g.recName[r], g.file(f))
	}
	var tds []*ir.Typedef
	for _, td := range g.api.Typedefs {
		if _, ok := g.tdGo[td.Name]; ok {
			tds = append(tds, td)
		}
	}
	sort.Slice(tds, func(i, j int) bool { return tds[i].Name < tds[j].Name })
	for _, td := range tds {
		g.emitTypedef(td, g.file(base(td.File)))
	}
}

func (g *gen) emitTypedef(td *ir.Typedef, fb *fileBuf) {
	n := g.tdGo[td.Name]
	doc := g.docText(td.Doc)
	under := td.Type.Under
	res := under.Resolve()
	switch res.Kind {
	case ir.Struct, ir.Union:
		if g.recName[res.Rec] == n {
			return
		}
		if rn, ok := g.recName[res.Rec]; ok {
			fb.types = append(fb.types, start(doc).Type().Id(n).Op("=").Id(rn))
			return
		}
	case ir.Enum:
		if g.enumName[res.Enum] == n {
			return
		}
		if en, ok := g.enumName[res.Enum]; ok {
			fb.types = append(fb.types, start(doc).Type().Id(n).Op("=").Id(en))
			return
		}
	case ir.Pointer:
		if isFuncPtr(res) {
			g.emitCallback(td, n, doc, res.Elem.Resolve().Sig, fb)
			return
		}
		fb.types = append(fb.types, start(doc).Type().Id(n).Qual("unsafe", "Pointer"))
		return
	case ir.Int, ir.Bool, ir.Float:
		b, _ := basicName(res)
		fb.types = append(fb.types, start(doc).Type().Id(n).Id(b))
		return
	}
	g.skip(td.Name, "typedef kind not supported")
	delete(g.tdGo, td.Name)
}

// emitCallback declares a C function pointer type and a constructor that
// wraps a Go function with purego.NewCallback.
func (g *gen) emitCallback(td *ir.Typedef, n string, doc []string, sig *ir.Sig, fb *fileBuf) {
	doc = append(doc, "", "It holds a C function pointer; create one with New"+n+".")
	fb.types = append(fb.types, start(doc).Type().Id(n).Id("uintptr"))
	var params []jen.Code
	for i, p := range sig.Params {
		t, err := g.typ(p.Type)
		if err != nil {
			g.skip("New"+n, err.Error())
			return
		}
		params = append(params, jen.Id(paramName(p.Name, i)).Add(t))
	}
	fnType := jen.Func().Params(params...)
	if r := sig.Ret.Resolve(); r.Kind != ir.Void {
		t, err := g.typ(sig.Ret)
		if err != nil {
			g.skip("New"+n, err.Error())
			return
		}
		fnType = fnType.Add(t)
	}
	cdoc := []string{
		"New" + n + " wraps fn as a C callback of type " + n + ".",
		"",
		"The callback is allocated once and never freed, and purego supports at most",
		"about 2000 callbacks per process: create each callback once and reuse it.",
	}
	fb.types = append(fb.types, start(cdoc).Func().Id("New"+n).Params(jen.Id("fn").Add(fnType)).Id(n).Block(
		jen.Return(jen.Id(n).Call(jen.Qual("github.com/ebitengine/purego", "NewCallback").Call(jen.Id("fn")))),
	))
}

func storageType(size int64, align int) (jen.Code, int64) {
	var unit int64 = int64(align)
	if unit > 8 || unit < 1 {
		unit = 1
	}
	name := map[int64]string{1: "uint8", 2: "uint16", 4: "uint32", 8: "uint64"}[unit]
	return jen.Id(name), size / unit
}

func (g *gen) emitRecord(r *ir.Record, n string, fb *fileBuf) {
	doc := g.docText(r.Doc)
	if r.Incomplete {
		fb.types = append(fb.types, start(append(doc, "", "It is an opaque handle owned by SDL.")).Type().Id(n).Struct())
		return
	}
	if r.Union {
		g.emitUnion(r, n, doc, fb)
		return
	}
	var fields []jen.Code
	var lf []layoutField
	used := map[string]bool{}
	for i, f := range r.Fields {
		t, err := g.typ(f.Type)
		if err != nil {
			g.skip(n, "field "+f.Name+": "+err.Error()+" (emitted as raw storage)")
			unit, cnt := storageType(r.Size, r.Align)
			fb.types = append(fb.types, start(doc).Type().Id(n).Struct(jen.Id("_").Index(jen.Lit(int(cnt))).Add(unit)))
			g.layouts = append(g.layouts, layoutEntry{goName: n, size: r.Size, align: int64(r.Align)})
			return
		}
		fnm := f.Name
		if fnm == "" {
			fnm = fmt.Sprintf("anon%d", i)
		}
		name := fieldName(fnm)
		for used[name] {
			name += "_"
		}
		used[name] = true
		fields = append(fields, start(g.docText(f.Doc)).Id(name).Add(t))
		lf = append(lf, layoutField{name, f.Offset})
	}
	fb.types = append(fb.types, start(doc).Type().Id(n).Struct(fields...))
	g.layouts = append(g.layouts, layoutEntry{goName: n, size: r.Size, align: int64(r.Align), fields: lf})
}

// emitUnion declares a union as aligned raw storage with typed accessors.
func (g *gen) emitUnion(r *ir.Record, n string, doc []string, fb *fileBuf) {
	unit, cnt := storageType(r.Size, r.Align)
	doc = append(doc, "", "It is a C union: use the accessor methods to view the active member.")
	fb.types = append(fb.types, start(doc).Type().Id(n).Struct(jen.Id("data").Index(jen.Lit(int(cnt))).Add(unit)))
	g.layouts = append(g.layouts, layoutEntry{goName: n, size: r.Size, align: int64(r.Align), union: true})
	used := map[string]bool{}
	for i, f := range r.Fields {
		t, err := g.typ(f.Type)
		if err != nil {
			g.skip(n+"."+f.Name, err.Error())
			continue
		}
		fnm := f.Name
		if fnm == "" {
			fnm = fmt.Sprintf("anon%d", i)
		}
		name := fieldName(fnm)
		for used[name] {
			name += "_"
		}
		used[name] = true
		fd := g.docText(f.Doc)
		recv := jen.Id("u").Op("*").Id(n)
		ptr := jen.Parens(jen.Op("*").Add(t)).Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("u")))
		rt := f.Type.Resolve()
		if rt.Kind == ir.Struct || rt.Kind == ir.Union || rt.Kind == ir.Array {
			fd = append(fd, "", "The result aliases the union storage.")
			fb.types = append(fb.types, start(fd).Func().Params(recv).Id(name).Params().Op("*").Add(t).Block(
				jen.Return(ptr)))
			continue
		}
		fb.types = append(fb.types, start(fd).Func().Params(recv).Id(name).Params().Add(t).Block(
			jen.Return(jen.Op("*").Add(ptr))))
		fb.types = append(fb.types, start([]string{"Set" + name + " stores v in the " + f.Name + " member."}).Func().Params(recv).Id("Set"+name).Params(jen.Id("v").Add(t)).Block(
			jen.Op("*").Add(ptr).Op("=").Id("v")))
	}
}
