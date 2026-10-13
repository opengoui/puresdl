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
	bg     = color.RGBA{33, 33, 33, 255}
	blue   = color.RGBA{0, 0, 255, 255}
	red    = color.RGBA{255, 0, 0, 255}
	green  = color.RGBA{0, 255, 0, 255}
	yellow = color.RGBA{255, 255, 0, 255}
)

// newSeeded builds a scene from a fixed random seed.
func newSeeded(t *testing.T, seed uint64) *scene {
	t.Helper()
	return sdltest.Call(t, func() *scene {
		sdl.Srand(seed)
		return newScene()
	})
}

// feed pumps SDL and applies every queued event like the run loop does.
func feed(t *testing.T, s *scene) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if s.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func drawFrame(t *testing.T, s *scene, w, h int32) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, w, h)
	sdltest.OnMain(t, func() { s.draw(c.Renderer) })
	return c.Capture(t)
}

func isPoint(s *scene, x, y int) bool {
	for _, p := range s.points {
		if int(p.X) == x && int(p.Y) == y {
			return true
		}
	}
	return false
}

func TestPointsAreScatteredInsideTheRectangle(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 1)
	distinct := map[sdl.FPoint]bool{}
	for i, p := range s.points {
		if p.X < 100 || p.X >= 540 || p.Y < 100 || p.Y >= 380 {
			t.Fatalf("point %d = %v outside [100,540)x[100,380)", i, p)
		}
		distinct[p] = true
	}
	if len(distinct) < 490 {
		t.Errorf("only %d distinct points of %d: not random", len(distinct), len(s.points))
	}
	if other := newSeeded(t, 2); other.points == s.points {
		t.Error("different seeds produced identical points")
	}
	if again := newSeeded(t, 1); again.points != s.points {
		t.Error("same seed produced different points")
	}
}

func TestFrameLayersAreDrawnInOrder(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 1)
	img := drawFrame(t, s, 640, 480)

	// Cleared background outside the filled rectangle (clear of the diagonals).
	sdltest.AssertRegion(t, img, image.Rect(5, 150, 95, 350), bg)
	sdltest.AssertRegion(t, img, image.Rect(545, 150, 635, 350), bg)

	// Blue fill, sampled away from points, lines and the green outline.
	blueSamples := 0
	for _, p := range []image.Point{{110, 200}, {115, 250}, {200, 200}, {450, 200}, {500, 300}, {350, 330}} {
		if isPoint(s, p.X, p.Y) {
			continue
		}
		sdltest.AssertPixel(t, img, p.X, p.Y, blue)
		blueSamples++
	}
	if blueSamples < 3 {
		t.Fatalf("only %d blue samples survived", blueSamples)
	}

	// The unfilled rectangle is inset by 30 and drawn after the points.
	sdltest.AssertPixel(t, img, 300, 130, green)
	sdltest.AssertPixel(t, img, 300, 349, green)
	sdltest.AssertPixel(t, img, 130, 200, green)
	sdltest.AssertPixel(t, img, 509, 200, green)

	// Both diagonals are yellow and drawn last (over fill, points, outline).
	for _, p := range []image.Point{{0, 0}, {200, 150}, {320, 240}, {400, 300}, {200, 330}, {400, 180}} {
		sdltest.AssertPixel(t, img, p.X, p.Y, yellow)
	}
}

func TestPointsAreRedOnTopOfTheFill(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 7)
	img := drawFrame(t, s, 640, 480)
	reds := 0
	for i, p := range s.points {
		got := sdltest.PixelAt(img, int(p.X), int(p.Y))
		switch got {
		case red:
			reds++
		case green, yellow: // later layers may legitimately cover a point
		default:
			t.Fatalf("point %d at (%v,%v) is %v, want red (or covered by outline/line)", i, p.X, p.Y, got)
		}
	}
	if reds < 400 {
		t.Errorf("only %d of %d points visible as red", reds, len(s.points))
	}
}

func TestDrawIsIdempotentAndOverwritesPreviousFrame(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 3)
	c := sdltest.NewCanvas(t, 640, 480)
	sdltest.OnMain(t, func() { s.draw(c.Renderer) })
	first := c.Capture(t)
	// Dirty the target, then draw again: the clear must erase everything.
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 9, 9, 9, 255)
		sdl.RenderClear(c.Renderer)
		s.draw(c.Renderer)
	})
	second := c.Capture(t)
	if string(first.Pix) != string(second.Pix) {
		t.Error("second frame differs from the first for an unchanged scene")
	}
}

func TestDifferentSeedsChangeOnlyPoints(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := drawFrame(t, newSeeded(t, 10), 640, 480)
	b := drawFrame(t, newSeeded(t, 11), 640, 480)
	if string(a.Pix) == string(b.Pix) {
		t.Fatal("frames identical despite different points")
	}
	sdltest.AssertRegion(t, b, image.Rect(5, 150, 95, 350), bg)
	sdltest.AssertPixel(t, b, 200, 150, yellow)
}

func TestQuitEventEndsProgramOthersDoNot(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 1)
	in := sdltest.NewInput(t, 0)

	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(10, 10, 1, 1, 0)
	in.MouseButton(1, true, 10, 10)
	in.MouseWheel(0, 1)
	in.CloseRequested() // the C program only reacts to EVENT_QUIT
	in.Resized(800, 600)
	if feed(t, s) {
		t.Fatal("non-quit events ended the program")
	}
	in.Quit()
	if !feed(t, s) {
		t.Fatal("EVENT_QUIT did not end the program")
	}
}

func TestLetterboxPresentationKeepsAspect(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := newSeeded(t, 1)
	// A target wider than 4:3: logical 640x480 is centred with 80px bars.
	c := sdltest.NewCanvas(t, 800, 480)
	var rect sdl.FRect
	var wx, wy float32
	sdltest.OnMain(t, func() {
		sdl.SetRenderLogicalPresentation(c.Renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		sdl.GetRenderLogicalPresentationRect(c.Renderer, &rect)
		sdl.RenderCoordinatesToWindow(c.Renderer, 0, 0, &wx, &wy)
		s.draw(c.Renderer)
	})
	if want := (sdl.FRect{X: 80, Y: 0, W: 640, H: 480}); rect != want {
		t.Errorf("presentation rect = %v, want %v", rect, want)
	}
	if wx != 80 || wy != 0 {
		t.Errorf("logical origin maps to window (%v,%v), want (80,0)", wx, wy)
	}
	// Reads are in logical space, so the frame is the same as without bars.
	img := c.Capture(t)
	if got, want := img.Bounds(), image.Rect(0, 0, 640, 480); got != want {
		t.Fatalf("capture bounds = %v, want %v", got, want)
	}
	sdltest.AssertPixel(t, img, 5, 240, bg)
	sdltest.AssertPixel(t, img, 320, 240, yellow)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
