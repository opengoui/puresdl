package bindgen

import (
	"math/big"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

func TestGeneratedRecipes(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	u32 := &ir.Type{Kind: ir.Int, Size: 4}
	u8 := &ir.Type{Kind: ir.Int, Size: 1}
	sizeT := &ir.Type{Kind: ir.Int, Size: 8}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	stream := ptrTo(synthModeType)
	fn := func(name, doc string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Doc: doc, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	free := "Call SDL_free() on the buffer."
	loadParams := func(extra ...ir.Param) []ir.Param {
		ps := []ir.Param{{Name: "src", Type: stream}, {Name: "closeio", Type: boolT}}
		ps = append(ps, extra...)
		return append(ps, ir.Param{Name: "audio_buf", Type: ptrTo(ptrTo(u8))}, ir.Param{Name: "audio_len", Type: ptrTo(u32)})
	}
	spec := ir.Param{Name: "spec", Type: ptrTo(synthModeType)}
	consts := []*ir.Const{{Name: "SDL_IO_SEEK_SET", Kind: ir.ConstInt, Int: big.NewInt(0), File: "SDL_synth.h"}}
	funcs := func(more ...*ir.Function) []*ir.Function {
		return append([]*ir.Function{
			fn("SDL_IOFromConstMem", "", stream, ir.Param{Name: "mem", Type: ptrTo(void)}, ir.Param{Name: "size", Type: sizeT}),
			fn("SDL_IOFromDynamicMem", "", stream),
			fn("SDL_WriteIO", "", sizeT, ir.Param{Name: "context", Type: stream}, ir.Param{Name: "ptr", Type: ptrTo(void)}, ir.Param{Name: "size", Type: sizeT}),
			fn("SDL_SeekIO", "", sizeT, ir.Param{Name: "context", Type: stream}, ir.Param{Name: "offset", Type: sizeT}, ir.Param{Name: "whence", Type: i32}),
			fn("SDL_CloseIO", "", boolT, ir.Param{Name: "context", Type: stream}),
		}, more...)
	}

	sigs := funcSigs(t, synthFileConsts(t, consts, funcs(
		fn("SDL_LoadThing_IO", free, boolT, loadParams(spec)...),
		// Not matched: no free documentation, no closeio, a buffer of the wrong type.
		fn("SDL_LoadPlain_IO", "", boolT, loadParams(spec)...),
		fn("SDL_LoadNoClose_IO", free, boolT, ir.Param{Name: "src", Type: stream}, ir.Param{Name: "audio_buf", Type: ptrTo(ptrTo(u8))}, ir.Param{Name: "audio_len", Type: ptrTo(u32)}),
		fn("SDL_LoadInts_IO", free, boolT, ir.Param{Name: "src", Type: stream}, ir.Param{Name: "closeio", Type: boolT}, ir.Param{Name: "buf", Type: ptrTo(ptrTo(i32))}, ir.Param{Name: "len", Type: ptrTo(u32)}),
	)...))
	if got, want := sigs["IOFromBytes"], "func(b []byte) *DisplayMode"; got != want {
		t.Errorf("IOFromBytes = %q, want %q", got, want)
	}
	if got, want := sigs["LoadThingBytes"], "func(data []byte) (SDL_DisplayMode, []byte, error)"; got != want && got != "func(data []byte) (DisplayMode, []byte, error)" {
		t.Errorf("LoadThingBytes = %q, want %q", got, want)
	}
	for _, name := range []string{"LoadPlainBytes", "LoadNoCloseBytes", "LoadIntsBytes"} {
		if _, found := sigs[name]; found {
			t.Errorf("%s must not be generated", name)
		}
	}

	// Without the stream constructors the owned-buffer variant has nothing to build on.
	sigs = funcSigs(t, synthFile(t, fn("SDL_LoadThing_IO", free, boolT, loadParams(spec)...)))
	for _, name := range []string{"IOFromBytes", "LoadThingBytes"} {
		if _, found := sigs[name]; found {
			t.Errorf("%s must not be generated without IOFromConstMem", name)
		}
	}
}
