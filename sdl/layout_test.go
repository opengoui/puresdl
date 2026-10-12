package sdl

import "testing"

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
