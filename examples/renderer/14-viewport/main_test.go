package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var black = color.RGBA{0, 0, 0, 255}

// sample decodes the embedded PNG as an independent reference.
func sample(t *testing.T) *image.RGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	for i := 3; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 255 {
			t.Fatal("sample.png must be opaque for the pixel model to be exact")
		}
	}
	return out
}

// blit copies src to dst at (ox, oy) clipped to clip, like a viewport draw.
func blit(dst, src *image.RGBA, ox, oy int, clip image.Rectangle) {
	r := src.Bounds().Add(image.Pt(ox, oy)).Intersect(clip).Intersect(dst.Bounds())
	draw.Draw(dst, r, src, r.Min.Sub(image.Pt(ox, oy)), draw.Src)
}

// expectedFrame models the C program's four draws on a black 640x480 frame.
func expectedFrame(ref *image.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(black), image.Point{}, draw.Src)
	full := img.Bounds()
	blit(img, ref, 0, 0, full)
	blit(img, ref, windowWidth/2, windowHeight/2, image.Rect(windowWidth/2, windowHeight/2, windowWidth, windowHeight))
	blit(img, ref, 0, windowHeight-windowHeight/5, image.Rect(0, windowHeight-windowHeight/5, windowWidth/5, windowHeight))
	blit(img, ref, 100, 200-50, image.Rect(100, 200, 100+windowWidth, 200+windowHeight))
	return img
}

// capture resets the viewport (RenderReadPixels with a nil rect reads the
// current viewport, which draw leaves at its last value) and reads the frame.
func capture(t *testing.T, r *sdl.Renderer) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() { sdl.SetRenderViewport(r, nil) })
	return sdltest.Capture(t, r)
}

func newTestApp(t *testing.T, r *sdl.Renderer) *app {
	t.Helper()
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp(r) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a
}

func TestTextureMatchesEmbeddedPNG(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a := newTestApp(t, c.Renderer)
	ref := sample(t)
	if int(a.width) != ref.Bounds().Dx() || int(a.height) != ref.Bounds().Dy() {
		t.Fatalf("app size %vx%v, png %v", a.width, a.height, ref.Bounds().Size())
	}
}

func TestDrawFrame(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a := newTestApp(t, c.Renderer)
	ref := sample(t)
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	got := capture(t, c.Renderer)
	want := expectedFrame(ref)
	if !bytes.Equal(got.Pix, want.Pix) {
		for y := 0; y < windowHeight; y++ {
			for x := 0; x < windowWidth; x++ {
				if g, w := sdltest.PixelAt(got, x, y), want.RGBAAt(x, y); g != w {
					t.Fatalf("first mismatch at (%d,%d): got %v want %v", x, y, g, w)
				}
			}
		}
	}
}

func TestViewportClipsAndRepositions(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a := newTestApp(t, c.Renderer)
	ref := sample(t)
	w, h := ref.Bounds().Dx(), ref.Bounds().Dy()
	if w <= 100 || h <= 100 {
		t.Skip("sample too small for the geometry assertions")
	}
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	var vp sdl.Rect
	sdltest.OnMain(t, func() { sdl.GetRenderViewport(c.Renderer, &vp) })
	if want := (sdl.Rect{X: 100, Y: 200, W: windowWidth, H: windowHeight}); vp != want {
		t.Fatalf("viewport = %+v, want %+v", vp, want)
	}
	img := capture(t, c.Renderer)
	// Last viewport draw: texture origin moves to (100,150) but rows above y=200 are clipped,
	// so the area just right of the first draw and above y=200 stays black.
	sdltest.AssertPixel(t, img, windowWidth-1, 0, black)
	// Bottom-right quarter shows texture pixel (0,0) at its origin.
	sdltest.AssertPixel(t, img, windowWidth/2, windowHeight/2, ref.RGBAAt(0, 0))
	// Bottom-left 20% viewport shows the texture origin, and clips its width to 128 px.
	sdltest.AssertPixel(t, img, 0, windowHeight-windowHeight/5, ref.RGBAAt(0, 0))
	// Offset draw: pixel (100,200) shows texture row 50 (shifted by -50), column 0.
	sdltest.AssertPixel(t, img, 100, 200, ref.RGBAAt(0, 50))
}

func TestHandleEvent(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	a := newTestApp(t, c.Renderer)
	in := sdltest.NewInput(t, 0)

	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(5, 5, 1, 1, 0)
	in.MouseButton(1, true, 5, 5)
	in.MouseWheel(0, 1)
	in.Resized(800, 600)
	in.CloseRequested()
	in.Text("x")
	var quit bool
	var types []sdl.EventType
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			types = append(types, sdl.EventType(e.Type()))
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	if quit {
		t.Fatalf("non-quit events must not quit; saw %v", types)
	}
	in.Quit()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if sdl.EventType(e.Type()) != sdl.EVENT_QUIT {
				t.Errorf("unexpected event %v", e.Type())
			}
			quit = a.handleEvent(&e)
		}
	})
	if !quit {
		t.Fatal("EVENT_QUIT must end the program")
	}
}

func TestLetterboxedWindow(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, windowWidth*2, windowHeight)
	var r *sdl.Renderer
	sdltest.OnMain(t, func() { r = sdl.CreateRenderer(win, "software") })
	if r == nil {
		t.Skip("no software renderer for window")
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	sdltest.OnMain(t, func() { configure(r) })
	a := newTestApp(t, r)
	ref := sample(t)
	sdltest.OnMain(t, func() { a.draw(r) })
	// Read the whole output: disable the logical mapping so the viewport is the window.
	sdltest.OnMain(t, func() { sdl.SetRenderLogicalPresentation(r, 0, 0, sdl.LOGICAL_PRESENTATION_DISABLED) })
	img := capture(t, r)
	if img.Bounds().Dx() != windowWidth*2 {
		t.Fatalf("output width %d", img.Bounds().Dx())
	}
	// Scene is centred: 640 px bars on each side of a 640x480 scene at 1:1.
	off := windowWidth / 2
	sdltest.AssertRegion(t, img, image.Rect(0, 0, off, windowHeight), black)
	sdltest.AssertPixel(t, img, off, 0, ref.RGBAAt(0, 0))
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
