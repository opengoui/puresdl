package sdl

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// The direct-call bridge (call0 ... call8) must not allocate for scalar and
// pointer signatures; NUL-terminated names avoid the cstr copy.
func TestDirectCallsDoNotAllocate(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	const key = "puresdl.direct\x00"
	if !SetNumberProperty(props, key, 7) {
		t.Fatal(GetError())
	}
	var now Time
	for name, f := range map[string]func(){
		"GetTicks":          func() { _ = GetTicks() },
		"GetNumberProperty": func() { _ = GetNumberProperty(props, key, 0) },
		"HasProperty":       func() { _ = HasProperty(props, key) },
		"GetCurrentTime":    func() { _ = GetCurrentTime(&now) },
	} {
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s allocates %v times per call", name, n)
		}
	}
}

// Results, out-parameters and bool conversions survive the uintptr bridge,
// including across garbage collections.
func TestDirectCallValues(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	for i := range 200 {
		key := strings.Repeat("k", i%17+1) // heap copy made by cstr each call
		if !SetNumberProperty(props, key, int64(-i)) {
			t.Fatal(GetError())
		}
		if got := GetNumberProperty(props, key, 1); got != int64(-i) {
			t.Fatalf("GetNumberProperty(%q) = %d, want %d", key, got, -i)
		}
		if !HasProperty(props, key) || HasProperty(props, key+"?") {
			t.Fatalf("HasProperty misreports %q", key)
		}
		if i%50 == 0 {
			runtime.GC()
		}
	}
	var now Time
	if !GetCurrentTime(&now) || now == 0 {
		t.Fatalf("GetCurrentTime = %d: %s", now, GetError())
	}
	if w := GetWindowFromID(0xFFFF_FFF0); w != nil {
		t.Errorf("GetWindowFromID(unknown) = %p, want nil", w)
	}
}

func TestDirectCallMissingSymbolPanics(t *testing.T) {
	saved := addrGetTicks
	addrGetTicks = 0
	defer func() {
		addrGetTicks = saved
		msg, _ := recover().(string)
		if !strings.Contains(msg, "SDL_GetTicks is not available") {
			t.Errorf("panic = %q, want a message naming SDL_GetTicks", msg)
		}
	}()
	GetTicks()
}

// A C function that calls back into Go on the same thread runs the callback
// on the calling goroutine's stack, which may grow and move it during the
// call. The result of the direct call must survive that.
func TestDirectCallSurvivesStackGrowthInCallback(t *testing.T) {
	const name = "PURESDL_STACK_GROWTH_HINT"
	calls := 0
	cb := NewHintCallback(func(ud unsafe.Pointer, n, oldV, newV *byte) {
		calls++
		growStack(64)
	})
	if !AddHintCallback(name, cb, nil) {
		t.Fatal(GetError())
	}
	defer RemoveHintCallback(name, cb, nil)
	defer ResetHint(name)
	for i := range 20 {
		// A fresh goroutine starts with a small stack, so the callback
		// moves it.
		ok := make(chan bool)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ok <- SetHint(name, strconv.Itoa(i))
		}()
		if !<-ok {
			t.Fatalf("SetHint #%d = false across a stack-moving callback: %s", i, GetError())
		}
	}
	if calls < 20 {
		t.Fatalf("hint callback ran %d times, want at least 20", calls)
	}
}

// growStack uses about depth KiB of goroutine stack.
//
//go:noinline
func growStack(depth int) byte {
	var buf [1024]byte
	buf[depth%len(buf)] = byte(depth)
	if depth == 0 {
		return buf[0]
	}
	return growStack(depth-1) + buf[depth%len(buf)]
}

// positionalArgs restores parameter order for the Microsoft x64 ABI, which
// cannot be exercised on the machines that run most tests.
func TestPositionalArgs(t *testing.T) {
	a := [maxDirectArgs]uintptr{1, 2, 3, 4, 5, 6, 7, 8}
	f := floatArgs{101, 102, 103, 104, 105, 106, 107, 108}
	for _, tc := range []struct {
		name  string
		fmask uint16
		nInt  int
		want  []uintptr
	}{
		{"integers only", 0, 3, []uintptr{1, 2, 3}},
		// SetRenderDrawColorFloat(renderer, r, g, b, a).
		{"pointer then floats", 0b11110, 1, []uintptr{1, 101, 102, 103, 104}},
		// RenderTextureRotated(renderer, texture, srcrect, dstrect, angle, center, flip).
		{"float between integers", 0b0010000, 6, []uintptr{1, 2, 3, 4, 101, 5, 6}},
		{"leading float", 0b1, 1, []uintptr{101, 1}},
		{"eight of each, interleaved", 0b1010101010101010, 8,
			[]uintptr{1, 101, 2, 102, 3, 103, 4, 104, 5, 105, 6, 106, 7, 107, 8, 108}},
	} {
		in := a
		clear(in[tc.nInt:])
		nFloat := 0
		for m := tc.fmask; m != 0; m &= m - 1 {
			nFloat++
		}
		fin := f
		clear(fin[nFloat:])
		got := positionalArgs(&in, &fin, tc.fmask)
		want := make([]uintptr, len(got))
		copy(want, tc.want)
		if got != [2 * maxDirectArgs]uintptr(want) {
			t.Errorf("%s: positionalArgs = %v, want %v", tc.name, got, want)
		}
	}
}
