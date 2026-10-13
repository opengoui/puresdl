package main

import (
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

func feed(t *testing.T, c *clearer) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if c.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

// want converts a float channel the way the C program's output is observed:
// quantised to 8 bits.
func want(f float32) uint8 { return uint8(math.Round(float64(f) * 255)) }

func near(a, b uint8) bool { return a == b || a+1 == b || b+1 == a }

func assertFill(t *testing.T, img *image.RGBA, seconds float64) {
	t.Helper()
	r, g, b := colorAt(seconds)
	w := color.RGBA{want(r), want(g), want(b), 255}
	b0 := img.Bounds()
	for _, p := range []image.Point{b0.Min, {b0.Max.X - 1, b0.Max.Y - 1}, {b0.Dx() / 2, b0.Dy() / 2}} {
		got := img.RGBAAt(p.X, p.Y)
		if !near(got.R, w.R) || !near(got.G, w.G) || !near(got.B, w.B) || got.A != 255 {
			t.Errorf("t=%vs pixel %v = %v, want about %v", seconds, p, got, w)
		}
	}
}

func TestColorAtKnownValues(t *testing.T) {
	// At t=0: red=0.5, green=0.5+0.5*sin(2pi/3), blue=0.5+0.5*sin(4pi/3).
	r, g, b := colorAt(0)
	if r != 0.5 || math.Abs(float64(g)-0.9330127) > 1e-6 || math.Abs(float64(b)-0.0669873) > 1e-6 {
		t.Errorf("colorAt(0) = %v %v %v", r, g, b)
	}
	// At t=pi/2 red peaks at 1.
	if r, _, _ := colorAt(math.Pi / 2); math.Abs(float64(r)-1) > 1e-6 {
		t.Errorf("red at pi/2 = %v, want 1", r)
	}
}

func TestDrawClearsWholeTargetToTimeColor(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	var clock sdltest.Clock
	c := newClearer(clock.Ticks)
	cv := sdltest.NewCanvas(t, 64, 48)
	for _, ms := range []uint64{0, 1500, 3000, 4712} {
		clock.Advance(ms - clock.Ticks())
		sdltest.OnMain(t, func() { c.draw(cv.Renderer) })
		assertFill(t, cv.Capture(t), float64(ms)/1000)
	}
}

func TestDrawAlphaIsOpaqueAndOverwritesPrevious(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	var clock sdltest.Clock
	c := newClearer(clock.Ticks)
	cv := sdltest.NewCanvas(t, 8, 8)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(cv.Renderer, 1, 2, 3, 0)
		sdl.RenderClear(cv.Renderer)
		c.draw(cv.Renderer)
	})
	assertFill(t, cv.Capture(t), 0)
}

func TestQuitEvent(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := newClearer(new(sdltest.Clock).Ticks)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 1, 1)
	in.Resized(100, 100)
	in.MouseWheel(0, 1)
	if feed(t, c) {
		t.Error("non-quit events caused quit")
	}
	in.Quit()
	if !feed(t, c) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestConfigureSetsLetterbox(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	cv := sdltest.NewCanvas(t, 800, 480)
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	var rect sdl.FRect
	sdltest.OnMain(t, func() {
		configure(cv.Renderer)
		sdl.GetRenderLogicalPresentation(cv.Renderer, &w, &h, &mode)
		sdl.GetRenderLogicalPresentationRect(cv.Renderer, &rect)
	})
	if w != 640 || h != 480 || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("presentation = %dx%d mode %v", w, h, mode)
	}
	// 640x480 centred in 800x480: 80px bars left and right.
	if rect.X != 80 || rect.Y != 0 || rect.W != 640 || rect.H != 480 {
		t.Errorf("letterbox rect = %+v", rect)
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
