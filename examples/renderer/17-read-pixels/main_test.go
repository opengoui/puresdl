package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	red   = color.RGBA{R: 255, A: 255}
	white = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black = color.RGBA{A: 255}
)

// newTestApp builds an app on a 640x480 offscreen canvas driven by a fake clock.
func newTestApp(t *testing.T) (*app, *sdltest.Canvas, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	clock := &sdltest.Clock{}
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp(c.Renderer, clock.Ticks) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a, c, clock
}

func iterate(t *testing.T, a *app) {
	t.Helper()
	var err error
	sdltest.OnMain(t, func() { err = a.iterate() })
	if err != nil {
		t.Fatal(err)
	}
}

func scene(t *testing.T, a *app, c *sdltest.Canvas) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, a.drawScene)
	return c.Capture(t)
}

func samplePixels(t *testing.T) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestRotationCycleIsTwoSeconds(t *testing.T) {
	for _, tc := range []struct {
		now  uint64
		want float64
	}{{0, 0}, {500, 90}, {1000, 180}, {1500, 270}, {2000, 0}, {2500, 90}, {123456000, 0}} {
		if got := rotation(tc.now); got != tc.want {
			t.Errorf("rotation(%d) = %v, want %v", tc.now, got, tc.want)
		}
	}
	if got := rotation(1999); got <= 359 || got >= 360 {
		t.Errorf("rotation(1999) = %v, want just below 360", got)
	}
}

func TestOnlyQuitEventQuits(t *testing.T) {
	a, _, _ := newTestApp(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(5, 5, 1, 1, 0)
	in.MouseButton(1, true, 5, 5)
	in.Resized(100, 100)
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
		t.Fatal("non-quit events must not end the program")
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
		t.Fatal("EVENT_QUIT did not quit")
	}
}

func TestTextureMatchesEmbeddedPNG(t *testing.T) {
	a, _, _ := newTestApp(t)
	b := samplePixels(t).Bounds()
	if a.textureWidth != int32(b.Dx()) || a.textureHeight != int32(b.Dy()) {
		t.Fatalf("texture size %dx%d, want %dx%d", a.textureWidth, a.textureHeight, b.Dx(), b.Dy())
	}
}

func TestUnrotatedSceneIsCenteredSprite(t *testing.T) {
	a, c, _ := newTestApp(t)
	img := scene(t, a, c)
	src := samplePixels(t)
	b := src.Bounds()
	ox, oy := (windowWidth-b.Dx())/2, (windowHeight-b.Dy())/2

	// Outside the sprite everything is the black clear colour.
	sdltest.AssertRegion(t, img, image.Rect(0, 0, windowWidth, oy), black)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, ox, windowHeight), black)

	// Inside, opaque sprite pixels land unscaled at the centered offset.
	checked := 0
	for y := 0; y < b.Dy(); y += 7 {
		for x := 0; x < b.Dx(); x += 7 {
			n := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if n.A != 255 {
				continue
			}
			want := color.RGBA{R: n.R, G: n.G, B: n.B, A: 255}
			if got := sdltest.PixelAt(img, ox+x, oy+y); got != want {
				t.Fatalf("sprite (%d,%d) at screen (%d,%d) = %v, want %v", x, y, ox+x, oy+y, got, want)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Fatalf("only %d sprite pixels checked", checked)
	}
}

func TestHalfTurnMirrorsTheSprite(t *testing.T) {
	a, c, clock := newTestApp(t)
	first := scene(t, a, c)
	clock.Advance(1000)
	half := scene(t, a, c)
	// The sprite's centre sits on a half pixel row, so allow the mirror axis
	// to be off by one pixel; rasterization may differ along edges only.
	best := windowWidth * windowHeight
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			diff := 0
			for y := 0; y < windowHeight; y++ {
				for x := 0; x < windowWidth; x++ {
					mx, my := windowWidth-1-x+dx, windowHeight-1-y+dy
					if mx < 0 || my < 0 || mx >= windowWidth || my >= windowHeight {
						continue
					}
					if first.RGBAAt(x, y) != half.RGBAAt(mx, my) {
						diff++
					}
				}
			}
			best = min(best, diff)
		}
	}
	if limit := windowWidth * windowHeight / 100; best > limit {
		t.Fatalf("%d pixels differ from the point-mirrored frame, limit %d", best, limit)
	}
	quarter := func() *image.RGBA { clock.Advance(1500); return scene(t, a, c) }()
	if bytes.Equal(first.Pix, quarter.Pix) {
		t.Fatal("a quarter turn must change the frame")
	}
}

func TestRotationIsPeriodic(t *testing.T) {
	a, c, clock := newTestApp(t)
	clock.Advance(300)
	f1 := scene(t, a, c)
	clock.Advance(2000)
	f2 := scene(t, a, c)
	if !bytes.Equal(f1.Pix, f2.Pix) {
		t.Fatal("frames 2000ms apart must be identical")
	}
}

func classify(c color.RGBA) color.RGBA {
	avg := (int(c.R) + int(c.G) + int(c.B)) / 3
	switch {
	case avg == 0:
		return red
	case avg > 50:
		return white
	default:
		return black
	}
}

// TestCornerIsThresholdedQuarterSizeCopy checks the read-back, threshold and
// redraw pipeline against an independent classification of the scene drawn
// before the overlay.
func TestCornerIsThresholdedQuarterSizeCopy(t *testing.T) {
	a, c, clock := newTestApp(t)
	clock.Advance(250)
	base := scene(t, a, c)
	iterate(t, a)
	out := c.Capture(t)

	// Everything outside the corner overlay is the untouched scene.
	cw, ch := windowWidth/4, windowHeight/4
	for y := 0; y < windowHeight; y++ {
		for x := 0; x < windowWidth; x++ {
			if x < cw && y < ch {
				continue
			}
			if out.RGBAAt(x, y) != base.RGBAAt(x, y) {
				t.Fatalf("pixel (%d,%d) outside the overlay changed: %v -> %v", x, y, base.RGBAAt(x, y), out.RGBAAt(x, y))
			}
		}
	}

	// Overlay pixels whose whole 3x3 source neighbourhood (in 4x4 blocks) has
	// one class must show exactly that class, which also proves the
	// bilinear downscale did not leak unconverted colours.
	seen := map[color.RGBA]int{}
	blockClass := func(bx, by int) (color.RGBA, bool) {
		cl := classify(base.RGBAAt(bx*4, by*4))
		for y := by * 4; y < by*4+4; y++ {
			for x := bx * 4; x < bx*4+4; x++ {
				if classify(base.RGBAAt(x, y)) != cl {
					return cl, false
				}
			}
		}
		return cl, true
	}
	for by := 1; by < ch-1; by++ {
		for bx := 1; bx < cw-1; bx++ {
			cl, ok := blockClass(bx, by)
			if !ok {
				continue
			}
			uniform := true
			for dy := -1; dy <= 1 && uniform; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if n, ok := blockClass(bx+dx, by+dy); !ok || n != cl {
						uniform = false
						break
					}
				}
			}
			if !uniform {
				continue
			}
			if got := out.RGBAAt(bx, by); got != cl {
				t.Fatalf("overlay (%d,%d) = %v, want %v", bx, by, got, cl)
			}
			seen[cl]++
		}
	}
	// Pure black background becomes red, bright content white, dark content black.
	for _, cl := range []color.RGBA{red, white, black} {
		if seen[cl] == 0 {
			t.Errorf("no verified overlay pixel of class %v (seen %v)", cl, seen)
		}
	}
	sdltest.AssertPixel(t, out, 2, 2, red)
}

func TestIterateIsRepeatableAndReusesConversionTexture(t *testing.T) {
	a, c, _ := newTestApp(t)
	iterate(t, a)
	tex := a.converted
	if tex == nil || a.convW != windowWidth || a.convH != windowHeight {
		t.Fatalf("conversion texture %v %dx%d, want %dx%d", tex, a.convW, a.convH, windowWidth, windowHeight)
	}
	f1 := c.Capture(t)
	iterate(t, a)
	if a.converted != tex {
		t.Error("conversion texture must be reused while the size is unchanged")
	}
	if !bytes.Equal(f1.Pix, c.Capture(t).Pix) {
		t.Error("same time must give the same frame")
	}
}

func TestConversionTextureFollowsWindowResize(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, windowWidth, windowHeight)
	var r *sdl.Renderer
	sdltest.OnMain(t, func() { r = sdl.CreateRenderer(win, "software") })
	if r == nil {
		t.Skipf("no window renderer: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	var clock sdltest.Clock
	var a *app
	var err error
	sdltest.OnMain(t, func() {
		sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		a, err = newApp(r, clock.Ticks)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })

	iterate(t, a)
	if a.convW != windowWidth || a.convH != windowHeight {
		t.Fatalf("initial conversion size %dx%d", a.convW, a.convH)
	}
	sdltest.ResizeWindow(t, win, 800, 600)
	iterate(t, a)
	if a.convW != 800 || a.convH != 600 {
		t.Fatalf("conversion size after resize %dx%d, want 800x600", a.convW, a.convH)
	}
	// The overlay still occupies the logical top-left quarter, now letterboxed
	// to a 4:3 area that exactly fills the 800x600 window.
	img := sdltest.Capture(t, r)
	if img.Rect.Dx() != 800 || img.Rect.Dy() != 600 {
		t.Fatalf("capture %v", img.Rect)
	}
	sdltest.AssertPixel(t, img, 2, 2, red)
	sdltest.AssertPixel(t, img, 700, 10, black)
}

func TestCloseReleasesTextures(t *testing.T) {
	a, _, _ := newTestApp(t)
	iterate(t, a)
	sdltest.OnMain(t, a.close)
	if a.texture != nil || a.converted != nil {
		t.Fatal("close must drop texture handles")
	}
	sdltest.OnMain(t, a.close) // idempotent
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
