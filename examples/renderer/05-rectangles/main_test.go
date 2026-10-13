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
	red   = color.RGBA{255, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
	white = color.RGBA{255, 255, 255, 255}
)

// frameAt renders one frame with the clock at the given tick.
func frameAt(t *testing.T, ms uint64) *sdltest.Canvas {
	t.Helper()
	sdltest.Skip(t)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a := newApp(func() uint64 { return ms })
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	return c
}

// At tick 500 the scale is zero, so shapes have their base size.
func TestFrameAtRestScale(t *testing.T) {
	img := frameAt(t, 500).Capture(t)
	// Red outline square at (100,100) size 100: corner on, interior off.
	sdltest.AssertPixel(t, img, 100, 100, red)
	sdltest.AssertPixel(t, img, 150, 100, red)
	sdltest.AssertPixel(t, img, 150, 150, black)
	// Three concentric green outlines (sizes 50, 100, 150) centred on the window.
	sdltest.AssertPixel(t, img, 320, 215, green)
	sdltest.AssertPixel(t, img, 320, 190, green)
	sdltest.AssertPixel(t, img, 320, 165, green)
	sdltest.AssertPixel(t, img, 320, 240, black)
	// Filled blue rectangle 400,50 100x50.
	sdltest.AssertRegion(t, img, rectOf(401, 51, 98, 48), blue)
	sdltest.AssertPixel(t, img, 399, 75, black)
	sdltest.AssertPixel(t, img, 400, 50, blue)   // top-left corner
	sdltest.AssertPixel(t, img, 499, 99, blue)   // bottom-right corner
	sdltest.AssertPixel(t, img, 500, 75, black)  // just right of the 100 px width
	sdltest.AssertPixel(t, img, 450, 100, black) // just below the 50 px height
	sdltest.AssertPixel(t, img, 450, 49, black)  // just above
	// Bar chart: the last bar is 40x120 and white, the first has zero height.
	sdltest.AssertRegion(t, img, rectOf(601, 361, 38, 118), white)
	sdltest.AssertPixel(t, img, 20, 479, black)
	sdltest.AssertPixel(t, img, 60, 476, white)
	sdltest.AssertPixel(t, img, 60, 470, black)
}

// At tick 0 the scale is +1, so shapes are at double size.
func TestFrameAtMaxScale(t *testing.T) {
	img := frameAt(t, 0).Capture(t)
	sdltest.AssertPixel(t, img, 100, 100, red)
	sdltest.AssertPixel(t, img, 290, 100, red) // square is now 200 wide
	sdltest.AssertRegion(t, img, rectOf(401, 51, 198, 98), blue)
	sdltest.AssertPixel(t, img, 599, 140, blue)
	sdltest.AssertPixel(t, img, 400, 50, blue)
	sdltest.AssertPixel(t, img, 599, 149, blue)  // 200x100 at double size
	sdltest.AssertPixel(t, img, 600, 100, black) // just right of the width
	sdltest.AssertPixel(t, img, 450, 150, black) // just below the height
}

// At tick 1000 the scale is -1: the filled rectangle vanishes, bars remain.
func TestFrameAtMinScale(t *testing.T) {
	img := frameAt(t, 1000).Capture(t)
	sdltest.AssertRegion(t, img, rectOf(380, 40, 240, 120), black)
	sdltest.AssertRegion(t, img, rectOf(601, 361, 38, 118), white)
}

// The animation depends only on now modulo 2000.
func TestAnimationPeriodIs2000ms(t *testing.T) {
	a := frameAt(t, 123).Capture(t)
	b := frameAt(t, 2123).Capture(t)
	if string(a.Pix) != string(b.Pix) {
		t.Error("frames 2000ms apart differ")
	}
	c := frameAt(t, 623).Capture(t)
	if string(a.Pix) == string(c.Pix) {
		t.Error("frames 500ms apart are identical; animation is not running")
	}
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	sdltest.Skip(t)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var ms uint64
	a := newApp(func() uint64 { return ms })
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	ms = 1000
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	// The blue fill of the first frame (tick 0) must be gone.
	sdltest.AssertRegion(t, c.Capture(t), rectOf(380, 40, 240, 120), black)
}

func TestHandleEventQuitOnly(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(func() uint64 { return 0 })
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(1, 1, 0, 0, 0)
	in.Resized(100, 100)
	in.CloseRequested()
	var quit bool
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	if quit {
		t.Error("non-quit events caused quit")
	}
	in.Quit()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	if !quit {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestConfigureLogicalPresentation(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 1280, 480)
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	var r sdl.FRect
	sdltest.OnMain(t, func() {
		configure(c.Renderer)
		sdl.GetRenderLogicalPresentation(c.Renderer, &w, &h, &mode)
		sdl.GetRenderLogicalPresentationRect(c.Renderer, &r)
	})
	if w != windowWidth || h != windowHeight || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("got %dx%d mode %d", w, h, mode)
	}
	// 640x480 letterboxed into 1280x480 is centred: 320px bars each side.
	if r.X != 320 || r.Y != 0 || r.W != 640 || r.H != 480 {
		t.Errorf("presentation rect = %+v", r)
	}
	a := newApp(func() uint64 { return 500 })
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	img := c.Capture(t)
	// RenderReadPixels with a nil rect returns the logical area only, so the
	// letterbox bars are not observable; drawing still lands in logical space.
	if got := img.Bounds(); got != rectOf(0, 0, windowWidth, windowHeight) {
		t.Errorf("capture bounds = %v", got)
	}
	sdltest.AssertPixel(t, img, 100, 100, red)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}

func rectOf(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
