package sdltest_test

import (
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

// pollLoop mimics an example's event loop on the main thread: it hands every
// queued event to handle, the way an example's PollEvent loop would.
func pollLoop(t *testing.T, handle func(*sdl.Event)) {
	t.Helper()
	sdltest.OnMain(t, func() {
		var e sdl.Event
		for sdl.PollEvent(&e) {
			handle(&e)
		}
	})
}

func TestInjectedEventsReachPollingLoopInOrder(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 64, 48)
	in := sdltest.NewInput(t, sdltest.WindowID(t, win))
	sdltest.Drain(t) // discard events from window creation

	in.Key(true, sdl.SCANCODE_RIGHT, sdl.SDLK_RIGHT, sdl.KMOD_LSHIFT)
	in.MouseMotion(10, 20, 3, -4, sdl.BUTTON_LMASK)
	in.MouseButton(sdl.BUTTON_LEFT, true, 10, 20)
	in.MouseWheel(0, 2)
	in.Resized(100, 80)
	in.Quit()

	var got []sdl.EventType
	var key sdl.KeyboardEvent
	var motion sdl.MouseMotionEvent
	var resized sdl.WindowEvent
	pollLoop(t, func(e *sdl.Event) {
		got = append(got, e.Type())
		switch e.Type() {
		case sdl.EVENT_KEY_DOWN:
			key = *e.Key()
		case sdl.EVENT_MOUSE_MOTION:
			motion = *e.Motion()
		case sdl.EVENT_WINDOW_RESIZED:
			resized = *e.Window()
		}
	})

	want := []sdl.EventType{sdl.EVENT_KEY_DOWN, sdl.EVENT_MOUSE_MOTION, sdl.EVENT_MOUSE_BUTTON_DOWN,
		sdl.EVENT_MOUSE_WHEEL, sdl.EVENT_WINDOW_RESIZED, sdl.EVENT_WINDOW_PIXEL_SIZE_CHANGED, sdl.EVENT_QUIT}
	if len(got) != len(want) {
		t.Fatalf("event types = %#x, want %#x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %#x, want %#x", i, got[i], want[i])
		}
	}
	if key.Key != sdl.SDLK_RIGHT || key.Scancode != sdl.SCANCODE_RIGHT || key.Mod != sdl.KMOD_LSHIFT || !key.Down {
		t.Errorf("key event = %+v", key)
	}
	if motion.X != 10 || motion.Y != 20 || motion.Xrel != 3 || motion.Yrel != -4 || motion.State != sdl.BUTTON_LMASK {
		t.Errorf("motion event = %+v", motion)
	}
	if resized.Data1 != 100 || resized.Data2 != 80 || resized.WindowID != sdltest.WindowID(t, win) {
		t.Errorf("resize event = %+v", resized)
	}
}

func TestPressDerivesKeycodeFromScancode(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	sdltest.NewInput(t, 0).Press(sdl.SCANCODE_ESCAPE)
	events := sdltest.Drain(t)
	if len(events) != 2 || events[0].Key().Key != sdl.SDLK_ESCAPE || !events[0].Key().Down ||
		events[1].Type() != sdl.EVENT_KEY_UP || events[1].Key().Down {
		t.Fatalf("Press produced %d events, first key %+v", len(events), events[0].Key())
	}
}

func TestTextAndDropStringsSurviveQueueing(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	in := sdltest.NewInput(t, 0)
	in.Text("héllo")
	in.Drop("/tmp/a.png")
	var texts, drops []string
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			switch e.Type() {
			case sdl.EVENT_TEXT_INPUT:
				texts = append(texts, sdl.GoString(e.Text().Text))
			case sdl.EVENT_DROP_FILE:
				drops = append(drops, e.Drop().DataString())
			}
		}
	})
	if len(texts) != 1 || texts[0] != "héllo" || len(drops) != 1 || drops[0] != "/tmp/a.png" {
		t.Fatalf("texts = %q, drops = %q", texts, drops)
	}
}

func TestPenAndUserEvents(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	in := sdltest.NewInput(t, 0)
	in.PenProximity(7, true)
	in.PenTouch(7, true, false, 5, 6)
	in.PenMotion(7, 8, 9, sdl.PEN_INPUT_DOWN)
	in.PenAxis(7, sdl.PEN_AXIS_PRESSURE, 0.5, 8, 9)
	in.PenButton(7, 1, true, 8, 9)
	in.User(42)
	ev := sdltest.Drain(t)
	types := []sdl.EventType{sdl.EVENT_PEN_PROXIMITY_IN, sdl.EVENT_PEN_DOWN, sdl.EVENT_PEN_MOTION,
		sdl.EVENT_PEN_AXIS, sdl.EVENT_PEN_BUTTON_DOWN, sdl.EVENT_USER}
	if len(ev) != len(types) {
		t.Fatalf("got %d events, want %d", len(ev), len(types))
	}
	for i, typ := range types {
		if ev[i].Type() != typ {
			t.Fatalf("event %d type = %#x, want %#x", i, ev[i].Type(), typ)
		}
	}
	if p := ev[1].Ptouch(); !p.Down || p.Which != 7 || p.X != 5 || p.Y != 6 {
		t.Errorf("pen touch = %+v", p)
	}
	if p := ev[3].Paxis(); p.Axis != sdl.PEN_AXIS_PRESSURE || p.Value != 0.5 {
		t.Errorf("pen axis = %+v", p)
	}
	if ev[5].User().Code != 42 {
		t.Errorf("user code = %d", ev[5].User().Code)
	}
	if n := len(sdltest.EventsOfType(ev, sdl.EVENT_PEN_MOTION)); n != 1 {
		t.Errorf("EventsOfType found %d pen motions, want 1", n)
	}
}

func TestRealWindowResizeProducesEvents(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 64, 48)
	sdltest.Drain(t)
	sdltest.ResizeWindow(t, win, 120, 90)
	var w, h int32
	sdltest.OnMain(t, func() { sdl.GetWindowSize(win, &w, &h) })
	if w != 120 || h != 90 {
		t.Fatalf("window size = %dx%d, want 120x90", w, h)
	}
	if len(sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_WINDOW_RESIZED)) == 0 {
		t.Error("no EVENT_WINDOW_RESIZED after resizing a real window")
	}
}

func TestClipboardRoundTrip(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	got := sdltest.Call(t, func() string {
		sdl.SetClipboardText("hello clipboard")
		return sdl.GetClipboardText()
	})
	if got != "hello clipboard" {
		t.Fatalf("clipboard = %q", got)
	}
}
