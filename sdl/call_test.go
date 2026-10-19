package sdl

import (
	"runtime"
	"strings"
	"testing"
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
