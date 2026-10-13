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
	white = color.RGBA{255, 255, 255, 255}
	blue  = color.RGBA{51, 102, 255, 255}
	black = color.RGBA{0, 0, 0, 255}
)

// bounds returns the bounding box of all pixels in area that equal c, and
// whether any exist.
func bounds(img *image.RGBA, area image.Rectangle, c color.RGBA) (image.Rectangle, bool) {
	var b image.Rectangle
	found := false
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) != c {
				continue
			}
			p := image.Rect(x, y, x+1, y+1)
			if !found {
				b, found = p, true
			} else {
				b = b.Union(p)
			}
		}
	}
	return b, found
}

// countNot counts pixels in area that are not black.
func countNot(img *image.RGBA, area image.Rectangle) int {
	n := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) != black {
				n++
			}
		}
	}
	return n
}

func render(t *testing.T, w, h int32, seconds uint64, prep func(*sdl.Renderer)) *image.RGBA {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, w, h)
	var clock sdltest.Clock
	clock.Advance(seconds * 1000)
	d := newDemo(clock.Ticks)
	sdltest.OnMain(t, func() {
		if prep != nil {
			prep(c.Renderer)
		}
		d.draw(c.Renderer)
	})
	return c.Capture(t)
}

func TestTextLinesLandWhereTheCSourcePlacesThem(t *testing.T) {
	img := render(t, windowWidth, windowHeight, 0, nil)
	for _, tc := range []struct {
		name  string
		band  image.Rectangle // generous search band around one line
		col   color.RGBA
		x, y  int
		chars int
	}{
		{"hello", image.Rect(0, 90, 640, 120), white, 272, 100, 12},
		{"debug", image.Rect(0, 140, 640, 170), white, 224, 150, 24},
		{"colors", image.Rect(0, 190, 640, 220), blue, 184, 200, 34},
	} {
		b, ok := bounds(img, tc.band, tc.col)
		if !ok {
			t.Errorf("%s: no pixels of %v", tc.name, tc.col)
			continue
		}
		box := image.Rect(tc.x, tc.y, tc.x+tc.chars*8, tc.y+8)
		if !b.In(box) {
			t.Errorf("%s: ink %v escapes text box %v", tc.name, b, box)
		}
		if b.Min.X != tc.x && b.Min.X > tc.x+8 {
			t.Errorf("%s: ink starts at x=%d, expected near %d", tc.name, b.Min.X, tc.x)
		}
	}
	// The blue line must not contain white ink and vice versa.
	if _, ok := bounds(img, image.Rect(0, 190, 640, 220), white); ok {
		t.Error("white ink in the blue line")
	}
	if _, ok := bounds(img, image.Rect(0, 90, 640, 170), blue); ok {
		t.Error("blue ink in the white lines")
	}
}

func TestScaledTextIsFourTimesLarger(t *testing.T) {
	img := render(t, windowWidth, windowHeight, 0, nil)
	// (14,65)*4 = (56,260); 17 chars of 8 px at scale 4 = 544 x 32.
	b, ok := bounds(img, image.Rect(0, 240, 640, 320), white)
	if !ok {
		t.Fatal("no scaled text")
	}
	box := image.Rect(56, 260, 56+17*32, 260+32)
	if !b.In(box) {
		t.Errorf("scaled ink %v escapes %v", b, box)
	}
	// Glyph strokes are 4x4 blocks: the solid pixel run in a row is >= 4 wide.
	if b.Dy() < 16 {
		t.Errorf("scaled ink height %d suggests the scale was not applied", b.Dy())
	}
}

func TestScaleIsRestoredAfterDraw(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	d := newDemo((&sdltest.Clock{}).Ticks)
	var sx, sy float32
	sdltest.OnMain(t, func() {
		d.draw(c.Renderer)
		sdl.GetRenderScale(c.Renderer, &sx, &sy)
	})
	if sx != 1 || sy != 1 {
		t.Errorf("render scale = %v,%v, want 1,1", sx, sy)
	}
}

func TestNonASCIIEmojiOccupiesExactlyOneCell(t *testing.T) {
	img := render(t, windowWidth, windowHeight, 0, nil)
	// SDL decodes UTF-8 into code points: the emoji is one (placeholder) cell,
	// not four garbage bytes, and nothing follows it.
	prefix := len("This only does ASCII chars. So this laughing emoji won't draw: ")
	cell := 64 + prefix*8
	if n := countNot(img, image.Rect(cell+8, 345, 640, 365)); n != 0 {
		t.Errorf("%d pixels drawn after the emoji cell", n)
	}
	if countNot(img, image.Rect(64, 345, cell, 365)) == 0 {
		t.Error("ASCII part of the sentence was not drawn")
	}
	if cell < 0 || cell > 640-8 {
		t.Fatalf("test geometry off: cell x=%d", cell)
	}
}

func TestFooterReflectsElapsedSeconds(t *testing.T) {
	footer := image.Rect(0, 395, 640, 415)
	at0 := render(t, windowWidth, windowHeight, 0, nil)
	at0b := render(t, windowWidth, windowHeight, 0, nil)
	at7 := render(t, windowWidth, windowHeight, 7, nil)
	at123 := render(t, windowWidth, windowHeight, 123, nil)

	b, ok := bounds(at0, footer, white)
	if !ok {
		t.Fatal("no footer text")
	}
	// x = (640 - 8*46)/2 = 136, 52 chars incl. the one digit.
	if !b.In(image.Rect(136, 400, 136+8*52, 408)) {
		t.Errorf("footer ink %v outside expected box", b)
	}
	if !at0.Rect.Eq(at0b.Rect) || string(at0.Pix) != string(at0b.Pix) {
		t.Error("same time must render identically")
	}
	if string(at0.Pix) == string(at7.Pix) {
		t.Error("changing seconds 0 -> 7 did not change the frame")
	}
	// Only the footer may differ.
	if !diffWithin(at0, at7, footer) || !diffWithin(at0, at123, footer) {
		t.Error("frames differ outside the footer")
	}
	// Whole seconds: 999 ms still shows 0 seconds.
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	clock.Advance(999)
	sdltest.OnMain(t, func() { newDemo(clock.Ticks).draw(c.Renderer) })
	if string(c.Capture(t).Pix) != string(at0.Pix) {
		t.Error("999 ms must render as 0 seconds")
	}
	// 123 s is two digits wider than 7 s: ink extends further right.
	b7, _ := bounds(at7, footer, white)
	b123, _ := bounds(at123, footer, white)
	if b123.Max.X <= b7.Max.X {
		t.Errorf("123 s ink ends at %d, not past 7 s at %d", b123.Max.X, b7.Max.X)
	}
}

func diffWithin(a, b *image.RGBA, area image.Rectangle) bool {
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			if (image.Point{x, y}).In(area) {
				continue
			}
			if sdltest.PixelAt(a, x, y) != sdltest.PixelAt(b, x, y) {
				return false
			}
		}
	}
	return true
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	img := render(t, windowWidth, windowHeight, 0, func(r *sdl.Renderer) {
		sdl.SetRenderDrawColor(r, 200, 0, 0, 255)
		sdl.RenderFillRect(r, nil)
	})
	if _, ok := bounds(img, image.Rect(0, 0, 640, 480), color.RGBA{200, 0, 0, 255}); ok {
		t.Error("stale content survived the clear")
	}
	sdltest.AssertRegion(t, img, image.Rect(0, 0, 100, 90), black)
}

func TestLetterboxMapsLogicalToCenteredRect(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 1280, windowHeight)
	var r sdl.FRect
	sdltest.OnMain(t, func() {
		sdl.SetRenderLogicalPresentation(c.Renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		sdl.GetRenderLogicalPresentationRect(c.Renderer, &r)
		newDemo((&sdltest.Clock{}).Ticks).draw(c.Renderer)
	})
	// A 640x480 logical frame in a 1280x480 target is centered with 320 px bars.
	if r.X != 320 || r.Y != 0 || r.W != 640 || r.H != 480 {
		t.Errorf("presentation rect = %+v, want {320 0 640 480}", r)
	}
	// Readback is in logical coordinates, so the drawing is unchanged by it.
	base := render(t, windowWidth, windowHeight, 0, nil)
	if got := c.Capture(t); string(got.Pix) != string(base.Pix) {
		t.Error("logical-presentation frame differs from the plain 640x480 frame")
	}
}

func TestQuitEventStopsAndOthersDoNot(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	d := newDemo((&sdltest.Clock{}).Ticks)
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
	in.Press(sdl.SCANCODE_ESCAPE) // the C program ignores keys
	in.MouseButton(1, true, 5, 5)
	in.Text("q")
	in.Resized(800, 600)
	in.CloseRequested() // window close is not EVENT_QUIT
	if feed() {
		t.Error("non-quit events stopped the program")
	}
	in.Quit()
	if !feed() {
		t.Error("EVENT_QUIT did not stop the program")
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
