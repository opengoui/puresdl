package bindgen

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

var (
	synthMode = &ir.Record{Tag: "SDL_DisplayMode", Name: "SDL_DisplayMode", Size: 4, Align: 4, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "w", Type: &ir.Type{Kind: ir.Int, Size: 4, Signed: true}}}}
	synthModeType = &ir.Type{Kind: ir.Named, Name: "SDL_DisplayMode", Under: &ir.Type{Kind: ir.Struct, Rec: synthMode}}
)

// synthFile generates a package from fns (plus the SDL_DisplayMode record)
// and returns the source of the file that holds the wrappers.
func synthFile(t *testing.T, fns ...*ir.Function) string {
	t.Helper()
	return synthFileConsts(t, nil, fns...)
}

// synthFileConsts is synthFile with extra macro constants in the API.
func synthFileConsts(t *testing.T, consts []*ir.Const, fns ...*ir.Function) string {
	t.Helper()
	mode := synthMode
	td := &ir.Typedef{Name: "SDL_DisplayMode", File: "SDL_synth.h", Type: synthModeType}
	for _, f := range fns {
		f.File, f.Targets = "SDL_synth.h", []string{"linux", "darwin", "windows"}
	}
	api := &ir.API{Funcs: fns, Typedefs: []*ir.Typedef{td}, Records: []*ir.Record{mode}, Consts: consts, Version: "0.0.0"}
	g := &gen{
		api: api, pkg: "sdl",
		docNames: map[string]string{}, claimed: map[string]string{}, tdGo: map[string]string{},
		typedefs: map[string]*ir.Typedef{}, recName: map[*ir.Record]string{}, enumName: map[*ir.EnumDecl]string{},
		files: map[string]*fileBuf{}, handNames: map[string]bool{},
	}
	g.index()
	g.emitTypes()
	g.emitFuncs()
	if len(g.errs) > 0 {
		t.Fatal(g.errs)
	}
	f := g.newFile()
	for _, c := range g.file("SDL_synth.h").funcs {
		f.Add(c)
	}
	var buf bytes.Buffer
	if err := f.Render(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func ptrTo(e *ir.Type) *ir.Type { return &ir.Type{Kind: ir.Pointer, Size: 8, Elem: e} }

func TestGeneratedSliceRules(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	cchar := &ir.Type{Kind: ir.Int, Size: 1, Signed: true, Char: true, Const: true}
	count := ir.Param{Name: "count", Type: ptrTo(i32)}
	free := "The caller should free the array with SDL_free()."
	fn := func(name string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Doc: free, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	src := synthFile(t,
		fn("SDL_GetInts", ptrTo(i32), count),
		fn("SDL_GetNames", ptrTo(ptrTo(cchar)), count),
		fn("SDL_GetNameList", ptrTo(ptrTo(cchar))),
		fn("SDL_GetBlob", ptrTo(&ir.Type{Kind: ir.Void}), ir.Param{Name: "size", Type: ptrTo(i32)}),
		fn("SDL_GetModes", ptrTo(ptrTo(synthModeType)), count),
		fn("SDL_GetPlainPtrs", ptrTo(ptrTo(i32)), count),
	)
	sigs := funcSigs(t, src)
	for name, want := range map[string]string{
		"GetInts":     "func() []int32",
		"GetNames":    "func() []string",
		"GetNameList": "func() []string",
		"GetBlob":     "func() []byte",
		// SDL_DisplayMode is in deepCopy: the pointers share the array's
		// allocation, so the structs are copied by value.
		"GetModes":     "func() []DisplayMode",
		"GetPlainPtrs": "func() []*int32",
	} {
		if got := sigs[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestGeneratedKeepAliveForStoredPointers(t *testing.T) {
	ptr := ptrTo(&ir.Type{Kind: ir.Void})
	mk := func(name string, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Sig: &ir.Sig{Params: params}}
	}
	for _, tc := range []struct {
		f    *ir.Function
		want bool
	}{
		{mk("SDL_AddTimerLike", ir.Param{Name: "userdata", Type: ptr}), true},
		{mk("SDL_SetPointerProperty", ir.Param{Name: "value", Type: ptr}), true},
		{mk("SDL_SetTLS", ir.Param{Name: "value", Type: ptr}), true},
		{mk("SDL_EnumerateThings", ir.Param{Name: "userdata", Type: ptr}), false},
		{mk("SDL_RemoveEventWatch", ir.Param{Name: "userdata", Type: ptr}), false},
		{mk("SDL_Other", ir.Param{Name: "data", Type: ptr}), false},
	} {
		got := keepAliveNote(tc.f)
		if (got != "") != tc.want || (tc.want && !strings.HasPrefix(got, "Keep alive:")) {
			t.Errorf("keepAliveNote(%s) = %q, want note: %v", tc.f.Name, got, tc.want)
		}
	}
}

func TestGeneratedHandWrittenWrappersAreSkipped(t *testing.T) {
	cchar := &ir.Type{Kind: ir.Int, Size: 1, Signed: true, Char: true, Const: true}
	src := synthFile(t, &ir.Function{Name: "SDL_SetScancodeName", Sig: &ir.Sig{
		Ret:    &ir.Type{Kind: ir.Bool},
		Params: []ir.Param{{Name: "scancode", Type: &ir.Type{Kind: ir.Int, Size: 4}}, {Name: "name", Type: ptrTo(cchar)}}}})
	if _, found := funcSigs(t, src)["SetScancodeName"]; found {
		t.Error("hand-written wrapper must not be generated")
	}
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw := false
	for _, d := range file.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			for _, sp := range gd.Specs {
				for _, n := range sp.(*ast.ValueSpec).Names {
					raw = raw || n.Name == "fnSetScancodeName"
				}
			}
		}
	}
	if !raw {
		t.Error("raw function variable missing")
	}
}

func TestCallbackNullConstant(t *testing.T) {
	cb := &ir.Type{Kind: ir.Named, Name: "SDL_FooCallback", Under: ptrTo(&ir.Type{Kind: ir.Func, Sig: &ir.Sig{
		Ret: &ir.Type{Kind: ir.Void}, Params: []ir.Param{{Name: "x", Type: &ir.Type{Kind: ir.Int, Size: 4, Signed: true}}}}})}
	td := &ir.Typedef{Name: "SDL_FooCallback", File: "SDL_synth.h", Doc: "A callback.", Type: cb}
	api := &ir.API{Typedefs: []*ir.Typedef{td}, Version: "0.0.0"}
	g := &gen{
		api: api, pkg: "sdl",
		docNames: map[string]string{}, claimed: map[string]string{}, tdGo: map[string]string{},
		typedefs: map[string]*ir.Typedef{}, recName: map[*ir.Record]string{}, enumName: map[*ir.EnumDecl]string{},
		files: map[string]*fileBuf{}, handNames: map[string]bool{},
	}
	g.index()
	g.emitTypes()
	if len(g.errs) > 0 {
		t.Fatal(g.errs)
	}
	f := g.newFile()
	for _, c := range g.file("SDL_synth.h").types {
		f.Add(c)
	}
	var buf bytes.Buffer
	if err := f.Render(&buf); err != nil {
		t.Fatal(err)
	}
	pf, err := parser.ParseFile(token.NewFileSet(), "x.go", buf.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range pf.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			typ, _ := vs.Type.(*ast.Ident)
			val, _ := vs.Values[0].(*ast.BasicLit)
			if vs.Names[0].Name == "NoFooCallback" && typ != nil && typ.Name == "FooCallback" && val != nil && val.Value == "0" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no `const NoFooCallback FooCallback = 0` in:\n%s", buf.String())
	}
}

func TestAsyncIOLifetimeNotes(t *testing.T) {
	void := ptrTo(&ir.Type{Kind: ir.Void})
	u8pp := ptrTo(ptrTo(&ir.Type{Kind: ir.Int, Size: 1}))
	read := &ir.Function{
		Name: "SDL_ReadAsyncIO",
		Doc:  "Start an async read.\n\n`ptr` must remain available until the work is done, and may be accessed by the system at any time until then.",
		Sig:  &ir.Sig{Params: []ir.Param{{Name: "ptr", Type: void}, {Name: "userdata", Type: void}}},
	}
	got := keepAliveNote(read)
	for _, want := range []string{"Keep alive: ptr must stay alive and unmoved until the task completes", "NewUserdataHandle", "UserdataHandleValue"} {
		if !strings.Contains(got, want) {
			t.Errorf("ReadAsyncIO note %q lacks %q", got, want)
		}
	}
	// Without the doc text there is no ptr rule, only the userdata one.
	bare := &ir.Function{Name: "SDL_WriteAsyncIO", Sig: read.Sig}
	if got := keepAliveNote(bare); strings.Contains(got, "unmoved") || !strings.Contains(got, "NewUserdataHandle") {
		t.Errorf("WriteAsyncIO without ptr doc: %q", got)
	}
	// Other userdata functions do not mention the async helpers.
	timer := &ir.Function{Name: "SDL_AddTimerLike", Sig: &ir.Sig{Params: []ir.Param{{Name: "userdata", Type: void}}}}
	if got := keepAliveNote(timer); got == "" || strings.Contains(got, "NewUserdataHandle") {
		t.Errorf("timer note %q", got)
	}

	load := &ir.Function{
		Name: "SDL_LoadThing",
		Doc:  "When done, call SDL_free() on the buffer.",
		Sig:  &ir.Sig{Params: []ir.Param{{Name: "buf", Type: u8pp}}},
	}
	if got := freedBufferNote(load); !strings.Contains(got, "must be released with Free") {
		t.Errorf("freedBufferNote = %q", got)
	}
	load.Doc = "Loads a thing."
	if got := freedBufferNote(load); got != "" {
		t.Errorf("no SDL_free in the doc, got %q", got)
	}

	src := synthFile(t, &ir.Function{Name: "SDL_CloseAsyncIO", Sig: &ir.Sig{Ret: &ir.Type{Kind: ir.Bool}, Params: []ir.Param{{Name: "userdata", Type: void}}}})
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var doc string
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "CloseAsyncIO" {
			doc = fd.Doc.Text()
		}
	}
	if !strings.Contains(doc, "Keep alive: SDL stores the userdata argument") || !strings.Contains(doc, "UserdataHandleValue") {
		t.Errorf("CloseAsyncIO doc:\n%s", doc)
	}
}

func TestPtrFieldAccessors(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	cchar := &ir.Type{Kind: ir.Int, Size: 1, Signed: true, Char: true, Const: true}
	color := &ir.Record{Tag: "SDL_Color", Name: "SDL_Color", Size: 4, Align: 4, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "r", Type: i32}}}
	colorT := &ir.Type{Kind: ir.Named, Name: "SDL_Color", Under: &ir.Type{Kind: ir.Struct, Rec: color}}
	pal := &ir.Record{Tag: "SDL_Palette", Name: "SDL_Palette", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "ncolors", Type: i32}, {Name: "colors", Type: ptrTo(colorT), Offset: 8}}}
	drop := &ir.Record{Tag: "SDL_DropEvent", Name: "SDL_DropEvent", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "type", Type: i32}, {Name: "data", Type: ptrTo(cchar), Offset: 8}}}
	u64 := &ir.Type{Kind: ir.Int, Size: 8}
	void := &ir.Type{Kind: ir.Void}
	outcome := &ir.Record{Tag: "SDL_AsyncIOOutcome", Name: "SDL_AsyncIOOutcome", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "buffer", Type: ptrTo(void)}, {Name: "bytes_transferred", Type: u64, Offset: 8}}}
	surface := &ir.Record{Tag: "SDL_Surface", Name: "SDL_Surface", Size: 24, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "h", Type: i32}, {Name: "pitch", Type: i32, Offset: 4}, {Name: "pixels", Type: ptrTo(void), Offset: 8}}}
	textInput := &ir.Record{Tag: "SDL_TextInputEvent", Name: "SDL_TextInputEvent", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "type", Type: i32}, {Name: "text", Type: ptrTo(cchar), Offset: 8}}}
	textEditing := &ir.Record{Tag: "SDL_TextEditingEvent", Name: "SDL_TextEditingEvent", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "type", Type: i32}, {Name: "text", Type: ptrTo(cchar), Offset: 8}}}
	// A pointer field of an event that is not a C string gets no accessor.
	keyEvent := &ir.Record{Tag: "SDL_KeyEvent", Name: "SDL_KeyEvent", Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "type", Type: i32}, {Name: "extra", Type: ptrTo(i32), Offset: 8}}}
	other := &ir.Record{Tag: "SDL_Other", Name: "SDL_Other", Size: 8, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "name", Type: ptrTo(cchar)}}}
	ev := &ir.Record{Tag: "SDL_Event", Name: "SDL_Event", Union: true, Size: 16, Align: 8, File: "SDL_synth.h",
		Fields: []ir.Field{{Name: "drop", Type: &ir.Type{Kind: ir.Struct, Rec: drop}},
			{Name: "text", Type: &ir.Type{Kind: ir.Struct, Rec: textInput}},
			{Name: "edit", Type: &ir.Type{Kind: ir.Struct, Rec: textEditing}},
			{Name: "key", Type: &ir.Type{Kind: ir.Struct, Rec: keyEvent}}}}
	recs := []*ir.Record{color, pal, drop, textInput, textEditing, keyEvent, outcome, surface, other, ev}
	var tds []*ir.Typedef
	for _, r := range recs {
		tds = append(tds, &ir.Typedef{Name: r.Name, File: "SDL_synth.h", Type: &ir.Type{Kind: ir.Named, Name: r.Name, Under: &ir.Type{Kind: ir.Struct, Rec: r}}})
	}
	g := &gen{
		api: &ir.API{Typedefs: tds, Records: recs, Version: "0.0.0"}, pkg: "sdl",
		docNames: map[string]string{}, claimed: map[string]string{}, tdGo: map[string]string{},
		typedefs: map[string]*ir.Typedef{}, recName: map[*ir.Record]string{}, enumName: map[*ir.EnumDecl]string{},
		files: map[string]*fileBuf{}, handNames: map[string]bool{},
	}
	g.index()
	g.emitTypes()
	f := g.newFile()
	for _, c := range g.file("SDL_synth.h").types {
		f.Add(c)
	}
	var buf bytes.Buffer
	if err := f.Render(&buf); err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "synth.go", buf.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil {
			got[fd.Recv.List[0].Type.(*ast.StarExpr).X.(*ast.Ident).Name+"."+fd.Name.Name] = types.ExprString(fd.Type.Results.List[0].Type)
		}
	}
	for _, union := range []string{"Event.Drop", "Event.Text", "Event.Edit", "Event.Key"} {
		delete(got, union) // union accessors, not a pointer-field rule
	}
	want := map[string]string{
		"Palette.ColorSlice":          "[]Color",
		"DropEvent.DataString":        "string",
		"AsyncIOOutcome.Bytes":        "[]byte",
		"Surface.PixelBytes":          "[]byte",
		"TextInputEvent.TextString":   "string",
		"TextEditingEvent.TextString": "string",
	}
	if len(got) != len(want) {
		t.Fatalf("methods = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s returns %q, want %q", k, got[k], v)
		}
	}
}
