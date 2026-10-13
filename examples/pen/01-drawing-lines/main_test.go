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

const cw, ch = 200, 100

var (
	gray  = color.RGBA{100, 100, 100, 255}
	black = color.RGBA{0, 0, 0, 255}
)

type fixture struct {
	t  *testing.T
	a  *app
	c  *sdltest.Canvas
	in *sdltest.Input
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, cw, ch)
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp(c.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return &fixture{t, a, c, sdltest.NewInput(t, 0)}
}

// feed applies all queued events like the run loop; reports quit.
func (f *fixture) feed() (quit bool) {
	sdltest.OnMain(f.t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if f.a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return
}

func (f *fixture) frame() {
	sdltest.OnMain(f.t, f.a.draw)
}

func TestInitialFrameIsGrayCanvas(t *testing.T) {
	f := newFixture(t)
	f.frame()
	img := f.c.Capture(t)
	// Away from the debug text (top-left, y 8..16) the canvas is gray.
	sdltest.AssertRegion(t, img, imgRect(0, 40, cw, ch), gray)
	sdltest.AssertRegion(t, img, imgRect(0, 0, cw, 8), gray)
}

func TestQuitEvent(t *testing.T) {
	f := newFixture(t)
	f.in.Quit()
	if !f.feed() {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestOtherEventsDoNotQuit(t *testing.T) {
	f := newFixture(t)
	f.in.Press(sdl.SCANCODE_ESCAPE)
	f.in.PenProximity(1, true)
	if f.feed() {
		t.Error("unrelated events quit")
	}
}

func TestTiltAxesUpdateState(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_XTILT, 12.5, 0, 0)
	f.in.PenAxis(1, sdl.PEN_AXIS_YTILT, -30, 0, 0)
	f.feed()
	if f.a.tiltX != 12.5 || f.a.tiltY != -30 {
		t.Errorf("tilt = %v %v", f.a.tiltX, f.a.tiltY)
	}
	// Tilt readout is drawn: some pixel in the text band differs from the canvas.
	f.frame()
	if !anyNot(f.c.Capture(t), imgRect(0, 8, cw, 16), gray) {
		t.Error("tilt text not rendered")
	}
}

func TestPressureAxisStored(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 0.75, 0, 0)
	f.in.PenAxis(1, sdl.PEN_AXIS_DISTANCE, 0.3, 0, 0)
	f.feed()
	if f.a.pressure != 0.75 {
		t.Errorf("pressure = %v", f.a.pressure)
	}
	if f.a.tiltX != 0 || f.a.tiltY != 0 {
		t.Error("non-tilt axis changed tilt")
	}
}

func TestMotionWithoutPressureDrawsNothing(t *testing.T) {
	f := newFixture(t)
	f.in.PenMotion(1, 10, 50, 0)
	f.in.PenMotion(1, 150, 50, 0)
	f.feed()
	f.frame()
	sdltest.AssertRegion(t, f.c.Capture(t), imgRect(0, 40, cw, ch), gray)
	if f.a.prevX != -1 || f.a.prevY != -1 {
		t.Errorf("prev = %v,%v", f.a.prevX, f.a.prevY)
	}
}

func TestFirstTouchedMotionOnlyAnchors(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 1, 0, 0)
	f.in.PenMotion(1, 10, 50, 0)
	f.feed()
	f.frame()
	sdltest.AssertRegion(t, f.c.Capture(t), imgRect(0, 40, cw, ch), gray)
	if f.a.prevX != 10 || f.a.prevY != 50 {
		t.Errorf("prev = %v,%v", f.a.prevX, f.a.prevY)
	}
}

func TestFullPressureDrawsBlackLine(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 1, 0, 0)
	f.in.PenMotion(1, 10, 50, 0)
	f.in.PenMotion(1, 150, 50, 0)
	f.feed()
	f.frame()
	img := f.c.Capture(t)
	sdltest.AssertRegion(t, img, imgRect(20, 50, 140, 51), black)
	sdltest.AssertRegion(t, img, imgRect(20, 60, 140, 80), gray)
	if f.a.prevX != 150 {
		t.Errorf("prevX = %v", f.a.prevX)
	}
}

func TestPressureControlsAlpha(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 0.5, 0, 0)
	f.in.PenMotion(1, 10, 50, 0)
	f.in.PenMotion(1, 150, 50, 0)
	f.feed()
	f.frame()
	p := sdltest.PixelAt(f.c.Capture(t), 80, 50)
	// 50% black over gray 100 is about 50.
	if p.R < 45 || p.R > 55 || p.R != p.G || p.G != p.B {
		t.Errorf("pixel = %v, want ~50 gray", p)
	}
}

func TestLiftingBreaksTheStroke(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 1, 0, 0)
	f.in.PenMotion(1, 10, 50, 0)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 0, 0, 0)
	f.in.PenMotion(1, 20, 50, 0) // resets the anchor
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 1, 0, 0)
	f.in.PenMotion(1, 150, 80, 0) // re-anchors only
	f.feed()
	f.frame()
	img := f.c.Capture(t)
	sdltest.AssertRegion(t, img, imgRect(0, 40, cw, 60), gray)
	if f.a.prevX != 150 || f.a.prevY != 80 {
		t.Errorf("prev = %v,%v", f.a.prevX, f.a.prevY)
	}
}

func TestFrameRebuildsFromTargetEachDraw(t *testing.T) {
	f := newFixture(t)
	f.in.PenAxis(1, sdl.PEN_AXIS_PRESSURE, 1, 0, 0)
	f.in.PenMotion(1, 10, 50, 0)
	f.in.PenMotion(1, 150, 50, 0)
	f.feed()
	f.frame()
	f.frame() // strokes persist across frames
	sdltest.AssertRegion(t, f.c.Capture(t), imgRect(20, 50, 140, 51), black)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}

func imgRect(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }

func anyNot(img *image.RGBA, r image.Rectangle, c color.RGBA) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) != c {
				return true
			}
		}
	}
	return false
}
