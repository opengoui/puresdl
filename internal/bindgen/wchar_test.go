package bindgen

import (
	"fmt"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

func wcharNamed() *ir.Type {
	return &ir.Type{Kind: ir.Named, Name: "wchar_t", Under: &ir.Type{Kind: ir.Int, Size: 4, Signed: true}}
}

func TestWcharMapping(t *testing.T) {
	g := &gen{tdGo: map[string]string{}}
	got, err := g.typ(wcharNamed())
	if err != nil || fmt.Sprintf("%#v", got) != "Wchar" {
		t.Fatalf("typ(wchar_t) = %#v, %v; want Wchar", got, err)
	}
	ptr := &ir.Type{Kind: ir.Pointer, Size: 8, Const: true, Elem: wcharNamed()}
	got, err = g.typ(ptr)
	if err != nil || fmt.Sprintf("%#v", got) != "*Wchar" {
		t.Fatalf("typ(const wchar_t *) = %#v, %v; want *Wchar", got, err)
	}
	if !isWcharString(ptr) || isWcharString(cstrType(true)) {
		t.Error("isWcharString misclassifies wchar_t* / char*")
	}
	if isWcharString(&ir.Type{Kind: ir.Pointer, Elem: wcharNamed()}) {
		t.Error("non-const wchar_t* is an output buffer, not a string input")
	}
}

func TestDegradedField(t *testing.T) {
	g := &gen{}
	ptr := &ir.Type{Kind: ir.Pointer, Size: 8, Align: 8, Elem: &ir.Type{Kind: ir.Void}}
	if s := fmt.Sprintf("%#v", g.degradedField("R", ir.Field{Name: "p", Type: ptr}, fmt.Errorf("boom"))); s != "unsafe.Pointer" {
		t.Errorf("pointer field = %s", s)
	}
	arr := &ir.Type{Kind: ir.Int, Size: 16, Align: 8}
	if s := fmt.Sprintf("%#v", g.degradedField("R", ir.Field{Name: "a", Type: arr}, fmt.Errorf("boom"))); s != "[2]uint64" {
		t.Errorf("value field = %s", s)
	}
}
