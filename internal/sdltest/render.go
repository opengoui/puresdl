package sdltest

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"github.com/opengoui/puresdl/sdl"
)

// Canvas is an offscreen software renderer drawing into a surface. It needs
// no window, so it works wherever SDL video is initialized.
type Canvas struct {
	Renderer *sdl.Renderer
	surface  *sdl.Surface
}

// NewCanvas creates a w x h software renderer on an RGBA surface and destroys
// it on cleanup. SDL video must be initialized (see [Setup]). Pass
// c.Renderer to the drawing code of an example.
func NewCanvas(t testing.TB, w, h int32) *Canvas {
	t.Helper()
	c := &Canvas{}
	var errMsg string
	OnMain(t, func() {
		if c.surface = sdl.CreateSurface(w, h, sdl.PIXELFORMAT_RGBA32); c.surface == nil {
			errMsg = sdl.GetError()
			return
		}
		if c.Renderer = sdl.CreateSoftwareRenderer(c.surface); c.Renderer == nil {
			errMsg = sdl.GetError()
			sdl.DestroySurface(c.surface)
		}
	})
	if errMsg != "" {
		t.Fatalf("create canvas: %s", errMsg)
	}
	t.Cleanup(func() {
		OnMain(t, func() {
			sdl.DestroyRenderer(c.Renderer)
			sdl.DestroySurface(c.surface)
		})
	})
	return c
}

// Capture reads back everything the canvas has drawn so far.
func (c *Canvas) Capture(t testing.TB) *image.RGBA {
	t.Helper()
	return Capture(t, c.Renderer)
}

// Capture reads back the current render target of r (sdl.RenderReadPixels)
// and returns it as RGBA bytes in SDL's byte order. Use it on the renderer an
// example created itself, after its draw function ran.
func Capture(t testing.TB, r *sdl.Renderer) *image.RGBA {
	t.Helper()
	var errMsg string
	img := Call(t, func() *image.RGBA {
		raw := sdl.RenderReadPixels(r, nil)
		if raw == nil {
			errMsg = "SDL_RenderReadPixels: " + sdl.GetError()
			return nil
		}
		defer sdl.DestroySurface(raw)
		s := sdl.ConvertSurface(raw, sdl.PIXELFORMAT_RGBA32)
		if s == nil {
			errMsg = "SDL_ConvertSurface: " + sdl.GetError()
			return nil
		}
		defer sdl.DestroySurface(s)
		img := image.NewRGBA(image.Rect(0, 0, int(s.W), int(s.H)))
		src := unsafe.Slice((*byte)(s.Pixels), int(s.Pitch)*int(s.H))
		for y := 0; y < int(s.H); y++ {
			copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*int(s.Pitch):])
		}
		return img
	})
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	return img
}

// PixelAt returns the color at (x, y).
func PixelAt(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

// AssertPixel fails the test unless the pixel at (x, y) equals want.
func AssertPixel(t testing.TB, img *image.RGBA, x, y int, want color.RGBA) {
	t.Helper()
	if got := img.RGBAAt(x, y); got != want {
		t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
	}
}

// AssertRegion fails the test unless every pixel inside r equals want. It
// reports the first mismatch.
func AssertRegion(t testing.TB, img *image.RGBA, r image.Rectangle, want color.RGBA) {
	t.Helper()
	r = r.Intersect(img.Rect)
	if r.Empty() {
		t.Fatalf("region %v is outside the image %v", r, img.Rect)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if got := img.RGBAAt(x, y); got != want {
				t.Errorf("region %v: pixel (%d,%d) = %v, want %v", r, x, y, got, want)
				return
			}
		}
	}
}
