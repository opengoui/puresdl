// Package bindgen turns the ir model of the SDL3 headers into the Go
// package that wraps libSDL3 through purego.
package bindgen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// Options configures Generate.
type Options struct {
	Package string // Go package name, default "sdl"
	OutDir  string
	// HandDir is the directory whose hand-written Go files name the extra
	// exported identifiers that docs may refer to; default OutDir.
	HandDir string
	// Tag and Revision, when set, name the SDL release (for example
	// "release-3.4.18") and its SDL_REVISION string; they are recorded in the
	// header comment of every generated file.
	Tag, Revision string
}

// Result reports what Generate produced.
type Result struct {
	Files   []string
	Skipped []string
	// TypedConsts is the number of macro constants emitted with a named
	// integer type instead of as untyped constants.
	TypedConsts int
}

// gen holds the state of one generation run: the input API, the Go names
// assigned so far and the output buffers per header.
type gen struct {
	api      *ir.API
	opt      Options
	pkg      string
	docNames map[string]string // C identifier -> Go identifier (for doc rewriting)
	claimed  map[string]string // Go identifier -> C identifier
	tdGo     map[string]string // typedef C name -> Go name (emitted typedefs and enums/records they name)
	typedefs map[string]*ir.Typedef
	recName  map[*ir.Record]string
	enumName map[*ir.EnumDecl]string
	files    map[string]*fileBuf
	skipped  []string
	binds    [][2]string // {Go func var, C symbol}
	layouts  []layoutEntry
	errs     []string
	// typedConsts counts the macro constants emitted with a typedef type.
	typedConsts int
	// handNames holds the exported identifiers of the hand-written files in
	// the output directory.
	handNames map[string]bool
	// docUnknown collects SDL_ names that docs mention but that have no Go
	// counterpart; skippedNames holds the C names the generator skipped.
	docUnknown   map[string]bool
	skippedNames map[string]bool
}

// fileBuf accumulates the generated declarations of one output file.
type fileBuf struct {
	types, consts, funcs []jen.Code
}

// layoutEntry is the C size, alignment and field offsets that the layout test
// checks for one generated struct.
type layoutEntry struct {
	goName      string
	size, align int64
	union       bool
	fields      []layoutField
}

// layoutField is the C offset of one field of a layoutEntry.
type layoutField struct {
	goName string
	offset int64
}

// file returns the buffer for the output file of header, creating it on first use.
func (g *gen) file(header string) *fileBuf {
	f := fileName(header)
	if b, ok := g.files[f]; ok {
		return b
	}
	b := &fileBuf{}
	g.files[f] = b
	return b
}

// skip records that the C declaration what was not bound, with the reason.
func (g *gen) skip(what, why string) {
	g.skipped = append(g.skipped, fmt.Sprintf("%s: %s", what, why))
	if g.skippedNames == nil {
		g.skippedNames = map[string]bool{}
	}
	g.skippedNames[what] = true
	if i := strings.IndexByte(what, '.'); i > 0 {
		g.skippedNames[what[:i]] = true
	}
}

// errorf records a generation error; Generate reports them all at the end.
func (g *gen) errorf(format string, a ...any) {
	g.errs = append(g.errs, fmt.Sprintf(format, a...))
}

// claim reserves a Go identifier for a C declaration.
func (g *gen) claim(goN, cN string) bool {
	if prev, ok := g.claimed[goN]; ok && prev != cN {
		g.skip(cN, "Go name "+goN+" already used by "+prev)
		return false
	}
	g.claimed[goN] = cN
	g.docNames[cN] = goN
	return true
}

// Generate writes the Go package for api into opt.OutDir.
func Generate(api *ir.API, opt Options) (*Result, error) {
	if opt.Package == "" {
		opt.Package = "sdl"
	}
	if opt.HandDir == "" {
		opt.HandDir = opt.OutDir
	}
	g := &gen{
		api: api, opt: opt, pkg: opt.Package,
		docNames: map[string]string{}, claimed: map[string]string{}, tdGo: map[string]string{},
		typedefs: map[string]*ir.Typedef{}, recName: map[*ir.Record]string{}, enumName: map[*ir.EnumDecl]string{},
		files: map[string]*fileBuf{}, handNames: loadHandNames(opt.HandDir),
	}
	for _, m := range CheckNullPolicy(api) {
		g.errorf("%s", m)
	}
	g.index()
	g.emitTypes()
	g.emitEnums()
	g.emitConsts()
	g.emitFuncs()
	g.checkDocNames()
	if len(g.errs) > 0 {
		return nil, fmt.Errorf("bindgen: %s", strings.Join(g.errs, "\n"))
	}
	res := &Result{}
	var names []string
	for n := range g.files {
		names = append(names, n)
	}
	sort.Strings(names)
	write := func(name string, f *jen.File) error {
		var buf bytes.Buffer
		if err := f.Render(&buf); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		src, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("format %s: %w", name, err)
		}
		res.Files = append(res.Files, name)
		return os.WriteFile(filepath.Join(opt.OutDir, name), src, 0o644)
	}
	for _, n := range names {
		b := g.files[n]
		f := g.newFile()
		for _, group := range [][]jen.Code{b.types, b.consts, b.funcs} {
			for _, c := range group {
				f.Add(c)
			}
		}
		if err := write(n+"_gen.go", f); err != nil {
			return nil, err
		}
	}
	if err := write("bind_gen.go", g.bindFile()); err != nil {
		return nil, err
	}
	if err := write("layout_gen_test.go", g.layoutFile()); err != nil {
		return nil, err
	}
	sort.Strings(g.skipped)
	res.Skipped = g.skipped
	res.TypedConsts = g.typedConsts
	return res, nil
}

// newFile returns an empty output file with the generated-code header and purego import name.
func (g *gen) newFile() *jen.File {
	f := jen.NewFile(g.pkg)
	f.HeaderComment(g.headerComment())
	f.ImportName("github.com/ebitengine/purego", "purego")
	return f
}

// bindFile builds the file that registers every bound C symbol with purego.
func (g *gen) bindFile() *jen.File {
	f := g.newFile()
	sort.Slice(g.binds, func(i, j int) bool { return g.binds[i][1] < g.binds[j][1] })
	var body []jen.Code
	for _, b := range g.binds {
		body = append(body, jen.Id("register").Call(jen.Op("&").Id(b[0]), jen.Lit(b[1])))
	}
	f.Comment("bindAll binds every wrapped C function. It runs after the library is loaded.")
	f.Func().Id("bindAll").Params().Block(body...)
	f.Comment("fitsInt32 reports whether n can be passed to SDL as an int32 count. When it")
	f.Comment("cannot, it sets the SDL error to a message naming fn and returns false.")
	f.Func().Id("fitsInt32").Params(jen.Id("n").Int(), jen.Id("fn").String()).Bool().Block(
		jen.If(jen.Id("n").Op(">").Qual("math", "MaxInt32")).Block(
			jen.Id("SetError").Call(jen.Lit("%s: length %d exceeds the int32 limit"), jen.Id("fn"), jen.Id("n")),
			jen.Return(jen.False()),
		),
		jen.Return(jen.True()),
	)
	f.Comment("Number is the set of fixed-size numeric element types accepted by the generic")
	f.Comment("slice forms of the (pointer, byte length) functions. None of them contains a")
	f.Comment("pointer, so SDL can safely read or fill the backing array.")
	f.Type().Id("Number").Interface(jen.Op("~").Int8().Op("|").Op("~").Uint8().Op("|").Op("~").Int16().Op("|").Op("~").Uint16().Op("|").Op("~").Int32().Op("|").Op("~").Uint32().Op("|").Op("~").Int64().Op("|").Op("~").Uint64().Op("|").Op("~").Float32().Op("|").Op("~").Float64())
	f.Comment("planarPtrs returns the address of the first byte of each plane, with nil for")
	f.Comment("an empty plane. The caller must keep planes alive across the C call.")
	f.Func().Id("planarPtrs").Params(jen.Id("planes").Index().Index().Byte()).Index().Qual("unsafe", "Pointer").Block(
		jen.Id("ptrs").Op(":=").Make(jen.Index().Qual("unsafe", "Pointer"), jen.Len(jen.Id("planes"))),
		jen.For(jen.Id("i").Op(",").Id("p").Op(":=").Range().Id("planes")).Block(
			jen.If(jen.Len(jen.Id("p")).Op(">").Lit(0)).Block(
				jen.Id("ptrs").Index(jen.Id("i")).Op("=").Qual("unsafe", "Pointer").Call(jen.Op("&").Id("p").Index(jen.Lit(0))))),
		jen.Return(jen.Id("ptrs")),
	)
	textureHelpers(f)
	var names []jen.Code
	for _, b := range g.binds {
		names = append(names, jen.Lit(b[1]))
	}
	f.Comment("allBindings lists the C symbols that bindAll registers.")
	f.Func().Id("allBindings").Params().Index().String().Block(jen.Return(jen.Index().String().Values(names...)))
	return f
}

// layoutFile builds the generated test that compares struct sizes, alignments
// and field offsets with the C layout.
func (g *gen) layoutFile() *jen.File {
	f := g.newFile()
	f.ImportName("unsafe", "unsafe")
	sort.Slice(g.layouts, func(i, j int) bool { return g.layouts[i].goName < g.layouts[j].goName })
	// The expected sizes come from a 64-bit C front end, so 32-bit targets skip.
	body := []jen.Code{
		jen.If(jen.Qual("unsafe", "Sizeof").Call(jen.Uintptr().Call(jen.Lit(0))).Op("!=").Lit(8)).Block(
			jen.Id("t").Dot("Skip").Call(jen.Lit("layout expectations are for 64-bit targets")),
		),
	}
	for _, l := range g.layouts {
		typ := jen.Id(l.goName).Values()
		body = append(body, jen.Id("checkLayout").Call(jen.Id("t"), jen.Lit(l.goName),
			jen.Qual("unsafe", "Sizeof").Call(typ), jen.Qual("unsafe", "Alignof").Call(typ), jen.Lit(int(l.size)), jen.Lit(int(l.align))))
		for _, fl := range l.fields {
			body = append(body, jen.Id("checkOffset").Call(jen.Id("t"), jen.Lit(l.goName+"."+fl.goName),
				jen.Qual("unsafe", "Offsetof").Call(jen.Id(l.goName).Values().Dot(fl.goName)), jen.Lit(int(fl.offset))))
		}
	}
	f.Comment("TestGeneratedLayout compares every generated struct with the layout the C")
	f.Comment("front end computed from the headers.")
	f.Func().Id("TestGeneratedLayout").Params(jen.Id("t").Op("*").Qual("testing", "T")).Block(body...)
	return f
}

// start begins a statement preceded by one // line per doc line.
func start(lines []string) *jen.Statement {
	s := &jen.Statement{}
	for _, l := range lines {
		s.Comment(l).Line()
	}
	return s
}

// headerComment returns the "Code generated ... DO NOT EDIT." line naming the SDL headers' version.
func (g *gen) headerComment() string {
	src := g.api.Version + " headers"
	switch {
	case g.opt.Tag != "" && g.opt.Revision != "":
		src += fmt.Sprintf(" (%s, revision %s)", g.opt.Tag, g.opt.Revision)
	case g.opt.Tag != "":
		src += fmt.Sprintf(" (%s)", g.opt.Tag)
	}
	return fmt.Sprintf("Code generated by gensdl from the SDL %s; DO NOT EDIT.", src)
}
