package sdl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func requireLoaded(t *testing.T) {
	t.Helper()
	if err := LoadError(); err != nil {
		t.Skipf("SDL3 not loaded: %v", err)
	}
}

// Empty-but-successful results are non-nil, so they differ from NULL.
func TestCopyFreeEmptyIsNonNil(t *testing.T) {
	requireLoaded(t)
	if s := copyFree((*int32)(Malloc(8)), 0); s == nil || len(s) != 0 {
		t.Errorf("copyFree of an empty array = %#v, want empty non-nil", s)
	}
	if s := copyFreeBytes(Malloc(8), 0); s == nil || len(s) != 0 {
		t.Errorf("copyFreeBytes of an empty array = %#v, want empty non-nil", s)
	}
	if copyFree[int32](nil, 0) != nil || copyFreeBytes(nil, 0) != nil {
		t.Error("NULL must stay nil")
	}
}

func TestLoadFileEmptyAndMissing(t *testing.T) {
	requireLoaded(t)
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if b := LoadFile(empty); b == nil || len(b) != 0 {
		t.Errorf("LoadFile(empty) = %#v, want empty non-nil", b)
	}
	if b := LoadFile(filepath.Join(dir, "missing")); b != nil {
		t.Errorf("LoadFile(missing) = %#v, want nil", b)
	}
}

// The helpers copy hand-built SDL allocations, including the deep copy of an
// array of pointers whose pointees share the allocation.
func TestCopyFreeHelpersOnMallocArrays(t *testing.T) {
	requireLoaded(t)
	ints := (*int32)(Malloc(12))
	copy(unsafe.Slice(ints, 3), []int32{7, 8, 9})
	if got := copyFree(ints, 3); len(got) != 3 || got[0] != 7 || got[2] != 9 {
		t.Errorf("copyFree = %v", got)
	}

	const n = 3
	size := uintptr(n)*unsafe.Sizeof((*DisplayMode)(nil)) + uintptr(n)*unsafe.Sizeof(DisplayMode{})
	block := Malloc(uint(size))
	ptrs := unsafe.Slice((**DisplayMode)(block), n)
	modes := unsafe.Slice((*DisplayMode)(unsafe.Add(block, uintptr(n)*unsafe.Sizeof((*DisplayMode)(nil)))), n)
	for i := range modes {
		modes[i] = DisplayMode{W: int32(100 + i), H: int32(50 + i)}
		ptrs[i] = &modes[i]
	}
	got := copyFreeDeref((**DisplayMode)(block), n)
	for i, m := range got {
		if m.W != int32(100+i) || m.H != int32(50+i) {
			t.Errorf("mode[%d] = %+v", i, m)
		}
	}

	strs := (**byte)(Malloc(uint(2 * unsafe.Sizeof((*byte)(nil)))))
	sl := unsafe.Slice(strs, 2)
	sl[0], sl[1] = CString("ab"), CString("cd")
	if got := copyFreeStrings(strs, 2); len(got) != 2 || got[0] != "ab" || got[1] != "cd" {
		t.Errorf("copyFreeStrings = %v", got)
	}
}

// SDL keeps the scancode name pointer, so it has to survive Go's GC.
func TestSetScancodeNameSurvivesGC(t *testing.T) {
	requireLoaded(t)
	coreSkipUnless(t, "SDL_SetScancodeName")
	const name = "CustomNameForA"
	if !SetScancodeName(SCANCODE_A, name) {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { SetScancodeName(SCANCODE_A, "") })
	for i := 0; i < 20; i++ {
		runtime.GC()
		junk := make([][]byte, 256)
		for j := range junk {
			junk[j] = []byte(strings.Repeat("Z", 16))
		}
		runtime.KeepAlive(junk)
	}
	if got := GetScancodeName(SCANCODE_A); got != name {
		t.Errorf("GetScancodeName = %q, want %q", got, name)
	}
	// Replacing and restoring the default must not leave a dangling name.
	if !SetScancodeName(SCANCODE_A, "Other") || GetScancodeName(SCANCODE_A) != "Other" {
		t.Errorf("replace failed: %q", GetScancodeName(SCANCODE_A))
	}
	if !SetScancodeName(SCANCODE_A, "") || GetScancodeName(SCANCODE_A) == "Other" {
		t.Errorf("reset failed: %q", GetScancodeName(SCANCODE_A))
	}
}

// A nil override selects the default title root (the base path); an empty
// string is a different, empty root.
func TestOpenTitleStorageNilIsDefaultRoot(t *testing.T) {
	requireLoaded(t)
	base := GetBasePath()
	if base == "" {
		t.Skip("no base path on this platform")
	}
	marker := filepath.Join(base, "puresdl-title-marker.txt")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Skipf("base path not writable: %v", err)
	}
	t.Cleanup(func() { os.Remove(marker) })
	probe := func(override *string) bool {
		st := OpenTitleStorage(override, 0)
		if st == nil {
			t.Fatal(GetError())
		}
		defer CloseStorage(st)
		for i := 0; i < 100 && !StorageReady(st); i++ {
			Delay(1)
		}
		var info PathInfo
		return GetStoragePathInfo(st, "puresdl-title-marker.txt", &info)
	}
	if !probe(nil) {
		t.Error("nil override does not reach the base path")
	}
	if probe(Ptr("")) {
		t.Error("empty override must not be the base path")
	}
}
