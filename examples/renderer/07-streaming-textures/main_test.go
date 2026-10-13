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
	grey  = color.RGBA{66, 66, 66, 255}
	black = color.RGBA{0, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
)

const (
	texX = (windowWidth - textureSize) / 2
	texY = (windowHeight - textureSize) / 2
	bar  = textureSize / 10
)

// newTestApp creates an app on a 640x480 offscreen canvas driven by clock.
func newTestApp(t *testing.T) (*app, *sdltest.Canvas, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
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

// TestStripPosition checks the green strip over time. Expected rows are the
// contract from the C source: the strip sweeps from the texture bottom (t=0)
// to the top (t=1000) and back down.
func TestStripPosition(t *testing.T) {
	cases := []struct {
		now  uint64
		row  int // strip top, in texture rows
		name string
	}{
		{0, 135, "start at bottom"},
		{500, 67, "middle going up"},
		{999, 0, "nearly top"},
		{1000, 0, "top"},
		{1500, 67, "middle going down"},
		{1999, 134, "nearly bottom"},
		{2000, 135, "wraps to bottom"},
	}
	a, c, clock := newTestApp(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock.Advance(tc.now - clock.Ticks())
			sdltest.OnMain(t, a.draw)
			img := sdltest.Capture(t, c.Renderer)
			// Column inside the texture: scan its rows.
			for y := 0; y < textureSize; y++ {
				want := black
				if y >= tc.row && y < tc.row+bar {
					want = green
				}
				sdltest.AssertPixel(t, img, texX+75, texY+y, want)
				if t.Failed() {
					return
				}
			}
		})
	}
}

func TestFrameLayout(t *testing.T) {
	a, c, _ := newTestApp(t)
	sdltest.OnMain(t, a.draw)
	img := sdltest.Capture(t, c.Renderer)

	// Grey background everywhere outside the centered texture.
	sdltest.AssertPixel(t, img, 0, 0, grey)
	sdltest.AssertPixel(t, img, windowWidth-1, windowHeight-1, grey)
	sdltest.AssertPixel(t, img, texX-1, texY, grey)
	sdltest.AssertPixel(t, img, texX+textureSize, texY+textureSize-1, grey)
	sdltest.AssertPixel(t, img, texX, texY-1, grey)
	sdltest.AssertPixel(t, img, texX, texY+textureSize, grey)
	// Texture corners (at t=0 the strip is at the bottom).
	sdltest.AssertPixel(t, img, texX, texY, black)
	sdltest.AssertPixel(t, img, texX+textureSize-1, texY, black)
	sdltest.AssertRegion(t, img, imageRect(texX, texY+135, textureSize, bar), green)
}

// TestRedrawReplacesPreviousFrame verifies that each frame fully rewrites the
// texture: the old strip position must not linger.
func TestRedrawReplacesPreviousFrame(t *testing.T) {
	a, c, clock := newTestApp(t)
	sdltest.OnMain(t, a.draw)
	clock.Advance(1000)
	sdltest.OnMain(t, a.draw)
	img := sdltest.Capture(t, c.Renderer)
	sdltest.AssertRegion(t, img, imageRect(texX, texY, textureSize, bar), green)
	sdltest.AssertRegion(t, img, imageRect(texX, texY+bar, textureSize, textureSize-bar), black)
}

func TestHandleEvent(t *testing.T) {
	a, _, _ := newTestApp(t)
	in := sdltest.NewInput(t, 0)
	// Non-quit events must not end the program.
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(800, 600)
	in.CloseRequested()
	for _, e := range sdltest.Drain(t) {
		if sdl.EventType(e.Type()) == sdl.EVENT_QUIT {
			t.Fatal("unexpected quit in queue")
		}
		var quit bool
		ev := e
		sdltest.OnMain(t, func() { quit = a.handleEvent(&ev) })
		if quit {
			t.Errorf("event type %#x ended the program", e.Type())
		}
	}
	sdltest.NewInput(t, 0).Quit()
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
	if !quit {
		t.Error("EVENT_QUIT did not quit")
	}
}

// TestLetterbox renders to a window renderer of a different size and checks
// that the 640x480 logical frame is letterboxed and the texture scaled.
func TestLetterbox(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 1280, 480) // wider than 4:3: pillarbox, scale 1
	var r *sdl.Renderer
	var a *app
	var err error
	sdltest.OnMain(t, func() {
		r = sdl.CreateRenderer(win, "software")
		if r == nil {
			return
		}
		a, err = newApp(r, func() uint64 { return 0 })
	})
	if r == nil {
		t.Skip("software window renderer unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { a.close(); sdl.DestroyRenderer(r) }) })

	var w, h int32
	var mode sdl.RendererLogicalPresentation
	var rect sdl.FRect
	sdltest.OnMain(t, func() {
		sdl.GetRenderLogicalPresentation(r, &w, &h, &mode)
		sdl.GetRenderLogicalPresentationRect(r, &rect)
		a.draw()
	})
	if w != windowWidth || h != windowHeight || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Fatalf("logical presentation = %dx%d mode %v", w, h, mode)
	}
	if rect != (sdl.FRect{X: 320, Y: 0, W: 640, H: 480}) {
		t.Fatalf("presentation rect = %+v", rect)
	}
	// Window coordinates map into the logical frame offset by the bars.
	var lx, ly float32
	sdltest.OnMain(t, func() { sdl.RenderCoordinatesFromWindow(r, 320+texX+75, 100, &lx, &ly) })
	if lx != texX+75 || ly != 100 {
		t.Errorf("window->logical = (%v,%v), want (%d,100)", lx, ly, texX+75)
	}
	// Readback covers the logical frame, not the bars.
	img := sdltest.Capture(t, r)
	if b := img.Bounds(); b.Dx() != windowWidth || b.Dy() != windowHeight {
		t.Fatalf("readback size = %v", b)
	}
	sdltest.AssertPixel(t, img, texX+75, texY+135+5, green)
	sdltest.AssertPixel(t, img, texX+75, texY+10, black)
	sdltest.AssertPixel(t, img, 10, 10, grey)
}

// TestRunSmoke runs the real loop for a few frames with the dummy drivers.
func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}

func imageRect(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
