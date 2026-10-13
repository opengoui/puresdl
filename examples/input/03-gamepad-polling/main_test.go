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

var (
	white = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	green = color.RGBA{0x00, 0xFF, 0x00, 0xFF}
	blue  = color.RGBA{0x00, 0x00, 0xFF, 0xFF}
)

// fixture bundles the app, its canvas and the injectable clock.
type fixture struct {
	a      *app
	canvas *sdltest.Canvas
	clock  *sdltest.Clock
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	f := &fixture{clock: new(sdltest.Clock)}
	f.canvas = sdltest.NewCanvas(t, windowWidth, windowHeight)
	f.a = newApp(f.clock.Ticks)
	t.Cleanup(func() { sdltest.OnMain(t, f.a.close) })
	var err error
	sdltest.OnMain(t, func() { err = f.a.loadTexture(f.canvas.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// feed pumps SDL and applies every queued event like the run loop.
func (f *fixture) feed(t *testing.T) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if f.a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func (f *fixture) frame(t *testing.T) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() { f.a.draw(f.canvas.Renderer) })
	return f.canvas.Capture(t)
}

// plug attaches a gamepad and lets the app open it.
func (f *fixture) plug(t *testing.T, name string) *sdltest.VirtualJoystick {
	t.Helper()
	vj := sdltest.AttachGamepad(t, name)
	f.feed(t)
	if f.a.gamepad == nil {
		t.Fatal("gamepad was not opened after GAMEPAD_ADDED")
	}
	return vj
}

func center(r sdl.FRect) (int, int) { return int(r.X + r.W/2), int(r.Y + r.H/2) }

func TestNoGamepadShowsPrompt(t *testing.T) {
	f := newFixture(t)
	f.feed(t)
	img := f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, windowWidth, 100), white)
	sdltest.AssertRegion(t, img, image.Rect(0, 400, windowWidth, windowHeight), white)

	// The prompt is centered: 26 characters of 8 pixels each, 8 pixels tall.
	text := "Plug in a gamepad, please."
	w := len(text) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	x0 := (windowWidth - w) / 2
	y0 := (windowHeight - sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE) / 2
	blueIn := func(r image.Rectangle) int {
		n := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if img.RGBAAt(x, y) == blue {
					n++
				}
			}
		}
		return n
	}
	if blueIn(image.Rect(x0, y0, x0+w, y0+sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)) == 0 {
		t.Error("no blue text pixels in the centered prompt box")
	}
	if n := blueIn(image.Rect(0, 0, windowWidth, y0)) + blueIn(image.Rect(0, y0+8, windowWidth, windowHeight)); n != 0 {
		t.Errorf("%d blue pixels outside the prompt row", n)
	}
	if n := blueIn(image.Rect(0, y0, x0, y0+8)) + blueIn(image.Rect(x0+w, y0, windowWidth, y0+8)); n != 0 {
		t.Errorf("%d blue pixels beside the prompt", n)
	}
}

func TestQuitEvent(t *testing.T) {
	f := newFixture(t)
	sdltest.NewInput(t, 0).Quit()
	if !f.feed(t) {
		t.Error("EVENT_QUIT did not request quit")
	}
}

func TestOtherEventsDoNotQuit(t *testing.T) {
	f := newFixture(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.Press(sdl.SCANCODE_Q)
	in.CloseRequested()
	in.Resized(300, 200)
	if f.feed(t) {
		t.Error("only EVENT_QUIT should end the program")
	}
}

func TestHotplugOpensGamepadAndShowsName(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	var id sdl.JoystickID
	sdltest.OnMain(t, func() { id = sdl.GetGamepadID(f.a.gamepad) })
	if id != vj.ID {
		t.Errorf("opened gamepad ID %d, want %d", id, vj.ID)
	}
	img := f.frame(t)
	// The name is drawn on the bottom row instead of the center prompt.
	name := sdltest.Call(t, func() string { return sdl.GetGamepadName(f.a.gamepad) })
	if name == "" {
		t.Fatal("empty gamepad name")
	}
	w := len(name) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	x0 := (windowWidth - w) / 2
	y0 := windowHeight - (sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE + 2)
	found := false
	for y := y0; y < y0+8 && !found; y++ {
		for x := x0; x < x0+w; x++ {
			if img.RGBAAt(x, y) == blue {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("gamepad name not drawn on the bottom row")
	}
}

func TestSecondGamepadIsIgnored(t *testing.T) {
	f := newFixture(t)
	first := f.plug(t, "First")
	second := sdltest.AttachGamepad(t, "Second")
	f.feed(t)
	if got := sdltest.Call(t, func() sdl.JoystickID { return sdl.GetGamepadID(f.a.gamepad) }); got != first.ID {
		t.Errorf("tracking gamepad %d, want first %d", got, first.ID)
	}
	// Unplugging the unused one must keep the first open.
	sdltest.OnMain(t, func() { sdl.CloseJoystick(second.Joystick); sdl.DetachVirtualJoystick(second.ID) })
	second.Joystick = nil
	f.feed(t)
	if f.a.gamepad == nil {
		t.Error("removing an unrelated gamepad closed the tracked one")
	}
}

func TestUnplugReturnsToPrompt(t *testing.T) {
	f := newFixture(t)
	before := f.frame(t)
	vj := f.plug(t, "Test Pad")
	with := f.frame(t)
	if sameImage(before, with) {
		t.Fatal("drawing with a gamepad should differ from the prompt screen")
	}
	sdltest.OnMain(t, func() { sdl.CloseJoystick(vj.Joystick); sdl.DetachVirtualJoystick(vj.ID) })
	vj.Joystick = nil
	f.feed(t)
	if f.a.gamepad != nil {
		t.Fatal("gamepad still open after GAMEPAD_REMOVED")
	}
	if !sameImage(before, f.frame(t)) {
		t.Error("frame after unplug differs from the initial prompt frame")
	}
}

func TestReplugOpensAgain(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "One")
	sdltest.OnMain(t, func() { sdl.CloseJoystick(vj.Joystick); sdl.DetachVirtualJoystick(vj.ID) })
	vj.Joystick = nil
	f.feed(t)
	if f.a.gamepad != nil {
		t.Fatal("still open")
	}
	f.plug(t, "Two")
}

func sameImage(a, b *image.RGBA) bool {
	return a.Rect == b.Rect && string(a.Pix) == string(b.Pix)
}

func TestButtonsDrawGreenBoxes(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	base := f.frame(t)
	for i, r := range buttons {
		x, y := center(r)
		if base.RGBAAt(x, y) == green {
			t.Fatalf("button %d: baseline already green at (%d,%d)", i, x, y)
		}
	}
	for i, r := range buttons {
		b := sdl.GamepadButton(i)
		vj.SetGamepadButton(b, true)
		f.feed(t)
		img := f.frame(t)
		sdltest.AssertRegion(t, img, image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H)), green)
		// Every other button stays unpainted.
		for j, o := range buttons {
			if j == i {
				continue
			}
			ox, oy := center(o)
			if img.RGBAAt(ox, oy) == green {
				t.Errorf("button %d pressed but button %d box is green", i, j)
			}
		}
		vj.SetGamepadButton(b, false)
		f.feed(t)
		x, y := center(r)
		if f.frame(t).RGBAAt(x, y) == green {
			t.Errorf("button %d still green after release", i)
		}
	}
}

func TestLeftStickBoxFollowsAxisAndFades(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	base := f.frame(t)
	// Full right: box spans x 137..167, y 252..282.
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, 32767)
	f.feed(t)
	f.clock.Advance(1000)
	img := f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(137, 252, 167, 282), blue)
	if base.RGBAAt(110, 255) == blue {
		t.Fatal("baseline already blue at probe point")
	}
	// Moving 499 ms later still draws (resting position at the same axis).
	f.clock.Advance(499)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, 0)
	f.feed(t)
	img = f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(107, 252, 137, 282), blue)
	// Exactly 500 ms after the last movement frame it disappears.
	f.clock.Advance(499)
	if f.frame(t).RGBAAt(120, 265) != base.RGBAAt(120, 265) {
		t.Error("box should still be visible 499 ms after movement")
	}
	f.clock.Advance(1)
	img = f.frame(t)
	if !sameImage(img, base) {
		t.Error("frame 500 ms after the last movement should equal the idle frame")
	}
}

func TestStickDeadZone(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	f.clock.Advance(10000)
	base := f.frame(t)
	// 1000 is not above the threshold; the sticks stay hidden.
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, 1000)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHTY, -1000)
	f.feed(t)
	if !sameImage(f.frame(t), base) {
		t.Error("axis value 1000 must not draw a stick box")
	}
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, -1001)
	f.feed(t)
	if sameImage(f.frame(t), base) {
		t.Error("axis value -1001 must draw a stick box")
	}
}

func TestRightStickBox(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	f.clock.Advance(10000)
	// Right stick pushed left and down: box at about (367, 400).
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHTX, -32768)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHTY, 32767)
	f.feed(t)
	img := f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(368, 400, 396, 428), blue)
}

// A virtual trigger axis reports raw values in -32768..32767 (unpressed to
// fully pressed); SDL rescales them to the gamepad range 0..32767.
func TestTriggersDrawBars(t *testing.T) {
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	f.clock.Advance(10000)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFT_TRIGGER, -32768)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHT_TRIGGER, -32768)
	f.feed(t)
	base := f.frame(t)

	// Full left trigger: bar 37 wide, 65 tall from y=1.
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFT_TRIGGER, 32767)
	f.feed(t)
	img := f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(128, 2, 163, 65), blue)
	if base.RGBAAt(145, 30) == blue {
		t.Fatal("baseline already blue")
	}

	// Half right trigger: height ~32.5, anchored at the bottom (y=66).
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFT_TRIGGER, -32768)
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHT_TRIGGER, 0)
	f.feed(t)
	img = f.frame(t)
	sdltest.AssertRegion(t, img, image.Rect(483, 40, 516, 65), blue)
	if img.RGBAAt(500, 10) == blue {
		t.Error("half trigger must not fill the top of the bar area")
	}
	if img.RGBAAt(145, 20) == blue {
		t.Error("released left trigger still drawn")
	}

	// A polled value of exactly 1000 is not drawn, 1002 is.
	axis := func() int16 {
		return sdltest.Call(t, func() int16 { return sdl.GetGamepadAxis(f.a.gamepad, sdl.GAMEPAD_AXIS_RIGHT_TRIGGER) })
	}
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHT_TRIGGER, -30767)
	f.feed(t)
	if v := axis(); v != 1000 {
		t.Fatalf("scaled trigger = %d, want 1000", v)
	}
	if !sameImage(f.frame(t), base) {
		t.Error("trigger value 1000 must not draw")
	}
	vj.SetGamepadAxis(sdl.GAMEPAD_AXIS_RIGHT_TRIGGER, -30765)
	f.feed(t)
	if v := axis(); v <= 1000 {
		t.Fatalf("scaled trigger = %d, want > 1000", v)
	}
	if sameImage(f.frame(t), base) {
		t.Error("trigger above 1000 must draw")
	}
}

func TestPolledStateWithoutEvents(t *testing.T) {
	// Drawing reads device state directly, so a frame drawn before the event
	// is pumped already shows the button.
	f := newFixture(t)
	vj := f.plug(t, "Test Pad")
	vj.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, true)
	sdltest.OnMain(t, sdl.UpdateGamepads)
	x, y := center(buttons[sdl.GAMEPAD_BUTTON_SOUTH])
	sdltest.AssertPixel(t, f.frame(t), x, y, green)
}

func TestEventAccessorRoundTrip(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	vj := sdltest.AttachGamepad(t, "RT")
	var added, removed []sdl.JoystickID
	collect := func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			switch sdl.EventType(e.Type()) {
			case sdl.EVENT_GAMEPAD_ADDED:
				added = append(added, e.Gdevice().Which)
			case sdl.EVENT_GAMEPAD_REMOVED:
				removed = append(removed, e.Gdevice().Which)
			}
		}
	}
	sdltest.OnMain(t, collect)
	sdltest.OnMain(t, func() { sdl.CloseJoystick(vj.Joystick); sdl.DetachVirtualJoystick(vj.ID) })
	vj.Joystick = nil
	sdltest.OnMain(t, collect)
	if len(added) != 1 || added[0] != vj.ID {
		t.Errorf("added = %v, want [%d]", added, vj.ID)
	}
	if len(removed) != 1 || removed[0] != vj.ID {
		t.Errorf("removed = %v, want [%d]", removed, vj.ID)
	}
}

func TestPNGTextureLoaded(t *testing.T) {
	f := newFixture(t)
	if f.a.texture == nil {
		t.Fatal("texture not created")
	}
	var w, h float32
	sdltest.OnMain(t, func() { sdl.GetTextureSize(f.a.texture, &w, &h) })
	if w <= 0 || h <= 0 {
		t.Errorf("texture size %vx%v", w, h)
	}
	// With a gamepad the picture fills the window, so it is no longer all white.
	f.plug(t, "Test Pad")
	img := f.frame(t)
	nonWhite := 0
	for y := 120; y < 360; y += 4 {
		for x := 0; x < windowWidth; x += 4 {
			if img.RGBAAt(x, y) != white {
				nonWhite++
			}
		}
	}
	if nonWhite == 0 {
		t.Error("gamepad picture not drawn")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.plug(t, "Test Pad")
	sdltest.OnMain(t, f.a.close)
	if f.a.gamepad != nil || f.a.texture != nil {
		t.Error("close left handles behind")
	}
	sdltest.OnMain(t, f.a.close)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}
