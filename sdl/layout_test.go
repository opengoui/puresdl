package sdl

import (
	"testing"
	"unsafe"
)

func checkLayout(t *testing.T, name string, gotSize, gotAlign uintptr, wantSize, wantAlign int) {
	t.Helper()
	if int(gotSize) != wantSize || int(gotAlign) != wantAlign {
		t.Errorf("%s: size/align = %d/%d, C has %d/%d", name, gotSize, gotAlign, wantSize, wantAlign)
	}
}

func checkOffset(t *testing.T, name string, got uintptr, want int) {
	t.Helper()
	if int(got) != want {
		t.Errorf("%s: offset = %d, C has %d", name, got, want)
	}
}

func init() {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		panic("sdl layout tests assume a 64-bit target")
	}
}
