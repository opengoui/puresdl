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

var black = color.RGBA{0, 0, 0, 255}

// newTestApp creates a texture on the given renderer with a manual clock.
func newTestApp(t *testing.T, r *sdl.Renderer) (*app, *sdltest.Clock) {
	t.Helper()
	var clock sdltest.Clock
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp(r, clock.Ticks) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a, &clock
}

// bbox returns the bounding box of all non-black pixels.
func bbox(img *image.RGBA) image.Rectangle {
	var r image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) != black {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

// visibleBounds returns the bounds of pixels that are not transparent black.
func visibleBounds(img image.Image) image.Rectangle {
	var r image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			if ca != 0 && cr|cg|cb != 0 {
				r = r.Union(image.Rect(x-b.Min.X, y-b.Min.Y, x-b.Min.X+1, y-b.Min.Y+1))
			}
		}
	}
	return r
}

func within(a, b, tol int) bool { return a-b <= tol && b-a <= tol }

func near(a, b int) bool { return a-b <= 1 && b-a <= 1 }

func TestDstRectFollowsTheScaleCycle(t *testing.T) {
	const w, h = 408, 167
	for _, tc := range []struct {
		now   uint64
		scale float64
	}{
		{0, 1}, {250, 0.5}, {500, 0}, {750, -0.5}, {1000, -1},
		{1250, -0.5}, {1500, 0}, {1750, 0.5}, {2000, 1}, {2500, 0},
	} {
		r := dstRect(tc.now, w, h)
		ww, hh := w*(1+tc.scale), h*(1+tc.scale)
		if math.Abs(float64(r.W)-ww) > 1e-3 || math.Abs(float64(r.H)-hh) > 1e-3 {
			t.Errorf("now=%d size = %vx%v, want %vx%v", tc.now, r.W, r.H, ww, hh)
		}
		if math.Abs(float64(r.X)-(windowWidth-ww)/2) > 1e-3 || math.Abs(float64(r.Y)-(windowHeight-hh)/2) > 1e-3 {
			t.Errorf("now=%d not centered: %+v", tc.now, r)
		}
	}
}

func TestTextureSizeMatchesThePNG(t *testing.T) {
	cfg, err := png.DecodeConfig(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a, _ := newTestApp(t, c.Renderer)
	if int(a.texW) != cfg.Width || int(a.texH) != cfg.Height {
		t.Errorf("texture %vx%v, PNG %dx%d", a.texW, a.texH, cfg.Width, cfg.Height)
	}
	var w, h float32
	sdltest.OnMain(t, func() { sdl.GetTextureSize(a.texture, &w, &h) })
	if int(w) != cfg.Width || int(h) != cfg.Height {
		t.Errorf("SDL texture size %vx%v, PNG %dx%d", w, h, cfg.Width, cfg.Height)
	}
}

func TestDrawnFrameBoundsFollowTheClock(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a, clock := newTestApp(t, c.Renderer)
	src, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	content := visibleBounds(src)
	if content.Empty() {
		t.Fatal("sample.png has no visible pixels")
	}
	for _, now := range []uint64{0, 250, 500, 750, 1250, 1750} {
		clock.Advance(now - clock.Ticks())
		sdltest.OnMain(t, func() { a.draw(c.Renderer) })
		img := c.Capture(t)
		want := dstRect(now, a.texW, a.texH)
		got := bbox(img)
		if got.Empty() {
			t.Fatalf("now=%d: nothing drawn", now)
		}
		// Expected bounds: the PNG's visible content mapped into dst.
		sx, sy := float64(want.W)/float64(a.texW), float64(want.H)/float64(a.texH)
		exp := image.Rect(
			int(math.Round(float64(want.X)+float64(content.Min.X)*sx)),
			int(math.Round(float64(want.Y)+float64(content.Min.Y)*sy)),
			int(math.Round(float64(want.X)+float64(content.Max.X)*sx)),
			int(math.Round(float64(want.Y)+float64(content.Max.Y)*sy)))
		exp = exp.Intersect(image.Rect(0, 0, windowWidth, windowHeight)) // 2x overflows the canvas
		if !within(got.Min.X, exp.Min.X, 3) || !within(got.Min.Y, exp.Min.Y, 3) ||
			!within(got.Max.X, exp.Max.X, 3) || !within(got.Max.Y, exp.Max.Y, 3) {
			t.Errorf("now=%d drawn bounds %v, want about %v", now, got, exp)
		}
	}
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a, clock := newTestApp(t, c.Renderer)
	sdltest.OnMain(t, func() { a.draw(c.Renderer) }) // 2x
	clock.Advance(1000)                              // size 0
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	img := c.Capture(t)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, windowWidth, windowHeight), black)
}

func TestPixelsComeFromTheSamplePNG(t *testing.T) {
	src, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a, clock := newTestApp(t, c.Renderer)
	clock.Advance(500) // native size
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	img := c.Capture(t)
	b := src.Bounds()
	ox, oy := (windowWidth-b.Dx())/2, (windowHeight-b.Dy())/2
	// Compare a coarse grid, allowing a one-row shift from the half-pixel offset.
	bad, total := 0, 0
	for y := 4; y < b.Dy()-4; y += 9 {
		for x := 4; x < b.Dx()-4; x += 9 {
			r, g, bl, al := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if al != 0xffff {
				continue
			}
			total++
			want := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
			ok := false
			for dy := -1; dy <= 1; dy++ {
				if img.RGBAAt(ox+x, oy+y+dy) == want {
					ok = true
				}
			}
			if !ok {
				bad++
			}
		}
	}
	if total == 0 {
		t.Fatal("sample has no opaque pixels")
	}
	if bad*10 > total {
		t.Errorf("%d of %d sampled pixels differ from the PNG", bad, total)
	}
}

func TestLetterboxKeepsAspectOnAWiderWindow(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, windowWidth*2, windowHeight)
	var r *sdl.Renderer
	sdltest.OnMain(t, func() {
		r = sdl.CreateRenderer(win, "software")
		sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
	})
	if r == nil {
		t.Fatalf("CreateRenderer: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	a, clock := newTestApp(t, r)
	clock.Advance(500)
	sdltest.OnMain(t, func() { a.draw(r) })

	// Read-back is expressed in logical coordinates, so the bars are checked
	// through the presentation rectangle instead of captured pixels.
	var rect sdl.FRect
	var ok bool
	sdltest.OnMain(t, func() { ok = sdl.GetRenderLogicalPresentationRect(r, &rect) })
	if !ok {
		t.Fatalf("GetRenderLogicalPresentationRect: %s", sdl.GetError())
	}
	want := sdl.FRect{X: windowWidth / 2, Y: 0, W: windowWidth, H: windowHeight}
	if rect != want {
		t.Errorf("presentation rect = %+v, want %+v (aspect kept, centered, 320px bars)", rect, want)
	}

	img := sdltest.Capture(t, r)
	if img.Bounds().Dx() != windowWidth || img.Bounds().Dy() != windowHeight {
		t.Fatalf("logical capture = %v, want %dx%d", img.Bounds(), windowWidth, windowHeight)
	}
	if bbox(img).Empty() {
		t.Error("nothing was drawn inside the letterboxed area")
	}
}

func TestOnlyQuitEndsTheProgram(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a, _ := newTestApp(t, c.Renderer)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(100, 100)
	in.CloseRequested()
	var quit bool
	for _, e := range sdltest.Drain(t) {
		quit = a.handleEvent(&e) || quit
	}
	if quit {
		t.Fatal("non-quit events ended the program")
	}
	in.Quit()
	for _, e := range sdltest.Drain(t) {
		quit = a.handleEvent(&e) || quit
	}
	if !quit {
		t.Error("EVENT_QUIT did not end the program")
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
