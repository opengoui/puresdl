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
)

func setup(t *testing.T) (*cloud, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	sdltest.OnMain(t, func() { sdl.Srand(12345) })
	var clock sdltest.Clock
	return newCloud(clock.Ticks), &clock
}

func TestInitialPointsAreInsideWindowWithValidSpeeds(t *testing.T) {
	c, _ := setup(t)
	for i, p := range c.points {
		if p.X < 0 || p.X >= windowWidth || p.Y < 0 || p.Y >= windowHeight {
			t.Fatalf("point %d = %+v outside window", i, p)
		}
		if s := c.pointSpeeds[i]; s < minPixelsPerSec || s >= maxPixelsPerSec {
			t.Fatalf("speed %d = %v outside [%d,%d)", i, s, minPixelsPerSec, maxPixelsPerSec)
		}
	}
}

func TestAdvanceMovesDiagonallyBySpeedTimesElapsed(t *testing.T) {
	c, clock := setup(t)
	for i := range c.points {
		c.points[i] = sdl.FPoint{X: 100, Y: 50}
		c.pointSpeeds[i] = 40
	}
	clock.Advance(500)
	c.advance()
	for i, p := range c.points {
		if p.X != 120 || p.Y != 70 {
			t.Fatalf("point %d = %+v, want (120,70)", i, p)
		}
	}
	// No time elapsed: nothing moves.
	c.advance()
	if p := c.points[0]; p.X != 120 || p.Y != 70 {
		t.Fatalf("zero-elapsed advance moved point to %+v", p)
	}
	// Elapsed is measured from the previous advance, not from start.
	clock.Advance(250)
	c.advance()
	if p := c.points[0]; p.X != 130 || p.Y != 80 {
		t.Fatalf("point = %+v, want (130,80)", p)
	}
}

func TestPointLeavingRightOrBottomRespawnsOnAnEdge(t *testing.T) {
	for _, start := range []sdl.FPoint{{X: windowWidth - 1, Y: 10}, {X: 10, Y: windowHeight - 1}, {X: windowWidth - 1, Y: windowHeight - 1}} {
		c, clock := setup(t)
		for i := range c.points {
			c.points[i] = start
			c.pointSpeeds[i] = 30
		}
		clock.Advance(1000) // moves 30px: off screen
		c.advance()
		top, left := 0, 0
		for i, p := range c.points {
			switch {
			case p.Y == 0 && p.X >= 0 && p.X < windowWidth:
				top++
			case p.X == 0 && p.Y >= 0 && p.Y < windowHeight:
				left++
			default:
				t.Fatalf("start %+v: point %d respawned at %+v, not on top/left edge", start, i, p)
			}
			if s := c.pointSpeeds[i]; s < minPixelsPerSec || s >= maxPixelsPerSec {
				t.Fatalf("respawn speed %v out of range", s)
			}
		}
		if top == 0 || left == 0 {
			t.Errorf("start %+v: respawn edges top=%d left=%d, want both used", start, top, left)
		}
	}
}

func TestExactlyAtBoundaryCountsAsOffScreen(t *testing.T) {
	c, clock := setup(t)
	c.points[0] = sdl.FPoint{X: windowWidth - 30, Y: 0}
	c.pointSpeeds[0] = 30
	clock.Advance(1000) // lands exactly on X == windowWidth
	c.advance()
	if p := c.points[0]; p.X == windowWidth || (p.X != 0 && p.Y != 0) {
		t.Fatalf("point at boundary not respawned: %+v", p)
	}
}

func TestPointsStayInsideWindowOverLongRun(t *testing.T) {
	c, clock := setup(t)
	for step := 0; step < 2000; step++ {
		clock.Advance(100)
		c.advance()
	}
	for i, p := range c.points {
		if p.X < 0 || p.X >= windowWidth || p.Y < 0 || p.Y >= windowHeight {
			t.Fatalf("point %d = %+v escaped the window", i, p)
		}
	}
}

func whiteCount(img *image.RGBA) (n int) {
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if sdltest.PixelAt(img, x, y) == white {
				n++
			}
		}
	}
	return n
}

func TestDrawPlotsWhitePointsOnBlack(t *testing.T) {
	c, _ := setup(t)
	canvas := sdltest.NewCanvas(t, windowWidth, windowHeight)
	for i := range c.points { // park everything on one pixel, then place a few
		c.points[i] = sdl.FPoint{X: 0, Y: 0}
	}
	c.points[1] = sdl.FPoint{X: 100, Y: 50}
	c.points[2] = sdl.FPoint{X: 639, Y: 479}
	// Stale content must be cleared by draw.
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(canvas.Renderer, 9, 9, 9, 255)
		sdl.RenderClear(canvas.Renderer)
		c.draw(canvas.Renderer)
	})
	img := canvas.Capture(t)
	for _, p := range [][2]int{{0, 0}, {100, 50}, {639, 479}} {
		sdltest.AssertPixel(t, img, p[0], p[1], white)
	}
	if n := whiteCount(img); n != 3 {
		t.Errorf("white pixels = %d, want 3", n)
	}
	sdltest.AssertPixel(t, img, 101, 50, black)
	sdltest.AssertPixel(t, img, 320, 240, black)
}

func TestDrawRandomCloudHasAtMostOnePixelPerPoint(t *testing.T) {
	c, _ := setup(t)
	canvas := sdltest.NewCanvas(t, windowWidth, windowHeight)
	sdltest.OnMain(t, func() { c.draw(canvas.Renderer) })
	n := whiteCount(canvas.Capture(t))
	if n == 0 || n > numPoints {
		t.Errorf("white pixels = %d, want 1..%d", n, numPoints)
	}
	if n < numPoints*9/10 {
		t.Errorf("white pixels = %d, suspiciously few for %d random points", n, numPoints)
	}
}

func TestLetterboxMapsLogicalCoordinatesOnResizedWindow(t *testing.T) {
	c, _ := setup(t)
	win := sdltest.NewWindow(t, 800, windowHeight) // wider than the logical 640x480
	var r *sdl.Renderer
	var dst sdl.FRect
	var ok bool
	sdltest.OnMain(t, func() {
		r = sdl.CreateRenderer(win, "software")
		if r == nil {
			return
		}
		ok = sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, logicalPresLetter)
		sdl.GetRenderLogicalPresentationRect(r, &dst)
	})
	if r == nil || !ok {
		t.Fatalf("renderer setup failed: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	if want := (sdl.FRect{X: 80, Y: 0, W: windowWidth, H: windowHeight}); dst != want {
		t.Errorf("letterbox rect = %+v, want %+v", dst, want)
	}
	for i := range c.points {
		c.points[i] = sdl.FPoint{}
	}
	c.points[1] = sdl.FPoint{X: 639, Y: 479}
	sdltest.OnMain(t, func() { c.draw(r) })
	// Reading pixels in logical mode returns the logical content area.
	img := sdltest.Capture(t, r)
	if b := img.Bounds(); b.Dx() != windowWidth || b.Dy() != windowHeight {
		t.Fatalf("capture size = %v, want %dx%d", b, windowWidth, windowHeight)
	}
	sdltest.AssertPixel(t, img, 0, 0, white)
	sdltest.AssertPixel(t, img, 639, 479, white)
	if n := whiteCount(img); n != 2 {
		t.Errorf("white pixels = %d, want 2", n)
	}
}

func TestQuitEventOnly(t *testing.T) {
	c, _ := setup(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(300, 200)
	in.CloseRequested()
	in.Quit()
	var quits []bool
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			quits = append(quits, c.handleEvent(&e))
		}
	})
	n := 0
	for _, q := range quits {
		if q {
			n++
		}
	}
	if n != 1 || len(quits) < 5 {
		t.Errorf("quit decisions %v: want exactly one quit among >=5 events", quits)
	}
}

func TestQuitEventTypeRoundTrip(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	sdltest.NewInput(t, 0).Quit()
	got := sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_QUIT)
	if len(got) != 1 {
		t.Fatalf("got %d quit events, want 1", len(got))
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
