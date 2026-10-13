package main

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var background = color.RGBA{0x42, 0x87, 0xf5, 0xff}

func newTestApp(t *testing.T, w, h int32) (*app, *sdltest.Canvas, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, w, h)
	var clock sdltest.Clock
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp(c.Renderer, clock.Ticks) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a, c, &clock
}

func frameAt(t *testing.T, a *app, c *sdltest.Canvas, clock *sdltest.Clock, ms uint64) *image.RGBA {
	t.Helper()
	clock.Advance(ms - clock.Ticks())
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	return c.Capture(t)
}

func TestInitialFrameDrawsCubeOnBackground(t *testing.T) {
	a, c, clock := newTestApp(t, windowWidth, windowHeight)
	img := frameAt(t, a, c, clock, 0)
	// At t=0 the rotation is the identity: a square of side 480/sqrt(3)
	// (about 277px) centred at (320,240).
	for _, p := range [][2]int{{5, 5}, {634, 474}, {100, 240}, {320, 50}, {540, 240}} {
		sdltest.AssertPixel(t, img, p[0], p[1], background)
	}
	if got := sdltest.PixelAt(img, 320, 240); got == background || got.A != 0xff {
		t.Errorf("centre pixel = %v, want opaque texture colour", got)
	}
}

func TestRotationIsPeriodicOver2000ms(t *testing.T) {
	a, c, clock := newTestApp(t, windowWidth, windowHeight)
	a1 := frameAt(t, a, c, clock, 500)
	b := bytes.Clone(a1.Pix)
	a2 := frameAt(t, a, c, clock, 2500)
	if !bytes.Equal(b, a2.Pix) {
		t.Error("frame at 500ms differs from frame at 2500ms")
	}
}

func TestCubeRotatesOverTime(t *testing.T) {
	a, c, clock := newTestApp(t, windowWidth, windowHeight)
	f0 := bytes.Clone(frameAt(t, a, c, clock, 0).Pix)
	f1 := frameAt(t, a, c, clock, 500).Pix
	if bytes.Equal(f0, f1) {
		t.Error("frames at 0ms and 500ms are identical")
	}
}

func TestDrawRedrawsBackgroundEachFrame(t *testing.T) {
	a, c, clock := newTestApp(t, windowWidth, windowHeight)
	frameAt(t, a, c, clock, 0)
	// Corners stay background across frames: no trail from earlier frames.
	for _, ms := range []uint64{250, 700, 1300} {
		img := frameAt(t, a, c, clock, ms)
		sdltest.AssertPixel(t, img, 2, 2, background)
		sdltest.AssertPixel(t, img, windowWidth-3, windowHeight-3, background)
	}
}

func TestLogicalPresentationScalesCube(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 2*windowWidth, 2*windowHeight)
	var a *app
	var err error
	var clock sdltest.Clock
	sdltest.OnMain(t, func() {
		sdl.SetRenderLogicalPresentation(c.Renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		a, err = newApp(c.Renderer, clock.Ticks)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	img := c.Capture(t)
	// At t=0 the cube spans logical x 181..459; at 2x that is 362..918.
	sdltest.AssertPixel(t, img, 350, 480, background)
	if got := sdltest.PixelAt(img, 900, 480); got == background {
		t.Errorf("pixel (900,480) = %v, want cube texture at 2x scale", got)
	}
}

func TestHandleEventQuitOnly(t *testing.T) {
	a, _, _ := newTestApp(t, 64, 64)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(100, 100)
	in.Quit()
	var quits int
	var others int
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quits++
				if sdl.EventType(e.Type()) != sdl.EVENT_QUIT {
					others++
				}
			}
		}
	})
	if quits != 1 || others != 0 {
		t.Errorf("quits=%d (non-quit=%d), want exactly the EVENT_QUIT", quits, others)
	}
}

func TestEmbeddedTextureSize(t *testing.T) {
	a, _, _ := newTestApp(t, 64, 64)
	var w, h float32
	var ok bool
	sdltest.OnMain(t, func() { ok = sdl.GetTextureSize(a.texture, &w, &h) })
	if !ok || w <= 0 || h <= 0 {
		t.Fatalf("texture size = %v x %v ok=%v", w, h, ok)
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
