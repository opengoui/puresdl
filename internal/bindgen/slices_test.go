package bindgen

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

// funcSigs parses src and returns every top-level function signature,
// rendered without the body, keyed by name.
func funcSigs(t *testing.T, src string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "gen.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		var sb strings.Builder
		if err := printer.Fprint(&sb, fset, fd.Type); err != nil {
			t.Fatal(err)
		}
		out[fd.Name.Name] = sb.String()
	}
	return out
}

func TestGeneratedSliceForms(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	constPtr := func(e *ir.Type) *ir.Type { p := ptrTo(e); p.Const = true; return p }
	ok := "Returns: true on success or false on failure."
	fn := func(name, doc string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Doc: doc, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	renderer := ir.Param{Name: "renderer", Type: ptrTo(i32)}
	items := ir.Param{Name: "items", Type: constPtr(synthModeType)}
	count := ir.Param{Name: "count", Type: i32}

	src := synthFile(t,
		fn("SDL_DrawItems", ok, boolT, renderer, items, count),
		fn("SDL_DrawItemsAfter", ok, boolT, renderer, items, ir.Param{Name: "numitems", Type: i32}, ir.Param{Name: "flags", Type: i32}),
		fn("SDL_CountInts", ok, i32, ir.Param{Name: "vals", Type: constPtr(i32)}, ir.Param{Name: "nvals", Type: i32}),
		fn("SDL_FillItems", "", void, ir.Param{Name: "items", Type: constPtr(synthModeType)}, ir.Param{Name: "nitems", Type: i32}),
		// Not matched:
		fn("SDL_GetItems", ok, boolT, renderer, ir.Param{Name: "items", Type: ptrTo(synthModeType)}, count),
		fn("SDL_PutBlob", ok, boolT, ir.Param{Name: "data", Type: constPtr(void)}, count),
		fn("SDL_SetFlags", ok, boolT, ir.Param{Name: "vals", Type: constPtr(i32)}, ir.Param{Name: "flags", Type: i32}),
		fn("SDL_ZipItems", ok, boolT, ir.Param{Name: "a", Type: constPtr(i32)}, items, count),
		fn("SDL_MaybeItems", ok, boolT, ir.Param{Name: "items", Type: constPtr(synthModeType), Doc: "the items, may be NULL."}, count),
		fn("SDL_TestPoints", "Returns: true if any item is inside.", boolT, items, count),
		fn("SDL_OptTwo", ok, boolT, renderer, items, count,
			ir.Param{Name: "indices", Type: constPtr(i32), Doc: "the indices, may be NULL."}, ir.Param{Name: "num_indices", Type: i32}),
		fn("SDL_RequiredTwo", ok, boolT, renderer, items, count,
			ir.Param{Name: "indices", Type: constPtr(i32), Doc: "the indices."}, ir.Param{Name: "num_indices", Type: i32}),
		fn("SDL_TwoArrays", ok, boolT, items, count, ir.Param{Name: "other", Type: constPtr(i32)}, ir.Param{Name: "num_other", Type: i32}),
	)
	sigs := funcSigs(t, src)

	want := map[string]string{
		"DrawItemsSlice":      "func(renderer *int32, items []DisplayMode) bool",
		"DrawItemsAfterSlice": "func(renderer *int32, items []DisplayMode, flags int32) bool",
		"CountIntsSlice":      "func(vals []int32) int32",
		"FillItemsSlice":      "func(items []DisplayMode)",
		// An optional second array becomes a second slice that may be empty.
		"OptTwoSlice": "func(renderer *int32, items []DisplayMode, indices []int32) bool",
	}
	for name, sig := range want {
		if got, found := sigs[name]; !found || got != sig {
			t.Errorf("%s = %q (present %v), want %q", name, got, found, sig)
		}
	}
	for _, name := range []string{"GetItemsSlice", "PutBlobSlice", "SetFlagsSlice", "ZipItemsSlice", "MaybeItemsSlice", "TestPointsSlice", "TwoArraysSlice", "RequiredTwoSlice"} {
		if _, found := sigs[name]; found {
			t.Errorf("%s must not be generated", name)
		}
	}
	// The raw wrappers keep the C signature.
	if got := sigs["DrawItems"]; got != "func(renderer *int32, items *DisplayMode, count int32) bool" {
		t.Errorf("DrawItems = %q", got)
	}
}

func TestGeneratedPlanarForm(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	boolT := &ir.Type{Kind: ir.Bool}
	voidPP := ptrTo(ptrTo(&ir.Type{Kind: ir.Void}))
	mk := func(name string, streamName string) *ir.Function {
		return &ir.Function{Name: name, Sig: &ir.Sig{Ret: boolT, Params: []ir.Param{
			{Name: streamName, Type: ptrTo(i32)},
			{Name: "channel_buffers", Type: voidPP},
			{Name: "num_channels", Type: i32},
			{Name: "num_samples", Type: i32},
		}}}
	}
	// The renamed stream parameter no longer matches the table entry.
	src := synthFile(t, mk("SDL_PutAudioStreamPlanarData", "stream"), mk("SDL_PutOtherPlanarData", "stream"))
	sigs := funcSigs(t, src)
	if got, want := sigs["PutAudioStreamPlanarDataSlices"], "func(stream *int32, planes [][]byte, numSamples int32) bool"; got != want {
		t.Errorf("PutAudioStreamPlanarDataSlices = %q, want %q", got, want)
	}
	if _, found := sigs["PutOtherPlanarDataSlices"]; found {
		t.Error("unlisted function must not get a planar form")
	}
	src = synthFile(t, mk("SDL_PutAudioStreamPlanarData", "s"))
	if _, found := funcSigs(t, src)["PutAudioStreamPlanarDataSlices"]; found {
		t.Error("planar form must not be generated when parameters no longer match the table")
	}
}

func TestGeneratedBytesForms(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	sizeT := &ir.Type{Kind: ir.Int, Size: 8}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	constPtr := func(e *ir.Type) *ir.Type { p := ptrTo(e); p.Const = true; return p }
	ok := "Returns: true on success or false on failure."
	fn := func(name, doc string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Doc: doc, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	stream := ir.Param{Name: "stream", Type: ptrTo(i32)}
	src := synthFile(t,
		fn("SDL_PutThing", ok, boolT, stream, ir.Param{Name: "buf", Type: constPtr(void)}, ir.Param{Name: "len", Type: i32}),
		fn("SDL_ReadThing", "", sizeT, stream, ir.Param{Name: "ptr", Type: ptrTo(void)}, ir.Param{Name: "size", Type: sizeT}),
		// Skipped: retained by SDL, excluded by name, non-void, non-size name.
		fn("SDL_PutThingNoCopy", ok, boolT, stream, ir.Param{Name: "buf", Type: constPtr(void)}, ir.Param{Name: "len", Type: i32}),
		fn("SDL_IOFromMem", "", sizeT, ir.Param{Name: "mem", Type: ptrTo(void)}, ir.Param{Name: "size", Type: sizeT}),
		fn("SDL_PutInts", ok, boolT, stream, ir.Param{Name: "buf", Type: constPtr(i32)}, ir.Param{Name: "len", Type: i32}),
		fn("SDL_PutFlags", ok, boolT, stream, ir.Param{Name: "buf", Type: constPtr(void)}, ir.Param{Name: "flags", Type: i32}),
		fn("SDL_KeepThing", "The buffer is not copied.", boolT, stream, ir.Param{Name: "buf", Type: constPtr(void)}, ir.Param{Name: "len", Type: i32}),
	)
	sigs := funcSigs(t, src)
	if got, want := sigs["PutThingBytes"], "func(stream *int32, b []byte) bool"; got != want {
		t.Errorf("PutThingBytes = %q, want %q", got, want)
	}
	if got, want := sigs["PutThingSlice"], "func[T Number](stream *int32, b []T) bool"; got != want {
		t.Errorf("PutThingSlice = %q, want %q", got, want)
	}
	if got, want := sigs["ReadThingSlice"], "func[T Number](stream *int32, b []T) uint64"; got != want {
		t.Errorf("ReadThingSlice = %q, want %q", got, want)
	}
	if got, want := sigs["ReadThingBytes"], "func(stream *int32, b []byte) uint64"; got != want {
		t.Errorf("ReadThingBytes = %q, want %q", got, want)
	}
	for _, name := range []string{"PutThingNoCopyBytes", "IOFromMemBytes", "PutIntsBytes", "PutFlagsBytes", "KeepThingBytes",
		"PutThingNoCopySlice", "IOFromMemSlice", "PutIntsSlice", "PutFlagsSlice", "KeepThingSlice"} {
		if _, found := sigs[name]; found {
			t.Errorf("%s must not be generated", name)
		}
	}
}

// TestGeneratedBytesFormUint64Length covers the storage functions: a uint64
// length named "length" and a "true if ... or false on failure" result.
func TestGeneratedBytesFormUint64Length(t *testing.T) {
	u64 := &ir.Type{Kind: ir.Int, Size: 8}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	doc := "Returns: true if it worked or false on failure."
	fn := func(name string, buf string) *ir.Function {
		return &ir.Function{Name: name, Doc: doc, Sig: &ir.Sig{Ret: boolT, Params: []ir.Param{
			{Name: "id", Type: ptrTo(u64)}, {Name: buf, Type: ptrTo(void)}, {Name: "length", Type: u64}}}}
	}
	src := synthFile(t, fn("SDL_ReadStorageFile", "destination"), fn("SDL_WriteStorageFile", "source"), fn("SDL_OtherFile", "source"))
	sigs := funcSigs(t, src)
	for _, name := range []string{"ReadStorageFile", "WriteStorageFile"} {
		if got, want := sigs[name+"Bytes"], "func(id *uint64, b []byte) bool"; got != want {
			t.Errorf("%sBytes = %q, want %q", name, got, want)
		}
		if got, want := sigs[name+"Slice"], "func[T Number](id *uint64, b []T) bool"; got != want {
			t.Errorf("%sSlice = %q, want %q", name, got, want)
		}
	}
	// Without the "true on success" wording and without a table entry the
	// result is not known to be a plain success flag.
	if _, found := sigs["OtherFileBytes"]; found {
		t.Error("OtherFileBytes must not be generated")
	}
}

func TestGeneratedTextureUpdateForms(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	u8 := &ir.Type{Kind: ir.Int, Size: 1}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	ok := "Returns: true on success or false on failure."
	fn := func(name string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Doc: ok, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	p := func(name string, t *ir.Type) ir.Param { return ir.Param{Name: name, Type: t} }
	tex, rect := p("texture", ptrTo(i32)), p("rect", ptrTo(i32))
	src := synthFile(t,
		fn("SDL_UpdateTexture", boolT, tex, rect, p("pixels", ptrTo(void)), p("pitch", i32)),
		fn("SDL_UpdateNVTexture", boolT, tex, rect, p("Yplane", ptrTo(u8)), p("Ypitch", i32), p("UVplane", ptrTo(u8)), p("UVpitch", i32)),
		// Renamed pitch parameter: the table entry no longer matches.
		fn("SDL_UpdateYUVTexture", boolT, tex, rect, p("Yplane", ptrTo(u8)), p("Ypitch", i32), p("Uplane", ptrTo(u8)), p("Upitch", i32), p("Vplane", ptrTo(u8)), p("stride", i32)),
	)
	sigs := funcSigs(t, src)
	if got, want := sigs["UpdateTextureBytes"], "func(texture *int32, rect *int32, pixels []byte, pitch int32) bool"; got != want {
		t.Errorf("UpdateTextureBytes = %q, want %q", got, want)
	}
	if got, want := sigs["UpdateNVTextureBytes"], "func(texture *int32, rect *int32, Yplane []byte, Ypitch int32, UVplane []byte, UVpitch int32) bool"; got != want {
		t.Errorf("UpdateNVTextureBytes = %q, want %q", got, want)
	}
	if _, found := sigs["UpdateYUVTextureBytes"]; found {
		t.Error("bytes form must not be generated when parameters no longer match the table")
	}
}
