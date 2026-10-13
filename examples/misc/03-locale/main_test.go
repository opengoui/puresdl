package main

import (
	"image"
	"image/color"
	"os"
	"strconv"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
)

func fixed(l ...sdl.LocaleInfo) func() []sdl.LocaleInfo {
	return func() []sdl.LocaleInfo { return l }
}

// whiteBounds returns the bounding box of white pixels inside band, or an
// empty rectangle if there are none.
func whiteBounds(img *image.RGBA, band image.Rectangle) image.Rectangle {
	var b image.Rectangle
	for y := band.Min.Y; y < band.Max.Y; y++ {
		for x := band.Min.X; x < band.Max.X; x++ {
			if img.RGBAAt(x, y) == white {
				b = b.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return b
}

// reference renders the expected lines independently: each message drawn at a
// hand-computed x, y with the SDL debug font.
func reference(t *testing.T, lines []string) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, width, height)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 0, 0, 0, 255)
		sdl.RenderClear(c.Renderer)
		sdl.SetRenderDrawColor(c.Renderer, 255, 255, 255, 255)
		for i, msg := range lines {
			x := float32(640-8*len(msg)) / 2
			sdl.RenderDebugText(c.Renderer, x, float32(16*i), msg)
		}
	})
	return c.Capture(t)
}

func drawn(t *testing.T, a *app) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, width, height)
	sdltest.OnMain(t, func() { a.draw(c.Renderer) })
	return c.Capture(t)
}

func samePixels(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("size %v, want %v", got.Rect, want.Rect)
	}
	for i := range got.Pix {
		if got.Pix[i] != want.Pix[i] {
			x, y := i/4%got.Rect.Dx(), i/4/got.Rect.Dx()
			t.Fatalf("first mismatch at pixel (%d,%d): got %v want %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
		}
	}
}

func TestLocaleLine(t *testing.T) {
	for _, tc := range []struct {
		l    sdl.LocaleInfo
		want string
	}{
		{sdl.LocaleInfo{Language: "en", Country: "US"}, " - en_US"},
		{sdl.LocaleInfo{Language: "fr"}, " - fr"},
	} {
		if got := localeLine(tc.l); got != tc.want {
			t.Errorf("localeLine(%+v) = %q, want %q", tc.l, got, tc.want)
		}
	}
}

func TestQuitEventOnly(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(fixed())
	in := sdltest.NewInput(t, 0)

	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(100, 100)
	in.CloseRequested()
	for _, e := range sdltest.Drain(t) {
		e := e
		if a.handleEvent(&e) {
			t.Errorf("event type %#x must not quit", e.Type())
		}
	}

	in.Quit()
	quit := false
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
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestDrawLocaleList(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(fixed(
		sdl.LocaleInfo{Language: "en", Country: "US"},
		sdl.LocaleInfo{Language: "fr"},
		sdl.LocaleInfo{Language: "zh", Country: "CN"},
	))
	got := drawn(t, a)
	want := reference(t, []string{
		"Locales, in order of preference (3 total):",
		" - en_US",
		" - fr",
		" - zh_CN",
	})
	samePixels(t, got, want)

	// Semantic checks independent of the reference: the header is centered
	// within its own 8px-per-character cell and rows are 16px apart.
	hdr := "Locales, in order of preference (3 total):"
	x0 := (640 - 8*len(hdr)) / 2
	b := whiteBounds(got, image.Rect(0, 0, 640, 16))
	if b.Min.X < x0 || b.Max.X > x0+8*len(hdr) || b.Empty() {
		t.Errorf("header bounds %v outside cell [%d,%d)", b, x0, x0+8*len(hdr))
	}
	// Row 4 (index 3) lives at y=48 and nothing is drawn below it.
	sdltest.AssertRegion(t, got, image.Rect(0, 64, 640, 480), black)
	if whiteBounds(got, image.Rect(0, 48, 640, 64)).Empty() {
		t.Error("third locale row is empty")
	}
}

func TestDrawZeroLocales(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	got := drawn(t, newApp(func() []sdl.LocaleInfo { return []sdl.LocaleInfo{} }))
	samePixels(t, got, reference(t, []string{"Locales, in order of preference (0 total):"}))
	sdltest.AssertRegion(t, got, image.Rect(0, 16, 640, 480), black)
}

func TestDrawUnavailableShowsError(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	sdltest.OnMain(t, func() { sdl.ClearError() })
	got := drawn(t, newApp(func() []sdl.LocaleInfo { return nil }))
	// With a cleared error the message is the bare prefix on the first row.
	samePixels(t, got, reference(t, []string{"Couldn't get locales: "}))
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, width, height)
	many := newApp(fixed(sdl.LocaleInfo{Language: "en"}, sdl.LocaleInfo{Language: "de"}, sdl.LocaleInfo{Language: "ja"}))
	few := newApp(fixed(sdl.LocaleInfo{Language: "en"}))
	sdltest.OnMain(t, func() { many.draw(c.Renderer); few.draw(c.Renderer) })
	got := c.Capture(t)
	sdltest.AssertRegion(t, got, image.Rect(0, 32, 640, 480), black)
}

func TestRealLocalesDraw(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	locales := sdltest.Call(t, sdl.GetPreferredLocales)
	if locales == nil {
		t.Skip("no preferred locales on this system")
	}
	for i, l := range locales {
		if l.Language == "" {
			t.Errorf("locale %d has empty language: %+v", i, l)
		}
	}
	got := drawn(t, newApp(fixed(locales...)))
	lines := []string{"Locales, in order of preference (" + strconv.Itoa(len(locales)) + " total):"}
	for _, l := range locales {
		lines = append(lines, localeLine(l))
	}
	samePixels(t, got, reference(t, lines))
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
