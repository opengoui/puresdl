package sdltest_test

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func fillRect(t *testing.T, c *sdltest.Canvas, r sdl.FRect, col color.RGBA) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, col.R, col.G, col.B, col.A)
		sdl.RenderFillRect(c.Renderer, &r)
	})
}

func clear(t *testing.T, c *sdltest.Canvas, col color.RGBA) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, col.R, col.G, col.B, col.A)
		sdl.RenderClear(c.Renderer)
	})
}

var (
	black = color.RGBA{0, 0, 0, 255}
	green = color.RGBA{0, 128, 0, 255}
	blue  = color.RGBA{80, 80, 255, 255}
)

func TestCanvasCapturesDrawnRect(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 32, 24)
	clear(t, c, black)
	fillRect(t, c, sdl.FRect{X: 4, Y: 6, W: 10, H: 8}, green)

	img := c.Capture(t)
	if img.Bounds() != image.Rect(0, 0, 32, 24) {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	sdltest.AssertRegion(t, img, image.Rect(4, 6, 14, 14), green)
	sdltest.AssertPixel(t, img, 3, 6, black)
	sdltest.AssertPixel(t, img, 14, 13, black)
	sdltest.AssertPixel(t, img, 4, 14, black)
	if got := sdltest.PixelAt(img, 5, 7); got != green {
		t.Errorf("PixelAt = %v, want %v", got, green)
	}
}

func TestCaptureWindowRenderer(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 40, 30)
	r := sdltest.Call(t, func() *sdl.Renderer { return sdl.CreateRenderer(win, "software") })
	if r == nil {
		t.Fatalf("CreateRenderer: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(r, blue.R, blue.G, blue.B, blue.A)
		sdl.RenderClear(r)
		sdl.RenderPresent(r)
	})
	img := sdltest.Capture(t, r)
	sdltest.AssertRegion(t, img, img.Bounds(), blue)
}

// recorder is a testing.TB that records failures instead of failing the real
// test, so negative paths of the assertion helpers can be checked.
type recorder struct {
	testing.TB
	failed bool
	msgs   []string
}

func (r *recorder) Helper() {}
func (r *recorder) Errorf(f string, a ...any) {
	r.failed = true
	r.msgs = append(r.msgs, fmt.Sprintf(f, a...))
}
func (r *recorder) Fatalf(f string, a ...any) { r.Errorf(f, a...); runtime.Goexit() }
func (r *recorder) Fatal(a ...any)            { r.Errorf("%s", fmt.Sprint(a...)); runtime.Goexit() }

// run executes fn against a recorder on its own goroutine, so Fatal can unwind.
func run(fn func(tb testing.TB)) *recorder {
	r := &recorder{TB: nil}
	done := make(chan struct{})
	go func() { defer close(done); fn(r) }()
	<-done
	return r
}

func setUpdate(t *testing.T, on bool) {
	t.Helper()
	f := flag.Lookup("update")
	prev := f.Value.String()
	val := "false"
	if on {
		val = "true"
	}
	f.Value.Set(val)
	t.Cleanup(func() { f.Value.Set(prev) })
}

func TestGoldenRoundTripAndMismatch(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	t.Chdir(t.TempDir()) // goldens resolve against the working directory

	c := sdltest.NewCanvas(t, 16, 16)
	clear(t, c, black)
	fillRect(t, c, sdl.FRect{X: 2, Y: 2, W: 6, H: 6}, green)
	img := c.Capture(t)

	if r := run(func(tb testing.TB) { sdltest.AssertImage(tb, "box", img) }); !r.failed {
		t.Fatal("AssertImage passed without a golden file")
	}
	if _, err := os.Stat(filepath.Join("testdata", "failed", "box.png")); err != nil {
		t.Fatalf("actual image not written on failure: %v", err)
	}

	setUpdate(t, true)
	sdltest.AssertImage(t, "box", img)
	setUpdate(t, false)
	sdltest.AssertImage(t, "box", img)

	off := image.NewRGBA(img.Rect)
	copy(off.Pix, img.Pix)
	off.Pix[2*off.Stride+2*4+1] += 3 // one channel of one pixel differs by 3
	if r := run(func(tb testing.TB) { sdltest.AssertImage(tb, "box", off) }); !r.failed {
		t.Error("exact comparison accepted a differing pixel")
	}
	sdltest.AssertImageTol(t, "box", off, sdltest.Tolerance{Channel: 3})
	if r := run(func(tb testing.TB) { sdltest.AssertImageTol(tb, "box", off, sdltest.Tolerance{Channel: 2}) }); !r.failed {
		t.Error("channel tolerance 2 accepted a difference of 3")
	}
	sdltest.AssertImageTol(t, "box", off, sdltest.Tolerance{Channel: 2, Pixels: 1})

	small := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if r := run(func(tb testing.TB) {
		sdltest.AssertImageTol(tb, "box", small, sdltest.Tolerance{Channel: 255, Pixels: 1 << 20})
	}); !r.failed {
		t.Error("a size mismatch must fail regardless of tolerance")
	}
}

func TestAssertRegionDetectsStrayPixel(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.SetRGBA(2, 1, green)
	r := run(func(tb testing.TB) { sdltest.AssertRegion(tb, img, img.Rect, color.RGBA{}) })
	if !r.failed {
		t.Fatal("AssertRegion missed a stray pixel")
	}
}
