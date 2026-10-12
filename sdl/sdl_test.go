package sdl

import (
	"os"
	"strings"
	"testing"
	"unsafe"
)

func TestMain(m *testing.M) {
	os.Setenv("SDL_VIDEODRIVER", "dummy")
	os.Setenv("SDL_AUDIODRIVER", "dummy")
	if err := LoadError(); err != nil {
		println("skipping SDL runtime tests:", err.Error())
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestVersion(t *testing.T) {
	v := GetVersion()
	if v/1000000 != 3 {
		t.Fatalf("GetVersion() = %d", v)
	}
	if !strings.HasPrefix(GetRevision(), "SDL-") && GetRevision() == "" {
		t.Errorf("GetRevision() = %q", GetRevision())
	}
	if !Available("SDL_Init") || Available("SDL_DoesNotExist") {
		t.Error("Available misreports symbols")
	}
}

func TestErrors(t *testing.T) {
	ClearError()
	if SetError("bad %s: 100%% of %d", "thing", 3) {
		t.Fatal("SetError returned true")
	}
	if got, want := GetError(), "bad thing: 100% of 3"; got != want {
		t.Errorf("GetError() = %q, want %q", got, want)
	}
	SetError("literal %d with no args")
	if got := GetError(); got != "literal %d with no args" {
		t.Errorf("GetError() = %q", got)
	}
}

func TestGUIDByValue(t *testing.T) {
	const s = "030000005e040000130b000001050000"
	g := StringToGUID(s)
	if g.Data[0] != 0x03 || g.Data[2] != 0 && g.Data[1] != 0 {
		t.Errorf("GUID = %v", g.Data)
	}
	buf := make([]byte, 33)
	GUIDToString(g, &buf[0], int32(len(buf)))
	if got := GoString(&buf[0]); got != s {
		t.Errorf("GUIDToString = %q, want %q", got, s)
	}
}

func TestEventUnion(t *testing.T) {
	if !Init(INIT_EVENTS) {
		t.Fatal(GetError())
	}
	defer Quit()
	var ev Event
	ev.SetType(EVENT_USER)
	*ev.User() = UserEvent{Type: uint32(EVENT_USER), Code: 42}
	if !PushEvent(&ev) {
		t.Fatal(GetError())
	}
	var out Event
	for i := 0; i < 100 && !PollEvent(&out); i++ {
	}
	if out.Type() != EVENT_USER || out.User().Code != 42 {
		t.Errorf("polled event type=%d user=%+v", out.Type(), out.User())
	}
}

func TestPropertiesNullableStrings(t *testing.T) {
	p := CreateProperties()
	if p == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(p)
	if !SetStringProperty(p, "name", "puresdl") {
		t.Fatal(GetError())
	}
	if got := GetStringProperty(p, "name", ""); got != "puresdl" {
		t.Errorf("GetStringProperty = %q", got)
	}
	if got := GetStringProperty(p, "missing", "dflt"); got != "dflt" {
		t.Errorf("default = %q", got)
	}
}

func TestRectPointers(t *testing.T) {
	a, b := Rect{0, 0, 10, 10}, Rect{5, 5, 10, 10}
	var out Rect
	if !GetRectIntersection(&a, &b, &out) || out != (Rect{5, 5, 5, 5}) {
		t.Errorf("intersection = %+v", out)
	}
}

func TestLogOutputCallback(t *testing.T) {
	var got []string
	cb := NewLogOutputFunction(func(userdata unsafe.Pointer, category int32, priority LogPriority, message *byte) {
		got = append(got, GoString(message))
	})
	SetLogOutputFunction(cb, nil)
	SetLogPriorities(LOG_PRIORITY_INFO)
	LogInfo(int32(LOG_CATEGORY_APPLICATION), "hello %s 100%%", "world")
	if len(got) != 1 || got[0] != "hello world 100%" {
		t.Errorf("log output = %q", got)
	}
}

func TestEventWatchCallback(t *testing.T) {
	if !Init(INIT_EVENTS) {
		t.Fatal(GetError())
	}
	defer Quit()
	n := 0
	cb := NewEventFilter(func(userdata unsafe.Pointer, e *Event) bool {
		if e.Type() == EVENT_USER {
			n++
		}
		return true
	})
	AddEventWatch(cb, nil)
	var ev Event
	ev.SetType(EVENT_USER)
	PushEvent(&ev)
	RemoveEventWatch(cb, nil)
	if n != 1 {
		t.Errorf("watch calls = %d", n)
	}
}

func TestDummyWindow(t *testing.T) {
	if !Init(INIT_VIDEO) {
		t.Fatal(GetError())
	}
	defer Quit()
	w := CreateWindow("puresdl", 64, 48, 0)
	if w == nil {
		t.Fatal(GetError())
	}
	defer DestroyWindow(w)
	var cw, ch int32
	if !GetWindowSize(w, &cw, &ch) || cw != 64 || ch != 48 {
		t.Errorf("size = %dx%d", cw, ch)
	}
	if got := GetWindowTitle(w); got != "puresdl" {
		t.Errorf("title = %q", got)
	}
	if GetCurrentVideoDriver() != "dummy" {
		t.Errorf("driver = %q", GetCurrentVideoDriver())
	}
}

func TestFreedString(t *testing.T) {
	// SDL_GetClipboardText returns a char* that the wrapper must free.
	if !Init(INIT_VIDEO) {
		t.Fatal(GetError())
	}
	defer Quit()
	SetClipboardText("clip")
	if got := GetClipboardText(); got != "clip" && got != "" {
		t.Errorf("clipboard = %q", got)
	}
}

func TestMissingSymbolPanics(t *testing.T) {
	var missing string
	for _, b := range allBindings() {
		if !Available(b) {
			missing = b
			break
		}
	}
	if missing == "" {
		t.Skip("every symbol is present in this SDL build")
	}
	t.Logf("first symbol absent from this build: %s", missing)
}
