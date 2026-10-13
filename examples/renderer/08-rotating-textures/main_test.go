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

// sample decodes the embedded PNG independently of SDL.
func sample(t *testing.T) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func nrgba(c color.Color) color.RGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return color.RGBA{n.R, n.G, n.B, n.A}
}

// setup initializes video, creates a 640x480 canvas and a demo on a Clock.
func setup(t *testing.T) (*demo, *sdltest.Canvas, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	var d *demo
	var err error
	sdltest.OnMain(t, func() { d, err = newDemo(c.Renderer, clock.Ticks) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, d.close) })
	return d, c, &clock
}

func frame(t *testing.T, d *demo, c *sdltest.Canvas) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() { d.draw(c.Renderer) })
	return c.Capture(t)
}

func TestRotationAt(t *testing.T) {
	for _, tc := range []struct {
		now  uint64
		want float64
	}{{0, 0}, {500, 90}, {1000, 180}, {1500, 270}, {1999, 359.82}, {2000, 0}, {2500, 90}, {123456000, 0}} {
		if got := rotationAt(tc.now); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("rotationAt(%d) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestTextureSizeMatchesPNG(t *testing.T) {
	d, _, _ := setup(t)
	b := sample(t).Bounds()
	if d.width != float32(b.Dx()) || d.height != float32(b.Dy()) {
		t.Fatalf("demo size %vx%v, PNG %v", d.width, d.height, b.Size())
	}
	var w, h float32
	if !sdltest.Call(t, func() bool { return sdl.GetTextureSize(d.texture, &w, &h) }) {
		t.Fatal(sdl.GetError())
	}
	if w != d.width || h != d.height {
		t.Errorf("texture size %vx%v, want %vx%v", w, h, d.width, d.height)
	}
}

func TestInitialFrameShowsCenteredUnrotatedImage(t *testing.T) {
	d, c, _ := setup(t)
	img := frame(t, d, c)
	src := sample(t)
	b := src.Bounds()
	x0 := (windowWidth - b.Dx()) / 2
	y0 := (windowHeight - b.Dy()) / 2
	// Pixel-exact at rotation 0 on a sparse grid of the image.
	opaque := 0
	for y := 0; y < b.Dy(); y += 7 {
		for x := 0; x < b.Dx(); x += 7 {
			want := nrgba(src.At(b.Min.X+x, b.Min.Y+y))
			if want.A != 255 {
				continue
			}
			opaque++
			sdltest.AssertPixel(t, img, x0+x, y0+y, want)
		}
	}
	if opaque == 0 {
		t.Fatal("no opaque sample pixels checked")
	}
	black := color.RGBA{0, 0, 0, 255}
	sdltest.AssertRegion(t, img, image.Rect(0, 0, windowWidth, y0), black)
	sdltest.AssertRegion(t, img, image.Rect(0, y0+b.Dy(), windowWidth, windowHeight), black)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, x0, windowHeight), black)
	sdltest.AssertRegion(t, img, image.Rect(x0+b.Dx(), 0, windowWidth, windowHeight), black)
}

func TestQuarterTurnRotatesAroundCenter(t *testing.T) {
	d, c, clock := setup(t)
	clock.Advance(500) // 90 degrees
	img := frame(t, d, c)
	b := sample(t).Bounds()
	w, h := b.Dx(), b.Dy()
	x0, y0 := (windowWidth-w)/2, (windowHeight-h)/2
	black := color.RGBA{0, 0, 0, 255}
	// Corners of the unrotated rectangle fall outside the rotated one.
	for _, p := range [][2]int{{x0 + 2, y0 + 2}, {x0 + w - 3, y0 + 2}, {x0 + 2, y0 + h - 3}, {x0 + w - 3, y0 + h - 3}} {
		sdltest.AssertPixel(t, img, p[0], p[1], black)
	}
	// The rotated image now extends vertically beyond the original rect.
	cx, cy := windowWidth/2, windowHeight/2
	if cy-w/2+5 >= y0 {
		t.Fatal("test geometry assumption broken")
	}
	nonBlack := 0
	for y := cy - w/2 + 3; y < y0-3; y++ {
		if img.RGBAAt(cx, y) != black {
			nonBlack++
		}
	}
	if nonBlack == 0 {
		t.Error("no image pixels above the unrotated rectangle after a quarter turn")
	}
	// A quarter turn maps the source's left-middle pixel to the top-middle.
	src := sample(t)
	sb := src.Bounds()
	want := nrgba(src.At(sb.Min.X+3, sb.Min.Y+h/2))
	got := img.RGBAAt(cx, cy-w/2+3)
	if want.A == 255 && (diff(got.R, want.R) > 40 || diff(got.G, want.G) > 40 || diff(got.B, want.B) > 40) {
		t.Errorf("top-middle pixel %v, want near %v (source left edge)", got, want)
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestHalfTurnFlipsImage(t *testing.T) {
	d, c, clock := setup(t)
	clock.Advance(1000)
	img := frame(t, d, c)
	src := sample(t)
	b := src.Bounds()
	x0, y0 := (windowWidth-b.Dx())/2, (windowHeight-b.Dy())/2
	checked, bad := 0, 0
	for y := 10; y < b.Dy()-10; y += 9 {
		for x := 10; x < b.Dx()-10; x += 9 {
			want := nrgba(src.At(b.Min.X+b.Dx()-1-x, b.Min.Y+b.Dy()-1-y))
			if want.A != 255 {
				continue
			}
			checked++
			got := img.RGBAAt(x0+x, y0+y)
			if diff(got.R, want.R) > 48 || diff(got.G, want.G) > 48 || diff(got.B, want.B) > 48 {
				bad++
			}
		}
	}
	if checked == 0 || bad*20 > checked {
		t.Errorf("%d of %d sampled pixels differ from the 180-degree flipped source", bad, checked)
	}
}

func TestFramesRepeatEveryTwoSeconds(t *testing.T) {
	d, c, clock := setup(t)
	clock.Advance(730)
	a := frame(t, d, c)
	clock.Advance(2000)
	b := frame(t, d, c)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("frame at t+2000ms differs from frame at t")
	}
	clock.Advance(500)
	if bytes.Equal(b.Pix, frame(t, d, c).Pix) {
		t.Error("frame did not change as time advanced")
	}
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	d, c, clock := setup(t)
	clock.Advance(500)
	frame(t, d, c)
	clock.Advance(500) // now 180 degrees; the quarter-turn extents must be gone
	img := frame(t, d, c)
	black := color.RGBA{0, 0, 0, 255}
	b := sample(t).Bounds()
	y0 := (windowHeight - b.Dy()) / 2
	sdltest.AssertRegion(t, img, image.Rect(0, 0, windowWidth, y0), black)
}

func TestHandleEvent(t *testing.T) {
	d, _, _ := setup(t)
	in := sdltest.NewInput(t, 0)
	feed := func() (quit bool) {
		sdltest.OnMain(t, func() {
			sdl.PumpEvents()
			var e sdl.Event
			for sdl.PollEvent(&e) {
				if d.handleEvent(&e) {
					quit = true
				}
			}
		})
		return
	}
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(5, 5, 1, 1, 0)
	in.Text("x")
	in.Resized(800, 600)
	in.CloseRequested()
	if feed() {
		t.Error("non-QUIT events must not quit (C ignores everything but SDL_EVENT_QUIT)")
	}
	in.Quit()
	if !feed() {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestLetterboxedWindowRenderer(t *testing.T) {
	sdltest.Skip(t)
	var errStr string
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	var out image.Point
	sdltest.OnMain(t, func() {
		if !sdl.Init(sdl.INIT_VIDEO) {
			errStr = sdl.GetError()
			return
		}
		defer sdl.Quit()
		var win *sdl.Window
		var r *sdl.Renderer
		if !sdl.CreateWindowAndRenderer("t", 1280, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIDDEN, &win, &r) {
			errStr = sdl.GetError()
			return
		}
		defer sdl.DestroyWindow(win)
		defer sdl.DestroyRenderer(r)
		sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		sdl.GetRenderLogicalPresentation(r, &w, &h, &mode)
		var ow, oh int32
		sdl.GetRenderOutputSize(r, &ow, &oh)
		out = image.Pt(int(ow), int(oh))
	})
	if errStr != "" {
		t.Fatal(errStr)
	}
	if w != windowWidth || h != windowHeight || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("logical presentation %dx%d mode %v", w, h, mode)
	}
	if out.X != 1280 || out.Y != 480 {
		t.Errorf("output size %v, want 1280x480", out)
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
