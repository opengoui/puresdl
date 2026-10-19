package bindgen

import (
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

func TestScratchParamRules(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	record := func(name string, size int64, fields ...ir.Field) *ir.Type {
		return &ir.Type{Kind: ir.Named, Name: name, Under: &ir.Type{Kind: ir.Struct, Rec: &ir.Record{Name: name, Size: size, Fields: fields}}}
	}
	frect := record("SDL_FRect", 16, ir.Field{Name: "x", Type: &ir.Type{Kind: ir.Float, Size: 4}})
	event := &ir.Type{Kind: ir.Named, Name: "SDL_Event", Under: &ir.Type{Kind: ir.Union, Rec: &ir.Record{Name: "SDL_Event", Size: 128, Union: true}}}
	atomic := record("SDL_AtomicInt", 4, ir.Field{Name: "value", Type: i32})
	cptr := func(e *ir.Type) *ir.Type { return &ir.Type{Kind: ir.Pointer, Size: 8, Elem: e, Const: true} }
	param := func(name string, t *ir.Type) ir.Param { return ir.Param{Name: name, Type: t} }
	fn := func(params ...ir.Param) *ir.Function {
		return &ir.Function{Name: "SDL_F", Sig: &ir.Sig{Ret: &ir.Type{Kind: ir.Void}, Params: params}}
	}
	mode := func(sp []scratchParam, i int) scratchMode {
		if sp == nil {
			return scratchNone
		}
		return sp[i].mode
	}
	for _, tc := range []struct {
		name string
		f    *ir.Function
		want []scratchMode
	}{
		{"const single rect", fn(param("r", ptrTo(i32)), param("rect", cptr(frect))), []scratchMode{scratchNone, scratchIn}},
		{"out rect", fn(param("rect", ptrTo(frect))), []scratchMode{scratchInOut}},
		{"union event", fn(param("event", ptrTo(event))), []scratchMode{scratchInOut}},
		{"array with count", fn(param("rect", cptr(frect)), param("count", i32)), []scratchMode{scratchNone, scratchNone}},
		{"plural name", fn(param("rects", cptr(frect))), []scratchMode{scratchNone}},
		{"array in doc", fn(ir.Param{Name: "rect", Type: cptr(frect), Doc: "an array of rectangles"}), []scratchMode{scratchNone}},
		{"retained", fn(ir.Param{Name: "rect", Type: cptr(frect), Doc: "the rect is not copied"}), []scratchMode{scratchNone}},
		// Its address matters: copying it would break the atomic operation.
		{"unlisted type", fn(param("a", ptrTo(atomic))), []scratchMode{scratchNone}},
		{"integer pointer", fn(param("w", ptrTo(i32))), []scratchMode{scratchNone}},
	} {
		sp := scratchParams(tc.f)
		for i, want := range tc.want {
			if got := mode(sp, i); got != want {
				t.Errorf("%s: parameter %d mode = %d, want %d", tc.name, i, got, want)
			}
		}
	}

	// Offsets are 8-byte aligned and the copies fit the scratch area.
	sp := scratchParams(fn(param("a", cptr(frect)), param("event", ptrTo(event)), param("b", cptr(frect))))
	if sp[0].off != 0 || sp[1].off != 16 || sp[2].off != 144 {
		t.Errorf("offsets = %d %d %d, want 0 16 144", sp[0].off, sp[1].off, sp[2].off)
	}
	var many []ir.Param
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		many = append(many, param(n, ptrTo(event)))
	}
	sp = scratchParams(fn(many...))
	if mode(sp, 3) != scratchInOut || mode(sp, 4) != scratchNone {
		t.Errorf("five events: modes %d %d, want the fifth to overflow the 512-byte area", mode(sp, 3), mode(sp, 4))
	}

	// A listed type with a Go pointer field is a generator bug.
	defer func() {
		if recover() == nil {
			t.Error("scratchParams accepted a scratch record with a pointer field")
		}
	}()
	scratchParams(fn(param("rect", ptrTo(record("SDL_Rect", 8, ir.Field{Name: "p", Type: ptrTo(i32)})))))
}
