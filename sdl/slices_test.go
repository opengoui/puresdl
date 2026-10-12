package sdl

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unsafe"
)

// globRaw calls SDL_GlobDirectory directly to get the SDL-allocated,
// NULL-terminated char ** that the helpers are meant for.
func globRaw(t *testing.T, names ...string) (**byte, int) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var count int32
	p := fnGlobDirectory(CString(dir), nil, 0, &count)
	if p == nil {
		t.Fatal(GetError())
	}
	return p, int(count)
}

func TestGoStringsOnSDLArray(t *testing.T) {
	p, n := globRaw(t, "a.txt", "b.txt", "c.txt")
	defer Free(unsafe.Pointer(p))
	if n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
	want := []string{"a.txt", "b.txt", "c.txt"}
	got := GoStrings(p)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("GoStrings = %q, want %q", got, want)
	}
	got = GoStringsN(p, n)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("GoStringsN = %q, want %q", got, want)
	}
	if got := GoStringsN(p, 2); len(got) != 2 {
		t.Errorf("GoStringsN(p, 2) has %d elements", len(got))
	}
}

func TestGoStringsEmptyAndNil(t *testing.T) {
	if GoStrings(nil) != nil || GoStringsN(nil, 3) != nil {
		t.Error("nil array must give a nil slice")
	}
	p, n := globRaw(t)
	defer Free(unsafe.Pointer(p))
	if n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	if got := GoStrings(p); len(got) != 0 {
		t.Errorf("GoStrings(empty) = %q", got)
	}
	if GoStringsN(p, 0) != nil || GoStringsN(p, -1) != nil {
		t.Error("n <= 0 must give a nil slice")
	}
}

func TestGoStringsGoMemory(t *testing.T) {
	a, b := CString("one"), CString("two")
	arr := []*byte{a, b, nil}
	if got := GoStrings(&arr[0]); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("GoStrings = %q", got)
	}
	arr = []*byte{a, nil, b}
	if got := GoStringsN(&arr[0], 3); !slices.Equal(got, []string{"one", "", "two"}) {
		t.Errorf("GoStringsN = %q (a nil element is the empty string)", got)
	}
}

func TestSlice(t *testing.T) {
	if Slice[int32](nil, 4) != nil {
		t.Error("nil pointer must give a nil slice")
	}
	x := []int32{1, 2, 3}
	if Slice(&x[0], 0) != nil || Slice(&x[0], -1) != nil {
		t.Error("n <= 0 must give a nil slice")
	}
	s := Slice(&x[0], 3)
	if !slices.Equal(s, x) {
		t.Errorf("Slice = %v", s)
	}
	s[1] = 20
	if x[1] != 20 {
		t.Error("Slice must alias the memory, not copy it")
	}
	// Over SDL-allocated memory.
	m := (*uint32)(Malloc(16))
	if m == nil {
		t.Fatal("Malloc failed")
	}
	defer Free(unsafe.Pointer(m))
	c := Slice(m, 4)
	if len(c) != 4 {
		t.Fatalf("len = %d", len(c))
	}
	for i := range c {
		c[i] = uint32(i * 7)
	}
	if got := *(*uint32)(unsafe.Add(unsafe.Pointer(m), 12)); got != 21 {
		t.Errorf("element 3 = %d, want 21", got)
	}
}

func TestGlobDirectoryUsesFreedStringsHelpers(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"x", "y"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	got := GlobDirectory(dir, "", 0)
	slices.Sort(got)
	if !slices.Equal(got, []string{"x", "y"}) {
		t.Errorf("GlobDirectory = %q", got)
	}
}
