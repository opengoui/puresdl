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

var black = color.RGBA{0, 0, 0, 255}

// newTestScene returns a scene with its texture loaded on a 640x480 canvas.
func newTestScene(t *testing.T) (*scene, *sdltest.Clock, *sdltest.Canvas) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	g := newScene(clock.Ticks)
	var err error
	sdltest.OnMain(t, func() { err = g.loadTexture(c.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, g.close) })
	return g, &clock, c
}

// frameAt draws one frame at the given time (ms) and captures it.
func frameAt(t *testing.T, g *scene, clock *sdltest.Clock, c *sdltest.Canvas, ms uint64) *image.RGBA {
	t.Helper()
	clock.Advance(ms - clock.Ticks())
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	return c.Capture(t)
}

func near(a, b uint8, tol int) bool {
	d := int(a) - int(b)
	return d >= -tol && d <= tol
}

func TestTriangleSizeFollowsTheClock(t *testing.T) {
	// Values derived from the C formula: size = 200 + 200*scale.
	for _, tc := range []struct {
		now  uint64
		want float32
	}{
		{0, 400}, {500, 200}, {1000, 0}, {1500, 200}, {2000, 400}, {2500, 200},
	} {
		if got := triangleSize(tc.now); got != tc.want {
			t.Errorf("triangleSize(%d) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestQuitEventOnly(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	g := newScene(new(sdltest.Clock).Ticks)
	in := sdltest.NewInput(t, 0)

	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(100, 100)
	in.Quit()
	quits := map[sdl.EventType]bool{}
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			quits[sdl.EventType(e.Type())] = g.handleEvent(&e) || quits[sdl.EventType(e.Type())]
		}
	})
	if !quits[sdl.EVENT_QUIT] {
		t.Error("EVENT_QUIT did not request quit")
	}
	for typ, q := range quits {
		if q && typ != sdl.EVENT_QUIT {
			t.Errorf("event type %#x requested quit", uint32(typ))
		}
	}
	if len(quits) < 2 {
		t.Errorf("expected other event types besides QUIT, got %v", quits)
	}
}

func TestInitialFrameGeometry(t *testing.T) {
	g, clock, c := newTestScene(t)
	img := frameAt(t, g, clock, c, 500) // triangle size 200: (320,140) (420,340) (220,340)

	// Outside every shape the clear colour shows.
	sdltest.AssertPixel(t, img, 600, 400, black)
	sdltest.AssertPixel(t, img, 5, 300, black)
	sdltest.AssertPixel(t, img, 320, 100, black)

	// Colour triangle: vertices are pure red / green / blue, blended between.
	if p := sdltest.PixelAt(img, 320, 146); !(p.R > 230 && p.G < 20 && p.B < 20) {
		t.Errorf("near red vertex = %v", p)
	}
	if p := sdltest.PixelAt(img, 410, 334); !(p.G > 220 && p.R < 25 && p.B < 25) {
		t.Errorf("near green vertex = %v", p)
	}
	if p := sdltest.PixelAt(img, 230, 334); !(p.B > 220 && p.R < 25 && p.G < 25) {
		t.Errorf("near blue vertex = %v", p)
	}
	// Centroid (320, 273.3) is an even mix of the three colours.
	p := sdltest.PixelAt(img, 320, 273)
	for _, ch := range []uint8{p.R, p.G, p.B} {
		if !near(ch, 85, 6) {
			t.Errorf("centroid = %v, want about (85,85,85)", p)
			break
		}
	}
}

func TestTriangleGrowsAndShrinks(t *testing.T) {
	g, clock, c := newTestScene(t)
	// At t=0 the triangle has size 400: apex y=40, base y=440.
	big := frameAt(t, g, clock, c, 0)
	if p := sdltest.PixelAt(big, 320, 46); p.R < 200 {
		t.Errorf("t=0 apex pixel = %v, want red", p)
	}
	if p := sdltest.PixelAt(big, 320, 436); p == black {
		t.Error("t=0 base pixel is black, triangle too small")
	}
	// At t=1000 the size is 0: the triangle is gone, only the clear colour
	// remains at its former centre.
	gone := frameAt(t, g, clock, c, 1000)
	sdltest.AssertPixel(t, gone, 320, 240, black)
	sdltest.AssertPixel(t, gone, 320, 46, black)
	// At t=1500 it is back at size 200 (grows again, symmetric with t=500).
	again := frameAt(t, g, clock, c, 1500)
	if p := sdltest.PixelAt(again, 320, 146); p.R < 230 {
		t.Errorf("t=1500 apex = %v, want red", p)
	}
}

func TestTexturedTriangleAndIndexedQuad(t *testing.T) {
	g, clock, c := newTestScene(t)
	img := frameAt(t, g, clock, c, 1000) // colour triangle has size 0: textures only

	// The indexed quad's first triangle has the same geometry and UVs as the
	// plain textured triangle shifted by 450 px, so the pixels must match.
	for _, pt := range [][2]int{{12, 12}, {70, 20}, {20, 70}, {60, 60}, {120, 12}, {12, 120}} {
		a := sdltest.PixelAt(img, pt[0], pt[1])
		b := sdltest.PixelAt(img, pt[0]+450, pt[1])
		if a != b {
			t.Errorf("textured triangle %v = %v, indexed quad = %v", pt, a, b)
		}
	}

	// The second index triangle (1,2,3) covers the lower right half of the
	// quad; the plain textured triangle has no such half. Count lit pixels
	// strictly past the diagonal (dx+dy > 142) in both places.
	lit := func(ox int) (n int) {
		for dy := 0; dy < 140; dy++ {
			for dx := 0; dx < 140; dx++ {
				if dx+dy > 142 && sdltest.PixelAt(img, ox+dx, 10+dy) != black {
					n++
				}
			}
		}
		return n
	}
	if n := lit(460); n < 100 {
		t.Errorf("indexed quad lower half has only %d lit pixels, want the second triangle drawn", n)
	}
	if n := lit(10); n != 0 {
		t.Errorf("plain textured triangle drew %d pixels past its hypotenuse", n)
	}

	// The untextured left half has nothing in the lower right corner.
	sdltest.AssertPixel(t, img, 140, 140, black)
	// Outside the quad's bounds stays clear.
	sdltest.AssertPixel(t, img, 455, 80, black)
	sdltest.AssertPixel(t, img, 605, 80, black)
	sdltest.AssertPixel(t, img, 530, 155, black)
}

func TestFrameGolden(t *testing.T) {
	g, clock, c := newTestScene(t)
	sdltest.AssertImageTol(t, "frame-500ms", frameAt(t, g, clock, c, 500), sdltest.Tolerance{Channel: 3, Pixels: 200})
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	g, clock, c := newTestScene(t)
	frameAt(t, g, clock, c, 0)
	img := frameAt(t, g, clock, c, 1000)
	// Pixels covered by the t=0 triangle but not by anything at t=1000.
	sdltest.AssertPixel(t, img, 320, 400, black)
}

func TestCloseReleasesTexture(t *testing.T) {
	g, _, _ := newTestScene(t)
	if g.texture == nil {
		t.Fatal("texture not loaded")
	}
	sdltest.OnMain(t, g.close)
	if g.texture != nil {
		t.Error("texture still held after close")
	}
	sdltest.OnMain(t, g.close) // idempotent
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
