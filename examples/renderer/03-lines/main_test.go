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
	grey  = color.RGBA{100, 100, 100, 255}
	brown = color.RGBA{127, 49, 32, 255}
	green = color.RGBA{0, 255, 0, 255}
)

func frame(t *testing.T, c *sdltest.Canvas, seed uint64) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.Srand(seed)
		draw(c.Renderer)
	})
	return c.Capture(t)
}

func TestFixedGeometry(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	img := frame(t, c, 1)

	sdltest.AssertPixel(t, img, 5, 5, grey)
	sdltest.AssertPixel(t, img, 635, 475, grey)
	// Trunk rectangle outline.
	for _, p := range [][2]int{{300, 450}, {300, 356}, {240, 400}, {400, 400}, {240, 450}, {400, 356}} {
		sdltest.AssertPixel(t, img, p[0], p[1], brown)
	}
	sdltest.AssertPixel(t, img, 300, 400, grey) // trunk interior is not filled
	sdltest.AssertPixel(t, img, 300, 451, grey)
	sdltest.AssertPixel(t, img, 239, 400, grey)
	// Tree outline: bottom edge, horizontal ledges, and the diagonal start.
	for _, p := range [][2]int{{300, 354}, {180, 230}, {460, 230}, {100, 354}, {540, 354}} {
		sdltest.AssertPixel(t, img, p[0], p[1], green)
	}
	sdltest.AssertPixel(t, img, 300, 353, grey)
	// Midpoint of the left slope (140,230)-(320,100) is (230,165).
	sdltest.AssertPixel(t, img, 230, 165, green)
	sdltest.AssertPixel(t, img, 320, 150, grey) // interior of the tree
}

func TestSpokesStayInDisc(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	img := frame(t, c, 7)
	centre := image.Pt(320, 80)
	coloured := 0
	for y := 0; y < 200; y++ {
		for x := 200; x < 440; x++ {
			if img.RGBAAt(x, y) == grey {
				continue
			}
			dx, dy := x-centre.X, y-centre.Y
			if dx*dx+dy*dy <= 32*32 {
				coloured++
				continue
			}
			// Outside the disc only the green tree outline may appear.
			if img.RGBAAt(x, y) != green {
				t.Fatalf("unexpected pixel %v at (%d,%d) outside the spoke disc", img.RGBAAt(x, y), x, y)
			}
		}
	}
	if coloured < 200 {
		t.Errorf("only %d spoke pixels drawn", coloured)
	}
	// Spoke endpoints: angle 0 reaches x=350, angle 90 reaches y=110 (tree apex region).
	if img.RGBAAt(340, 80) == grey || img.RGBAAt(300, 80) == grey {
		t.Error("horizontal spokes missing")
	}
}

func TestSpokeColoursRandomPerFrame(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	a := frame(t, c, 1)
	a2 := frame(t, c, 1)
	b := frame(t, c, 2)
	spokes := image.Rect(285, 50, 355, 110)
	same := func(x, y *image.RGBA) bool {
		for py := spokes.Min.Y; py < spokes.Max.Y; py++ {
			for px := spokes.Min.X; px < spokes.Max.X; px++ {
				if x.RGBAAt(px, py) != y.RGBAAt(px, py) {
					return false
				}
			}
		}
		return true
	}
	if !same(a, a2) {
		t.Error("same seed must reproduce the frame")
	}
	if same(a, b) {
		t.Error("different seeds must change spoke colours")
	}
	// Static parts are independent of the seed.
	sdltest.AssertPixel(t, b, 240, 400, brown)
	sdltest.AssertPixel(t, b, 300, 354, green)
}

func TestLetterboxPresentation(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 1280, 720)
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	sdltest.OnMain(t, func() {
		configureRenderer(c.Renderer)
		sdl.GetRenderLogicalPresentation(c.Renderer, &w, &h, &mode)
	})
	if w != 640 || h != 480 || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Fatalf("logical presentation = %dx%d mode %v", w, h, mode)
	}
	img := frame(t, c, 1)
	// ReadPixels returns the letterboxed viewport: 1280x720 fits 960x720 (1.5x).
	if img.Rect.Dx() != 960 || img.Rect.Dy() != 720 {
		t.Fatalf("viewport = %v, want 960x720", img.Rect)
	}
	sdltest.AssertPixel(t, img, 360, 600, brown) // (240,400) scaled
	sdltest.AssertPixel(t, img, 5, 5, grey)
	sdltest.AssertPixel(t, img, 450, 531, green) // (300,354) scaled
}

func TestHandleEvent(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(300, 200)
	in.CloseRequested()
	var quits int
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if handleEvent(&e) {
				quits++
			}
		}
	})
	if quits != 0 {
		t.Errorf("non-quit events caused %d quits", quits)
	}
	in.Quit()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if handleEvent(&e) {
				quits++
			}
		}
	})
	if quits != 1 {
		t.Errorf("quit events = %d, want 1", quits)
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
