package bindgen

import (
	"bytes"
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
	mode := synthMode
	td := &ir.Typedef{Name: "SDL_DisplayMode", File: "SDL_synth.h", Type: synthModeType}
	for _, f := range fns {
		f.File, f.Targets = "SDL_synth.h", []string{"linux", "darwin", "windows"}
	}
	api := &ir.API{Funcs: fns, Typedefs: []*ir.Typedef{td}, Records: []*ir.Record{mode}, Version: "0.0.0"}
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
	for _, want := range []string{
		"func GetInts() []int32",
		"return copyFree(p, int(n))",
		"func GetNames() []string",
		"return copyFreeStrings(p, int(n))",
		"func GetNameList() []string",
		"return copyFreeStrings0(",
		"func GetBlob() []byte",
		"return copyFreeBytes(p, int(n))",
		"func GetPlainPtrs() []*int32",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated code lacks %q", want)
		}
	}
	// SDL_DisplayMode is in deepCopy: the pointers share the array's
	// allocation, so the structs are copied by value.
	if !strings.Contains(src, "func GetModes() []DisplayMode") || !strings.Contains(src, "copyFreeDeref(p, int(n))") {
		t.Errorf("deepCopy rule not applied:\n%s", src)
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
	if !strings.Contains(src, "var fnSetScancodeName func(") {
		t.Error("raw function variable missing")
	}
	if strings.Contains(src, "func SetScancodeName(") {
		t.Error("hand-written wrapper must not be generated")
	}
}
