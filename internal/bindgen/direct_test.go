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
	f64 := &ir.Type{Kind: ir.Float, Size: 8}
	var nine, mixed []ir.Param
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		nine = append(nine, param(n, i32))
	}
	// Eight integer and eight floating-point parameters still fit in
	// registers: each class has its own eight.
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		mixed = append(mixed, param(n, i32), param("f"+n, f32))
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
		fn("SDL_Mixed", void, mixed...),
		fn("SDL_Rotate", boolT, param("r", ptrTo(i32)), param("angle", f64)),
		fn("SDL_Ratio", f64, param("p", ptrTo(i32))),
	)
	for _, want := range []string{
		"var addrToggle uintptr",
		"return uint8(call1(addrToggle, cbool(on))) != 0",
		`missingSymbol("SDL_Toggle")`,
		"call2(addrTouch, uintptr(unsafe.Pointer(p)), uintptr(n))",
		"return (*DisplayMode)(cptr(call1(addrGetMode, uintptr(id))))",
		"return cptr(call0(addrGetBlob))",
		"var regScale func(s float32)",
		"callF0(addrScale, 0b1, &floatArgs{f32(s)})",
		"regScale(s)\n\t\treturn",
		"return regHalf(n)",
		"_, fr := callF1(addrHalf, 0b0, &floatArgs{}, uintptr(n))",
		"return retF32(fr)",
		// The integer result must not shadow the parameter named r.
		"r_, _ := callF1(addrRotate, 0b10, &floatArgs{f64(angle)}, uintptr(unsafe.Pointer(r)))",
		"return uint8(r_) != 0",
		"return retF64(fr)",
		"callF8(addrMixed, 0b1010101010101010, &floatArgs{f32(fa), f32(fb)",
		"var fnTakeMode func(mode DisplayMode) bool",
		"var fnNine func(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q", want)
		}
	}
	for _, name := range []string{"TakeMode", "Nine"} {
		if strings.Contains(src, "addr"+name) {
			t.Errorf("%s must stay on purego.RegisterFunc", name)
		}
	}
	for _, name := range []string{"Toggle", "Touch", "Scale", "Half"} {
		if strings.Contains(src, "reg"+name) != (name == "Scale" || name == "Half") {
			t.Errorf("%s: a reg%s fallback must exist exactly for floating-point signatures", name, name)
		}
	}
	sigs := funcSigs(t, src)
	if got, want := sigs["fnHalf"], "func(n int32) float32"; got != want {
		t.Errorf("fnHalf = %q, want %q (it must keep the raw signature)", got, want)
	}
	if got, want := sigs["fnTouch"], "func(p *int32, n int32)"; got != want {
		t.Errorf("fnTouch = %q, want %q (it must keep the raw signature)", got, want)
	}
}
