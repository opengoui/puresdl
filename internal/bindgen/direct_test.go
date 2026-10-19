package bindgen

import (
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

func TestDirectCallRules(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	f32 := &ir.Type{Kind: ir.Float, Size: 4}
	boolT := &ir.Type{Kind: ir.Bool}
	void := &ir.Type{Kind: ir.Void}
	fn := func(name string, ret *ir.Type, params ...ir.Param) *ir.Function {
		return &ir.Function{Name: name, Sig: &ir.Sig{Ret: ret, Params: params}}
	}
	param := func(name string, t *ir.Type) ir.Param { return ir.Param{Name: name, Type: t} }
	var nine []ir.Param
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		nine = append(nine, param(n, i32))
	}
	src := synthFile(t,
		fn("SDL_Toggle", boolT, param("on", boolT)),
		fn("SDL_Touch", void, param("p", ptrTo(i32)), param("n", i32)),
		fn("SDL_GetMode", ptrTo(synthModeType), param("id", i32)),
		fn("SDL_GetBlob", ptrTo(void)),
		fn("SDL_Scale", void, param("s", f32)),
		fn("SDL_Half", f32, param("n", i32)),
		fn("SDL_TakeMode", boolT, param("mode", synthModeType)),
		fn("SDL_Nine", void, nine...),
	)
	for _, want := range []string{
		"var addrToggle uintptr",
		"return uint8(call1(addrToggle, cbool(on))) != 0",
		`missingSymbol("SDL_Toggle")`,
		"call2(addrTouch, uintptr(unsafe.Pointer(p)), uintptr(n))",
		"return (*DisplayMode)(cptr(call1(addrGetMode, uintptr(id))))",
		"return cptr(call0(addrGetBlob))",
		"var fnScale func(s float32)",
		"var fnHalf func(n int32) float32",
		"var fnTakeMode func(mode DisplayMode) bool",
		"var fnNine func(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q", want)
		}
	}
	for _, name := range []string{"Scale", "Half", "TakeMode", "Nine"} {
		if strings.Contains(src, "addr"+name) {
			t.Errorf("%s must stay on purego.RegisterFunc", name)
		}
	}
	sigs := funcSigs(t, src)
	if got, want := sigs["fnTouch"], "func(p *int32, n int32)"; got != want {
		t.Errorf("fnTouch = %q, want %q (it must keep the raw signature)", got, want)
	}
}
