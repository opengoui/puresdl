package sdl

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Single plain-value pointer parameters are copied into the call frame, so
// passing the address of a literal or a local does not allocate.
func TestScratchParamsDoNotAllocate(t *testing.T) {
	renderer, _ := softwareRenderer(t, 8, 8)
	if !InitSubSystem(INIT_EVENTS) {
		t.Fatal(GetError())
	}
	defer QuitSubSystem(INIT_EVENTS)
	var i int32
	for name, f := range map[string]func(){
		"RenderFillRect(&FRect{})": func() { i++; _ = RenderFillRect(renderer, &FRect{X: float32(i & 3), W: 2, H: 2}) },
		"GetRenderViewport(&local)": func() {
			var r Rect
			_ = GetRenderViewport(renderer, &r)
		},
		"PollEvent(&local)": func() {
			var ev Event
			_ = PollEvent(&ev)
		},
	} {
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s allocates %v times per call", name, n)
		}
	}
}

// Const parameters are copied in, non-const ones in and back out, and nil
// is still passed as NULL.
func TestScratchParamValues(t *testing.T) {
	renderer, _ := softwareRenderer(t, 8, 8)
	if !SetRenderViewport(renderer, &Rect{X: 1, Y: 2, W: 3, H: 4}) {
		t.Fatal(GetError())
	}
	got := Rect{X: -1, Y: -1, W: -1, H: -1}
	if !GetRenderViewport(renderer, &got) || got != (Rect{X: 1, Y: 2, W: 3, H: 4}) {
		t.Errorf("GetRenderViewport = %+v, want {1 2 3 4}", got)
	}
	if !SetRenderViewport(renderer, nil) || !GetRenderViewport(renderer, &got) || got != (Rect{W: 8, H: 8}) {
		t.Errorf("after SetRenderViewport(nil), viewport = %+v, want {0 0 8 8}", got)
	}

	a, b := Rect{X: 0, Y: 0, W: 4, H: 4}, Rect{X: 2, Y: 1, W: 4, H: 4}
	result := Rect{X: 99}
	if !GetRectIntersection(&a, &b, &result) || result != (Rect{X: 2, Y: 1, W: 2, H: 3}) {
		t.Errorf("GetRectIntersection = %+v, want {2 1 2 3}", result)
	}
	// The same address for an input and the output: the inputs are copied
	// before the call and the output is copied back after it.
	if !GetRectIntersection(&a, &b, &a) || a != (Rect{X: 2, Y: 1, W: 2, H: 3}) {
		t.Errorf("GetRectIntersection(&a, &b, &a) leaves a = %+v, want {2 1 2 3}", a)
	}
}

// The copy-back targets the caller's variable even when an event watch,
// called back on the same goroutine during PushEvent, grows and moves the
// goroutine stack that holds it.
func TestScratchParamSurvivesStackGrowthInCallback(t *testing.T) {
	if !InitSubSystem(INIT_EVENTS) {
		t.Fatal(GetError())
	}
	defer QuitSubSystem(INIT_EVENTS)
	typ := RegisterEvents(1)
	if typ == 0 {
		t.Fatal(GetError())
	}
	watched := 0
	watch := NewEventFilter(func(ud unsafe.Pointer, ev *Event) bool {
		if ev.Type() == EventType(typ) {
			watched++
			growStack(64)
		}
		return true
	})
	if !AddEventWatch(watch, nil) {
		t.Fatal(GetError())
	}
	defer RemoveEventWatch(watch, nil)
	defer FlushEvent(EventType(typ))
	for i := range 20 {
		type result struct {
			ok   bool
			code int32
			typ  uint32
		}
		done := make(chan result)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var ev Event
			u := ev.User()
			u.Type, u.Code = typ, int32(i)
			ok := PushEvent(&ev)
			done <- result{ok, ev.User().Code, ev.User().Type}
		}()
		r := <-done
		if !r.ok || r.code != int32(i) || r.typ != typ {
			t.Fatalf("PushEvent #%d = %v with event code %d type %d, want true, %d, %d", i, r.ok, r.code, r.typ, i, typ)
		}
	}
	if watched < 20 {
		t.Fatalf("event watch ran %d times, want at least 20", watched)
	}
	var ev Event
	seen := 0
	for PollEvent(&ev) {
		if ev.Type() == EventType(typ) && ev.User().Code == int32(seen) {
			seen++
		}
	}
	if seen != 20 {
		t.Errorf("PollEvent returned %d of the 20 pushed events in order", seen)
	}
}

// String arguments without a trailing NUL, such as the PROP_* and HINT_*
// constants, are copied into the call frame instead of a fresh allocation.
func TestScratchStringsDoNotAllocate(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	SetNumberProperty(props, PROP_WINDOW_CREATE_WIDTH_NUMBER, 7)
	for name, f := range map[string]func(){
		"GetNumberProperty(PROP_*)": func() { _ = GetNumberProperty(props, PROP_WINDOW_CREATE_WIDTH_NUMBER, 0) },
		"SetNumberProperty(PROP_*)": func() { _ = SetNumberProperty(props, PROP_WINDOW_CREATE_WIDTH_NUMBER, 7) },
		"GetHintBoolean(HINT_*)":    func() { _ = GetHintBoolean(HINT_RENDER_VSYNC, false) },
	} {
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s allocates %v times per call", name, n)
		}
	}
}

// Several strings in one call get separate copies, a string too long for the
// scratch area falls back to an allocated copy, and a result string is
// copied before the scratch area is reused.
func TestScratchStringValues(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	long := strings.Repeat("v", 600)
	for _, kv := range [][2]string{{"name", "value"}, {"k", ""}, {"long.value", long}, {strings.Repeat("n", 300), strings.Repeat("w", 300)}, {strings.Repeat("m", 600), "past the scratch area"}} {
		if !SetStringProperty(props, kv[0], Ptr(kv[1])) {
			t.Fatalf("SetStringProperty(%.10q): %s", kv[0], GetError())
		}
		if got := GetStringProperty(props, kv[0], "default"); got != kv[1] {
			t.Errorf("GetStringProperty(%.10q) = %.10q (len %d), want %.10q (len %d)", kv[0], got, len(got), kv[1], len(kv[1]))
		}
	}
	if got := GetStringProperty(props, "missing", "fallback"); got != "fallback" {
		t.Errorf("GetStringProperty(missing) = %q, want the default", got)
	}
	const hint = "PURESDL_SCRATCH_HINT"
	defer ResetHint(hint)
	if !SetHint(hint, "on") {
		t.Fatal(GetError())
	}
	if v, ok := GetHintOK(hint); !ok || v != "on" {
		t.Errorf("GetHintOK = %q, %v, want \"on\", true", v, ok)
	}
}
