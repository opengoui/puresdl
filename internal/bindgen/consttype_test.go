package bindgen

import (
	"math/big"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

func TestFitsInt(t *testing.T) {
	for _, tc := range []struct {
		v      int64
		size   int64
		signed bool
		want   bool
	}{
		{255, 1, false, true},
		{256, 1, false, false},
		{-1, 4, false, false},
		{-128, 1, true, true},
		{128, 1, true, false},
		{1 << 16, 2, false, false},
	} {
		if got := fitsInt(big.NewInt(tc.v), tc.size, tc.signed); got != tc.want {
			t.Errorf("fitsInt(%d, %d, %v) = %v, want %v", tc.v, tc.size, tc.signed, got, tc.want)
		}
	}
}

func TestConstType(t *testing.T) {
	u32 := &ir.Type{Kind: ir.Int, Size: 4}
	flags := &ir.Typedef{Name: "SDL_Flags", Type: &ir.Type{Kind: ir.Named, Name: "SDL_Flags", Under: u32}}
	u16 := &ir.Type{Kind: ir.Int, Size: 2}
	small := &ir.Typedef{Name: "SDL_Small", Type: &ir.Type{Kind: ir.Named, Name: "SDL_Small", Under: u16}}
	g := &gen{
		tdGo:     map[string]string{"SDL_Flags": "Flags", "SDL_Small": "Small", "SDL_MouseButtonFlags": "MouseButtonFlags"},
		typedefs: map[string]*ir.Typedef{"SDL_Flags": flags, "SDL_Small": small, "SDL_MouseButtonFlags": flags},
	}
	c := func(name, after string, v int64) *ir.Const {
		return &ir.Const{Name: name, Kind: ir.ConstInt, Int: big.NewInt(v), After: after}
	}
	for _, tc := range []struct {
		k    *ir.Const
		want string
	}{
		{c("SDL_A", "SDL_Flags", 1), "Flags"},
		{c("SDL_B", "", 1), ""},
		{c("SDL_C", "SDL_Small", 1<<16), ""},              // does not fit the typedef
		{c("SDL_D", "Uint32", 1), ""},                     // builtin typedefs are not named Go types
		{c("SDL_E", "SDL_Flags", -1), ""},                 // negative into unsigned
		{c("SDL_WINDOWPOS_CENTERED", "SDL_Flags", 1), ""}, // override: untyped
		{c("SDL_BUTTON_LEFT", "SDL_Small", 1), "MouseButtonFlags"},
	} {
		if got := g.constType(tc.k); got != tc.want {
			t.Errorf("constType(%s) = %q, want %q", tc.k.Name, got, tc.want)
		}
	}
}
