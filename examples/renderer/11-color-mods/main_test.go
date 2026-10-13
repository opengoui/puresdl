package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// feed applies every queued event to g like the run loop does.
func feed(t *testing.T, g *game) (quit bool) {
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

// frame builds a game on a canvas at the given time and draws one frame.
func frame(t *testing.T, ms uint64) (*image.RGBA, *game) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	clock.Advance(ms)
	var g *game
	var err error
	sdltest.OnMain(t, func() { g, err = newGame(clock.Ticks, c.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, g.close) })
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	return c.Capture(t), g
}

func source(t *testing.T) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// expect is the source pixel at (x,y) modulated by mod and blended over black.
func expect(src image.Image, x, y int, mod [3]float32) color.RGBA {
	n := color.NRGBAModel.Convert(src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y)).(color.NRGBA)
	f := func(c uint8, m float32) uint8 {
		return uint8(math.Round(float64(c) * float64(m) * float64(n.A) / 255))
	}
	return color.RGBA{f(n.R, mod[0]), f(n.G, mod[1]), f(n.B, mod[2]), 255}
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return math.Abs(float64(x)-float64(y)) <= 2 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// checkTexture compares a grid of texels of the texture drawn at (ox,oy).
func checkTexture(t *testing.T, img *image.RGBA, src image.Image, ox, oy int, mod [3]float32, name string) {
	t.Helper()
	b := src.Bounds()
	for y := 0; y < b.Dy(); y += 13 {
		for x := 0; x < b.Dx(); x += 17 {
			want := expect(src, x, y, mod)
			if got := sdltest.PixelAt(img, ox+x, oy+y); !near(got, want) {
				t.Fatalf("%s texel (%d,%d): got %v want %v", name, x, y, got, want)
			}
		}
	}
}

func TestInitialFrameThreeModulatedCopies(t *testing.T) {
	img, g := frame(t, 0)
	src := source(t)
	w, h := int(g.w), int(g.h)
	if w != src.Bounds().Dx() || h != src.Bounds().Dy() {
		t.Fatalf("texture size %dx%d, source %v", w, h, src.Bounds())
	}
	r, gr, b := centerMod(0)
	checkTexture(t, img, src, 0, 0, [3]float32{0, 0, 1}, "top-left blue")
	checkTexture(t, img, src, (windowWidth-w)/2, (windowHeight-h)/2, [3]float32{r, gr, b}, "center")
	checkTexture(t, img, src, windowWidth-w, windowHeight-h, [3]float32{1, 0, 0}, "bottom-right red")
	// Corners not covered by any copy keep the black clear color.
	sdltest.AssertPixel(t, img, windowWidth-1, 0, color.RGBA{0, 0, 0, 255})
	sdltest.AssertPixel(t, img, 0, windowHeight-1, color.RGBA{0, 0, 0, 255})
}

func TestCenterModulationFollowsTime(t *testing.T) {
	// At t=1.5s the modulation differs from t=0; the outer copies must not change.
	a, g := frame(t, 0)
	b, _ := frame(t, 1500)
	w, h := int(g.w), int(g.h)
	src := source(t)
	r, gr, bl := centerMod(1.5)
	checkTexture(t, b, src, (windowWidth-w)/2, (windowHeight-h)/2, [3]float32{r, gr, bl}, "center@1.5s")
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("frames at 0s and 1.5s are identical")
	}
	checkTexture(t, b, src, 0, 0, [3]float32{0, 0, 1}, "top-left@1.5s")
}

func TestCenterModRange(t *testing.T) {
	// Contract from the C code: every channel stays within [0,1] and the three
	// channels are 120 degrees apart (they sum to 1.5).
	for ms := uint64(0); ms < 20000; ms += 137 {
		r, g, b := centerMod(float64(ms) / 1000)
		for _, v := range []float32{r, g, b} {
			if v < 0 || v > 1 {
				t.Fatalf("t=%d: out of range %v", ms, v)
			}
		}
		if s := r + g + b; math.Abs(float64(s)-1.5) > 1e-5 {
			t.Fatalf("t=%d: channel sum %v, want 1.5", ms, s)
		}
	}
	if r, g, b := centerMod(0); math.Abs(float64(r)-0.5) > 1e-6 || g < 0.93 || b > 0.07 {
		t.Fatalf("centerMod(0) = %v %v %v", r, g, b)
	}
}

func TestQuitEventEndsProgramOthersDoNot(t *testing.T) {
	_, g := frame(t, 0)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(10, 10, 1, 1, 0)
	in.MouseButton(1, true, 10, 10)
	in.MouseWheel(0, 1)
	in.Resized(800, 600)
	in.CloseRequested()
	if feed(t, g) {
		t.Fatal("non-quit events must not end the program")
	}
	in.Quit()
	if !feed(t, g) {
		t.Fatal("EVENT_QUIT must end the program")
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
