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
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
	grey  = color.RGBA{90, 90, 90, 255}
)

// feed applies every queued SDL event to g like the run loop does.
func feed(t *testing.T, g *poller) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if g.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

// newTestPoller returns a poller with distinct, known, non-black colours.
func newTestPoller(t *testing.T) *poller {
	t.Helper()
	g := Call(t)
	for i := range g.colors {
		g.colors[i] = sdl.Color{R: uint8(100 + i), G: uint8(200 - i), B: uint8(30 + 2*i), A: 255}
	}
	t.Cleanup(func() { sdltest.OnMain(t, g.close) })
	return g
}

func Call(t *testing.T) *poller { return sdltest.Call(t, newPoller) }

func col(g *poller, i int) color.RGBA {
	c := g.colors[i]
	return color.RGBA{c.R, c.G, c.B, c.A}
}

func render(t *testing.T, g *poller, w, h int32) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, w, h)
	sdltest.OnMain(t, func() { g.draw(c.Renderer, w, h) })
	return c.Capture(t)
}

func setup(t *testing.T) *poller {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	return newTestPoller(t)
}

func attachJoystick(t *testing.T, spec sdltest.JoystickSpec) *sdltest.VirtualJoystick {
	t.Helper()
	return sdltest.AttachJoystick(t, spec)
}

// attach adds a 2 axis, 3 button, 1 hat stick and lets g open it.
func attach(t *testing.T, g *poller) *sdltest.VirtualJoystick {
	t.Helper()
	js := attachJoystick(t, sdltest.JoystickSpec{Name: "Test Stick", Axes: 2, Buttons: 3, Hats: 1})
	feed(t, g)
	if g.joystick == nil {
		t.Fatal("joystick not opened after EVENT_JOYSTICK_ADDED")
	}
	return js
}

func detach(t *testing.T, js *sdltest.VirtualJoystick) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.CloseJoystick(js.Joystick)
		sdl.DetachVirtualJoystick(js.ID)
		js.Joystick = nil
	})
}

// whitePixels counts white pixels inside and outside r.
func whitePixels(img *image.RGBA, r image.Rectangle) (in, out int) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) == white {
				if image.Pt(x, y).In(r) {
					in++
				} else {
					out++
				}
			}
		}
	}
	return
}

func TestNoJoystickShowsPrompt(t *testing.T) {
	g := setup(t)
	if feed(t, g) || g.joystick != nil {
		t.Fatal("unexpected quit or joystick")
	}
	img := render(t, g, 640, 480)
	// 27 characters of 8 px, centered at (212, 236).
	in, out := whitePixels(img, image.Rect(212, 236, 212+27*8, 244))
	if in == 0 {
		t.Error("no prompt text pixels")
	}
	if out != 0 {
		t.Errorf("%d white pixels outside the prompt rectangle", out)
	}
	sdltest.AssertRegion(t, img, image.Rect(0, 0, 640, 100), black)
	sdltest.AssertRegion(t, img, image.Rect(0, 380, 640, 480), black)
}

func TestQuitEvent(t *testing.T) {
	g := setup(t)
	sdltest.NewInput(t, 0).Quit()
	if !feed(t, g) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestHotplugOpensFirstStickOnly(t *testing.T) {
	g := setup(t)
	first := attach(t, g)
	if got := sdltest.Call(t, func() string { return sdl.GetJoystickName(g.joystick) }); got != "Test Stick" {
		t.Errorf("opened %q", got)
	}
	if id := sdltest.Call(t, func() sdl.JoystickID { return sdl.GetJoystickID(g.joystick) }); id != first.ID {
		t.Errorf("opened id %d, want %d", id, first.ID)
	}
	second := attachJoystick(t, sdltest.JoystickSpec{Name: "Second", Buttons: 1})
	feed(t, g)
	if id := sdltest.Call(t, func() sdl.JoystickID { return sdl.GetJoystickID(g.joystick) }); id != first.ID {
		t.Errorf("second stick replaced the first: id %d", id)
	}
	// Removing a stick that is not ours leaves ours open.
	detach(t, second)
	feed(t, g)
	if g.joystick == nil {
		t.Error("removing an unrelated stick closed ours")
	}
}

func TestRemovalClosesAndRestoresPrompt(t *testing.T) {
	g := setup(t)
	js := attach(t, g)
	detach(t, js)
	feed(t, g)
	if g.joystick != nil {
		t.Fatal("joystick still open after removal")
	}
	img := render(t, g, 640, 480)
	in, _ := whitePixels(img, image.Rect(212, 236, 212+27*8, 244))
	if in == 0 {
		t.Error("prompt not shown after removal")
	}
	// A new stick is picked up after the old one is gone.
	attach(t, g)
}

func TestStickNameIsCentered(t *testing.T) {
	g := setup(t)
	attach(t, g)
	img := render(t, g, 640, 480)
	// "Test Stick" is 10 characters: x = (640-80)/2 = 280.
	in, _ := whitePixels(img, image.Rect(280, 236, 360, 244))
	if in == 0 {
		t.Error("name text missing")
	}
	if got := img.RGBAAt(200, 245); got != black {
		t.Errorf("prompt-width area not clear: %v", got)
	}
}

func TestIdleLayout(t *testing.T) {
	g := setup(t)
	attach(t, g)
	img := render(t, g, 640, 480)
	// Buttons: 3 blocks starting at x=275, black fill with white outline.
	for i := 0; i < 3; i++ {
		x := 275 + 30*i
		sdltest.AssertPixel(t, img, x+15, 15, black)
		sdltest.AssertPixel(t, img, x, 0, white)
		sdltest.AssertPixel(t, img, x+29, 29, white)
	}
	// Hat cross is grey, no direction lit.
	sdltest.AssertPixel(t, img, 320, 455, grey)
	sdltest.AssertPixel(t, img, 310, 465, grey)
	sdltest.AssertPixel(t, img, 330, 465, grey)
	sdltest.AssertPixel(t, img, 320, 475, grey)
	// Corners of the cross are empty.
	sdltest.AssertPixel(t, img, 308, 452, black)
	// Axes at rest draw nothing.
	sdltest.AssertRegion(t, img, image.Rect(0, 100, 640, 230), black)
}

func TestButtonsLightUpWithTheirColour(t *testing.T) {
	g := setup(t)
	js := attach(t, g)
	js.SetButton(1, true)
	feed(t, g)
	img := render(t, g, 640, 480)
	sdltest.AssertPixel(t, img, 275+30+15, 15, col(g, 1))
	sdltest.AssertPixel(t, img, 275+15, 15, black)
	sdltest.AssertPixel(t, img, 275+60+15, 15, black)
	// Outline stays white.
	sdltest.AssertPixel(t, img, 275+30, 0, white)

	js.SetButton(1, false)
	feed(t, g)
	sdltest.AssertPixel(t, render(t, g, 640, 480), 275+30+15, 15, black)
}

func TestHatDirections(t *testing.T) {
	g := setup(t)
	js := attach(t, g)
	c := col(g, 0)
	// Probe pixel per direction, within each third-sized arm.
	probes := map[string]image.Point{
		"up": {320, 455}, "right": {330, 465}, "down": {320, 475}, "left": {310, 465},
	}
	for _, tc := range []struct {
		name string
		hat  uint8
		lit  []string
	}{
		{"up", sdl.HAT_UP, []string{"up"}},
		{"right", sdl.HAT_RIGHT, []string{"right"}},
		{"down", sdl.HAT_DOWN, []string{"down"}},
		{"left", sdl.HAT_LEFT, []string{"left"}},
		{"rightdown", sdl.HAT_RIGHTDOWN, []string{"right", "down"}},
		{"leftup", sdl.HAT_LEFTUP, []string{"left", "up"}},
		{"centered", sdl.HAT_CENTERED, nil},
	} {
		js.SetHat(0, tc.hat)
		feed(t, g)
		img := render(t, g, 640, 480)
		for name, p := range probes {
			want := grey
			for _, l := range tc.lit {
				if l == name {
					want = c
				}
			}
			if got := img.RGBAAt(p.X, p.Y); got != want {
				t.Errorf("hat %s: %s arm = %v, want %v", tc.name, name, got, want)
			}
		}
	}
}

func TestAxisBarsFollowDeflection(t *testing.T) {
	g := setup(t)
	js := attach(t, g)
	js.SetAxis(0, -32767)
	js.SetAxis(1, -16384)
	feed(t, g)
	img := render(t, g, 640, 480)
	// Axis 0: dx=0, width 320 -> bar covers x 0..320 on rows 210..240.
	sdltest.AssertPixel(t, img, 5, 215, col(g, 0))
	sdltest.AssertPixel(t, img, 315, 215, col(g, 0))
	sdltest.AssertPixel(t, img, 330, 215, black)
	// Axis 1: dx~=160, width~=160 -> bar covers x 161..320 on rows 240..270.
	sdltest.AssertPixel(t, img, 100, 260, black)
	sdltest.AssertPixel(t, img, 200, 260, col(g, 1))
	sdltest.AssertPixel(t, img, 315, 260, col(g, 1))
	sdltest.AssertPixel(t, img, 330, 260, black)
}

func TestWindowSizeRecentersLayout(t *testing.T) {
	g := setup(t)
	js := attach(t, g)
	js.SetButton(0, true)
	js.SetHat(0, sdl.HAT_UP)
	feed(t, g)
	img := render(t, g, 400, 300)
	// Buttons: x=(400-90)/2=155. Hat: x=(400-60)/2+15=185, y=270.
	sdltest.AssertPixel(t, img, 155+15, 15, col(g, 0))
	sdltest.AssertPixel(t, img, 185+15, 275, col(g, 0))
	sdltest.AssertPixel(t, img, 185+15, 295, grey)
	sdltest.AssertPixel(t, img, 275+15, 15, black)
}

func TestMoreButtonsThanColoursWrap(t *testing.T) {
	g := setup(t)
	js := attachJoystick(t, sdltest.JoystickSpec{Name: "Many", Buttons: 66})
	feed(t, g)
	js.SetButton(65, true)
	feed(t, g)
	img := render(t, g, 2200, 100)
	x := (2200-66*30)/2 + 65*30
	sdltest.AssertPixel(t, img, x+15, 15, col(g, 1)) // 65 % 64
}

func TestDrawDoesNotDependOnPresent(t *testing.T) {
	// draw only renders; two draws of identical state give identical frames.
	g := setup(t)
	attach(t, g)
	a := render(t, g, 640, 480)
	b := render(t, g, 640, 480)
	if string(a.Pix) != string(b.Pix) {
		t.Error("draw is not deterministic")
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
