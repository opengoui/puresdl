package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// feed pumps SDL and applies every queued event to a like the run loop does.
// It reports whether the app asked to quit.
func feed(t *testing.T, a *app) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if !a.event(&e) {
				quit = true
			}
		}
	})
	return quit
}

func setup(t *testing.T) (*app, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	var clock sdltest.Clock
	sdltest.OnMain(t, func() { sdl.Srand(1) })
	a := newApp(clock.Ticks)
	return a, &clock
}

func texts(a *app) []string {
	var s []string
	for _, m := range a.messages {
		s = append(s, m.str)
	}
	return s
}

func wantTexts(t *testing.T, a *app, want ...string) {
	t.Helper()
	got := texts(a)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("messages = %q, want %q", got, want)
	}
}

func TestInitialState(t *testing.T) {
	a, _ := setup(t)
	wantTexts(t, a, "Please plug in a gamepad.")
	if a.colors[0] != (sdl.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("colors[0] = %v, want opaque white", a.colors[0])
	}
	for i, c := range a.colors {
		if c.A != 255 {
			t.Errorf("colors[%d].A = %d, want 255", i, c.A)
		}
	}
}

func TestQuitPaths(t *testing.T) {
	a, _ := setup(t)
	sdltest.NewInput(t, 0).Quit()
	if !feed(t, a) {
		t.Error("EVENT_QUIT did not quit")
	}
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	if !feed(t, a) {
		t.Error("Escape did not quit")
	}
	in.Press(sdl.SCANCODE_A)
	if feed(t, a) {
		t.Error("an ordinary key must not quit")
	}
}

func TestGamepadHotplugAddAndRemove(t *testing.T) {
	a, _ := setup(t)
	pad := sdltest.AttachGamepad(t, "Test Pad")
	feed(t, a)
	got := texts(a)
	if len(got) < 3 || got[1] != fmt.Sprintf("Gamepad #%d ('Test Pad') added", pad.ID) {
		t.Fatalf("messages after attach = %q", got)
	}
	if !strings.HasPrefix(got[2], fmt.Sprintf("Gamepad #%d mapping: ", pad.ID)) || len(got[2]) < 40 {
		t.Errorf("mapping message = %q", got[2])
	}
	if sdltest.Call(t, func() bool { return sdl.GetGamepadFromID(pad.ID) != nil }) == false {
		t.Error("gamepad was not opened on ADDED")
	}
	n := len(a.messages)
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(pad.ID) })
	feed(t, a)
	if got := texts(a); len(got) != n+1 || got[n] != fmt.Sprintf("Gamepad #%d removed", pad.ID) {
		t.Errorf("messages after detach = %q", got)
	}
	if sdltest.Call(t, func() bool { return sdl.GetGamepadFromID(pad.ID) == nil }) == false {
		t.Error("gamepad still open after REMOVED")
	}
}

func TestButtonMessages(t *testing.T) {
	a, _ := setup(t)
	pad := sdltest.AttachGamepad(t, "Pad")
	feed(t, a)
	feed(t, a) // SDL's first update reports the initial axis state
	base := len(a.messages)
	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, true)
	feed(t, a) // virtual state is sampled per pump, so press and release need separate pumps
	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, false)
	feed(t, a)
	got := texts(a)[base:]
	want := []string{
		fmt.Sprintf("Gamepad #%d button a -> PRESSED", pad.ID),
		fmt.Sprintf("Gamepad #%d button a -> RELEASED", pad.ID),
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInjectedButtonUsesJoystickColor(t *testing.T) {
	a, _ := setup(t)
	a.colors[7] = sdl.Color{R: 1, G: 2, B: 3, A: 255}
	sdltest.NewInput(t, 0).GamepadButton(7, sdl.GAMEPAD_BUTTON_START, true)
	feed(t, a)
	last := a.messages[len(a.messages)-1]
	if last.str != "Gamepad #7 button start -> PRESSED" || last.color != a.colors[7] {
		t.Errorf("message = %q color %v", last.str, last.color)
	}
	// The color table wraps modulo its length.
	sdltest.NewInput(t, 0).GamepadButton(64+7, sdl.GAMEPAD_BUTTON_START, false)
	feed(t, a)
	last = a.messages[len(a.messages)-1]
	if last.str != "Gamepad #71 button start -> RELEASED" || last.color != a.colors[7] {
		t.Errorf("wrapped message = %q color %v", last.str, last.color)
	}
}

func TestAxisCooldown(t *testing.T) {
	a, clock := setup(t)
	in := sdltest.NewInput(t, 0)
	axisMsgs := func() int {
		n := 0
		for _, s := range texts(a) {
			if strings.Contains(s, " axis ") {
				n++
			}
		}
		return n
	}
	in.GamepadAxis(3, sdl.GAMEPAD_AXIS_LEFTX, 1000)
	in.GamepadAxis(3, sdl.GAMEPAD_AXIS_LEFTY, 2000)
	feed(t, a)
	if axisMsgs() != 1 {
		t.Fatalf("burst produced %d axis messages, want 1", axisMsgs())
	}
	if got := texts(a)[1]; got != "Gamepad #3 axis leftx -> 1000" {
		t.Errorf("message = %q", got)
	}
	clock.Advance(motionEventCooldown - 1)
	in.GamepadAxis(3, sdl.GAMEPAD_AXIS_RIGHTX, -5)
	feed(t, a)
	if axisMsgs() != 1 {
		t.Errorf("event inside the cooldown was reported")
	}
	clock.Advance(1)
	in.GamepadAxis(3, sdl.GAMEPAD_AXIS_RIGHTX, -5)
	feed(t, a)
	if axisMsgs() != 2 || texts(a)[2] != "Gamepad #3 axis rightx -> -5" {
		t.Errorf("event at the cooldown boundary: %q", texts(a))
	}
}

func TestVirtualAxisReported(t *testing.T) {
	a, clock := setup(t)
	pad := sdltest.AttachGamepad(t, "Pad")
	feed(t, a)
	clock.Advance(motionEventCooldown) // the initial-state axis event used the cooldown
	pad.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, 9000)
	feed(t, a)
	// SDL reports every axis on the first update, but leftx comes first and
	// the cooldown suppresses the rest of the burst.
	want := fmt.Sprintf("Gamepad #%d axis leftx -> 9000", pad.ID)
	if got := texts(a); got[len(got)-1] != want {
		t.Errorf("messages = %q, want last %q", got, want)
	}
}

func pushBattery(t *testing.T, id sdl.JoystickID, st sdl.PowerState, pct int32) {
	t.Helper()
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_JOYSTICK_BATTERY_UPDATED)
	b := e.Jbattery()
	b.Which, b.State, b.Percent = id, st, pct
	if !sdltest.Call(t, func() bool { return sdl.PushEvent(e) }) {
		t.Fatal("PushEvent failed")
	}
}

func TestBatteryOnlyForGamepads(t *testing.T) {
	a, _ := setup(t)
	pad := sdltest.AttachGamepad(t, "Pad")
	feed(t, a)
	base := len(a.messages)
	pushBattery(t, 9999, sdl.POWERSTATE_CHARGING, 50) // not a gamepad
	feed(t, a)
	if len(a.messages) != base {
		t.Fatalf("battery of a non-gamepad was reported: %q", texts(a)[base:])
	}
	pushBattery(t, pad.ID, sdl.POWERSTATE_ON_BATTERY, 42)
	feed(t, a)
	if got := texts(a)[base:]; len(got) != 1 || got[0] != fmt.Sprintf("Gamepad #%d battery -> ON BATTERY - 42%%", pad.ID) {
		t.Errorf("got %q", got)
	}
}

func TestBatteryStateString(t *testing.T) {
	for st, want := range map[sdl.PowerState]string{
		sdl.POWERSTATE_ERROR:      "ERROR",
		sdl.POWERSTATE_UNKNOWN:    "UNKNOWN",
		sdl.POWERSTATE_ON_BATTERY: "ON BATTERY",
		sdl.POWERSTATE_NO_BATTERY: "NO BATTERY",
		sdl.POWERSTATE_CHARGING:   "CHARGING",
		sdl.POWERSTATE_CHARGED:    "CHARGED",
		sdl.PowerState(99):        "UNKNOWN",
	} {
		if got := batteryStateString(st); got != want {
			t.Errorf("batteryStateString(%d) = %q, want %q", st, got, want)
		}
	}
}

var (
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
)

// countNot counts pixels in r that differ from black.
func countNot(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) != black {
				n++
			}
		}
	}
	return n
}

func TestDrawInitialFrame(t *testing.T) {
	a, _ := setup(t)
	c := sdltest.NewCanvas(t, 640, 480)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	img := c.Capture(t)
	msg := a.messages[0].str
	w := len(msg) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	x0 := (640 - w) / 2
	// Fresh message sits at the top, horizontally centered, opaque white.
	if countNot(img, image.Rect(x0, 0, x0+w, 8)) == 0 {
		t.Error("no text pixels in the top centered band")
	}
	if n := countNot(img, image.Rect(0, 0, x0, 480)) + countNot(img, image.Rect(x0+w, 0, 640, 480)) + countNot(img, image.Rect(0, 8, 640, 480)); n != 0 {
		t.Errorf("%d unexpected non-black pixels outside the text band", n)
	}
	for y := 0; y < 8; y++ {
		for x := x0; x < x0+w; x++ {
			if p := sdltest.PixelAt(img, x, y); p != black && p != white {
				t.Fatalf("pixel (%d,%d) = %v, want black or opaque white", x, y, p)
			}
		}
	}
}

func TestDrawMessageFallsAndFades(t *testing.T) {
	a, clock := setup(t)
	c := sdltest.NewCanvas(t, 640, 480)
	clock.Advance(msgLifetime / 2) // 1750 ms: halfway down, half alpha
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	img := c.Capture(t)
	if n := countNot(img, image.Rect(0, 0, 640, 236)) + countNot(img, image.Rect(0, 249, 640, 480)); n != 0 {
		t.Errorf("%d pixels outside the half-way band", n)
	}
	maxR := uint8(0)
	for y := 240; y < 248; y++ {
		for x := 0; x < 640; x++ {
			if p := sdltest.PixelAt(img, x, y); p.R > maxR {
				maxR = p.R
			}
		}
	}
	if maxR < 120 || maxR > 135 {
		t.Errorf("brightest text pixel R = %d, want about 127 (half faded)", maxR)
	}
}

func TestDrawExpiresMessages(t *testing.T) {
	a, clock := setup(t)
	c := sdltest.NewCanvas(t, 640, 480)
	clock.Advance(msgLifetime - 1)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	if len(a.messages) != 1 {
		t.Fatalf("message expired early")
	}
	clock.Advance(1)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	if len(a.messages) != 0 {
		t.Errorf("message survived its lifetime: %q", texts(a))
	}
	if n := countNot(c.Capture(t), image.Rect(0, 0, 640, 480)); n != 0 {
		t.Errorf("%d pixels drawn with no messages", n)
	}
}

func TestDrawStaggersCrowdedMessages(t *testing.T) {
	a, clock := setup(t)
	a.addMessage(0, "second")
	a.addMessage(0, "third")
	c := sdltest.NewCanvas(t, 640, 480)
	clock.Advance(50) // all three are 50ms old: same y, closer than one glyph
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	// The second message restarts its timer and the third is not visited.
	if a.messages[0].startTicks != 0 || a.messages[1].startTicks != 50 || a.messages[2].startTicks != 0 {
		t.Errorf("start ticks = %d %d %d, want 0 50 0", a.messages[0].startTicks, a.messages[1].startTicks, a.messages[2].startTicks)
	}
}

func TestDrawFollowsWindowSize(t *testing.T) {
	a, clock := setup(t)
	c := sdltest.NewCanvas(t, 800, 600)
	clock.Advance(msgLifetime / 2)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 800, 600) })
	img := c.Capture(t)
	w := len(a.messages[0].str) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	x0 := (800 - w) / 2
	if countNot(img, image.Rect(x0, 300, x0+w, 308)) == 0 {
		t.Error("text not centered at y = 300 for an 800x600 window")
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
