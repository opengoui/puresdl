package main

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

const (
	canvasW, canvasH = 640, 480
	cs               = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	titleY           = cs * 8
	firstRowY        = titleY + cs*2 + cs*3
	padName          = "Test Pad"
)

// feed pumps SDL and applies every queued event to a like the run loop does.
func feed(t *testing.T, a *app) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func setup(t *testing.T) *app {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	return &app{}
}

// attach plugs a virtual gamepad in and lets the app see the ADDED event.
func attach(t *testing.T, a *app, name string) *sdltest.VirtualJoystick {
	t.Helper()
	pad := sdltest.AttachGamepad(t, name)
	feed(t, a)
	return pad
}

func rowY(i int) int { return firstRowY + i*cs*2 }

// render draws a into a fresh canvas.
func render(t *testing.T, a *app) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, canvasW, canvasH)
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	return c.Capture(t)
}

// expectedText renders str centered at y in col on black, the way the example should.
func expectedText(t *testing.T, str string, y int, col color.RGBA) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, canvasW, canvasH)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 0, 0, 0, 255)
		sdl.RenderClear(c.Renderer)
		sdl.SetRenderDrawColor(c.Renderer, col.R, col.G, col.B, 255)
		sdl.RenderDebugText(c.Renderer, float32((canvasW-len(str)*cs)/2), float32(y), str)
	})
	return c.Capture(t)
}

// litPixels counts non-black pixels in the horizontal band [y, y+cs).
func litPixels(img *image.RGBA, y int) int {
	n := 0
	for yy := y; yy < y+cs; yy++ {
		for x := 0; x < canvasW; x++ {
			if p := sdltest.PixelAt(img, x, yy); p != (color.RGBA{0, 0, 0, 255}) {
				n++
			}
		}
	}
	return n
}

// bandEqual reports whether the band at y is pixel-identical in both images.
func bandEqual(a, b *image.RGBA, y int) bool {
	for yy := y; yy < y+cs; yy++ {
		for x := 0; x < canvasW; x++ {
			if sdltest.PixelAt(a, x, yy) != sdltest.PixelAt(b, x, yy) {
				return false
			}
		}
	}
	return true
}

func slotOf(a *app, id sdl.JoystickID) *gamepadInfo { return a.findGamepadInfo(id) }

func TestInitialFrameHasOnlyYellowTitle(t *testing.T) {
	a := setup(t)
	img := render(t, a)
	title := "Connect gamepads and press buttons to rumble."
	if !bandEqual(img, expectedText(t, title, titleY, color.RGBA{255, 255, 0, 255}), titleY) {
		t.Error("title band differs from yellow centered title")
	}
	sdltest.AssertPixel(t, img, 0, 0, color.RGBA{0, 0, 0, 255})
	for i := range a.gamepads {
		if n := litPixels(img, rowY(i)); n != 0 {
			t.Errorf("empty slot %d has %d lit pixels", i, n)
		}
	}
}

func TestQuitPaths(t *testing.T) {
	a := setup(t)
	in := sdltest.NewInput(t, 0)
	in.Quit()
	if !feed(t, a) {
		t.Error("EVENT_QUIT did not quit")
	}
	in.Press(sdl.SCANCODE_ESCAPE)
	if !feed(t, a) {
		t.Error("Escape did not quit")
	}
	in.Press(sdl.SCANCODE_A)
	if feed(t, a) {
		t.Error("other key quit")
	}
}

func TestHotplugAddFillsFirstFreeSlotAndRenders(t *testing.T) {
	a := setup(t)
	pad := attach(t, a, padName)
	if a.gamepads[0].id != pad.ID || a.gamepads[0].action != "idle" {
		t.Fatalf("slot 0 = %+v, want id %d idle", a.gamepads[0], pad.ID)
	}
	if sdl.GetGamepadFromID(pad.ID) == nil {
		t.Error("gamepad was not opened")
	}
	img := render(t, a)
	want := expectedText(t, sdl.GetGamepadNameForID(pad.ID)+": idle", rowY(0), color.RGBA{255, 255, 255, 255})
	if !bandEqual(img, want, rowY(0)) {
		t.Error("row 0 does not show \"<name>: idle\" in white")
	}
	if litPixels(img, rowY(1)) != 0 {
		t.Error("row 1 should be blank")
	}
}

func TestSouthAndEastRumbleAndRelease(t *testing.T) {
	a := setup(t)
	pad := attach(t, a, padName)
	for _, tc := range []struct {
		btn       sdl.GamepadButton
		low, high uint16
		action    string
	}{
		{sdl.GAMEPAD_BUTTON_SOUTH, 0xFFFF, 0, "rumble high frequency"},
		{sdl.GAMEPAD_BUTTON_EAST, 0, 0xFFFF, "rumble low frequency"},
	} {
		before := len(pad.Rumbles())
		pad.SetGamepadButton(tc.btn, true)
		feed(t, a)
		rs := pad.Rumbles()
		if len(rs) != before+1 || rs[len(rs)-1] != (sdltest.RumbleCall{Low: tc.low, High: tc.high}) {
			t.Fatalf("button %d rumbles = %+v", tc.btn, rs)
		}
		if got := slotOf(a, pad.ID).action; got != tc.action {
			t.Errorf("button %d action = %q, want %q", tc.btn, got, tc.action)
		}
		img := render(t, a)
		want := expectedText(t, sdl.GetGamepadNameForID(pad.ID)+": "+tc.action, rowY(0), color.RGBA{255, 255, 255, 255})
		if !bandEqual(img, want, rowY(0)) {
			t.Errorf("row text for %q not rendered", tc.action)
		}
		pad.SetGamepadButton(tc.btn, false)
		feed(t, a)
		rs = pad.Rumbles()
		if rs[len(rs)-1] != (sdltest.RumbleCall{}) {
			t.Errorf("release did not stop rumble: %+v", rs[len(rs)-1])
		}
		if got := slotOf(a, pad.ID).action; got != "idle" {
			t.Errorf("after release action = %q", got)
		}
	}
}

func TestOtherButtonNeverRumbles(t *testing.T) {
	a := setup(t)
	pad := attach(t, a, padName)
	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_NORTH, true)
	feed(t, a)
	for _, r := range pad.Rumbles() {
		if r != (sdltest.RumbleCall{}) {
			t.Errorf("NORTH press rumbled %+v", r)
		}
	}
	if got := a.gamepads[0].action; got != "idle" {
		t.Errorf("action = %q, want idle", got)
	}
	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_NORTH, false)
	feed(t, a)
	// SDL may drop a stop request when the motors are already off, so only
	// require that nothing non-zero was ever sent.
	for _, r := range pad.Rumbles() {
		if r != (sdltest.RumbleCall{}) {
			t.Errorf("unexpected rumble %+v", r)
		}
	}
}

func TestRemovalFreesSlotAndClosesGamepad(t *testing.T) {
	a := setup(t)
	p1 := attach(t, a, "Pad One")
	p2 := attach(t, a, "Pad Two")
	if a.gamepads[0].id != p1.ID || a.gamepads[1].id != p2.ID {
		t.Fatalf("slots = %+v", a.gamepads[:2])
	}
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(p1.ID) })
	feed(t, a)
	if a.gamepads[0].id != 0 {
		t.Errorf("slot 0 not freed: %+v", a.gamepads[0])
	}
	if a.gamepads[1].id != p2.ID {
		t.Error("slot 1 disturbed")
	}
	img := render(t, a)
	if litPixels(img, rowY(0)) != 0 {
		t.Error("removed gamepad row still drawn")
	}
	want := expectedText(t, sdl.GetGamepadNameForID(p2.ID)+": idle", rowY(1), color.RGBA{255, 255, 255, 255})
	if !bandEqual(img, want, rowY(1)) {
		t.Error("remaining gamepad row 1 missing")
	}
	// A new device reuses the freed lowest slot.
	p3 := attach(t, a, "Pad Three")
	if a.gamepads[0].id != p3.ID {
		t.Errorf("slot 0 = %d, want new pad %d", a.gamepads[0].id, p3.ID)
	}
}

func TestMoreThanSixteenGamepadsAreIgnored(t *testing.T) {
	a := setup(t)
	var pads []*sdltest.VirtualJoystick
	for i := 0; i < len(a.gamepads)+1; i++ {
		pads = append(pads, attach(t, a, padName))
	}
	for i := range a.gamepads {
		if a.gamepads[i].id != pads[i].ID {
			t.Errorf("slot %d = %d, want %d", i, a.gamepads[i].id, pads[i].ID)
		}
	}
	if a.findGamepadInfo(pads[16].ID) != nil {
		t.Error("17th gamepad was tracked")
	}
	// Input from the untracked pad must not panic or touch tracked slots.
	pads[16].SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, true)
	feed(t, a)
	if rs := pads[16].Rumbles(); len(rs) != 1 {
		t.Errorf("untracked pad still rumbles: %+v", rs)
	}
	for i := range a.gamepads {
		if a.gamepads[i].action != "idle" {
			t.Errorf("slot %d action = %q", i, a.gamepads[i].action)
		}
	}
}

func TestEventAccessorsRoundTrip(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	in := sdltest.NewInput(t, 0)
	in.GamepadButton(7, sdl.GAMEPAD_BUTTON_EAST, true)
	in.GamepadButton(7, sdl.GAMEPAD_BUTTON_SOUTH, false)
	var got [][3]int
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if b := e.Gbutton(); sdl.EventType(e.Type()) == sdl.EVENT_GAMEPAD_BUTTON_DOWN || sdl.EventType(e.Type()) == sdl.EVENT_GAMEPAD_BUTTON_UP {
				d := 0
				if b.Down {
					d = 1
				}
				got = append(got, [3]int{int(b.Which), int(b.Button), d})
			}
		}
	})
	want := [][3]int{{7, int(sdl.GAMEPAD_BUTTON_EAST), 1}, {7, int(sdl.GAMEPAD_BUTTON_SOUTH), 0}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
