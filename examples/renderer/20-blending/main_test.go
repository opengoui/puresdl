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

// feed pumps SDL and applies every queued event to g like the run loop does.
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

// setup initialises SDL and builds a game on a w x h software canvas.
func setup(t *testing.T, w, h int32) (*game, *sdltest.Canvas, *sdltest.Input) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, w, h)
	var g *game
	var err error
	sdltest.OnMain(t, func() { g, err = newGame(c.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, g.close) })
	return g, c, sdltest.NewInput(t, 0)
}

func render(t *testing.T, g *game, c *sdltest.Canvas) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	return c.Capture(t)
}

// at returns the pixel at offset (dx, dy) inside panel i.
func at(img *image.RGBA, g *game, i int, dx, dy float32) color.RGBA {
	p := g.panels[i]
	return sdltest.PixelAt(img, int(p.X+dx), int(p.Y+dy))
}

func press(t *testing.T, g *game, in *sdltest.Input, sc sdl.Scancode, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		in.Press(sc)
	}
	feed(t, g)
}

func TestInitialLayout(t *testing.T) {
	g, _, _ := setup(t, windowWidth, windowHeight)
	if g.alpha != 255 {
		t.Errorf("initial alpha = %d, want 255", g.alpha)
	}
	// Panels form a 3x2 grid of equal squares that never overlap or leave the window.
	for i, p := range g.panels {
		if p.W != panelSize || p.H != panelSize {
			t.Errorf("panel %d size %vx%v", i, p.W, p.H)
		}
		if p.X < 0 || p.Y < 0 || p.X+p.W > windowWidth || p.Y+p.H > windowHeight {
			t.Errorf("panel %d outside window: %+v", i, p)
		}
		for j := i + 1; j < len(g.panels); j++ {
			q := g.panels[j]
			if p.X < q.X+q.W && q.X < p.X+p.W && p.Y < q.Y+q.H && q.Y < p.Y+p.H {
				t.Errorf("panels %d and %d overlap", i, j)
			}
		}
	}
	for i, tex := range g.textures {
		if tex == nil {
			t.Errorf("texture %d missing", i)
		}
	}
}

func TestAlphaKeys(t *testing.T) {
	g, _, in := setup(t, windowWidth, windowHeight)
	press(t, g, in, sdl.SCANCODE_UP, 1)
	if g.alpha != 255 {
		t.Errorf("UP at max changed alpha to %d", g.alpha)
	}
	press(t, g, in, sdl.SCANCODE_DOWN, 1)
	if g.alpha != 247 {
		t.Errorf("after DOWN alpha = %d, want 247", g.alpha)
	}
	press(t, g, in, sdl.SCANCODE_UP, 1)
	if g.alpha != 255 {
		t.Errorf("after UP alpha = %d, want 255", g.alpha)
	}
	// 255 - 31*8 = 7 < 8: DOWN stops there and never wraps below zero.
	press(t, g, in, sdl.SCANCODE_DOWN, 40)
	if g.alpha != 7 {
		t.Errorf("alpha floor = %d, want 7", g.alpha)
	}
	press(t, g, in, sdl.SCANCODE_UP, 1)
	if g.alpha != 15 {
		t.Errorf("UP from floor = %d, want 15", g.alpha)
	}
	// 15 + 30*8 = 255 exactly; further UP does nothing.
	press(t, g, in, sdl.SCANCODE_UP, 35)
	if g.alpha != 255 {
		t.Errorf("alpha ceiling = %d, want 255", g.alpha)
	}
}

func TestOtherEventsIgnored(t *testing.T) {
	g, _, in := setup(t, windowWidth, windowHeight)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.Press(sdl.SCANCODE_LEFT)
	in.MouseButton(1, true, 10, 10)
	in.MouseWheel(0, 1)
	in.Resized(800, 600)
	if feed(t, g) {
		t.Error("non-quit events requested quit")
	}
	if g.alpha != 255 {
		t.Errorf("alpha = %d", g.alpha)
	}
	// Key release alone must not change alpha.
	in.Key(false, sdl.SCANCODE_DOWN, sdl.SDLK_DOWN, 0)
	feed(t, g)
	if g.alpha != 255 {
		t.Errorf("key up changed alpha to %d", g.alpha)
	}
}

func TestQuit(t *testing.T) {
	g, _, in := setup(t, windowWidth, windowHeight)
	in.Quit()
	if !feed(t, g) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestBlendModeSupport(t *testing.T) {
	g, _, _ := setup(t, windowWidth, windowHeight)
	for i, m := range g.blendModes {
		ok := sdltest.Call(t, func() bool { return sdl.SetTextureBlendMode(g.textures[0], m) })
		builtin := i < 5
		if builtin && !ok {
			t.Errorf("software renderer rejected built-in mode %s", blendModeNames[i])
		}
		got := sdltest.Call(t, func() sdl.BlendMode {
			var bm sdl.BlendMode
			sdl.GetTextureBlendMode(g.textures[0], &bm)
			return bm
		})
		if ok && got != m {
			t.Errorf("mode %d read back %#x, want %#x", i, uint32(got), uint32(m))
		}
	}
}

var (
	gray70  = color.RGBA{70, 70, 70, 255}
	gray110 = color.RGBA{110, 110, 110, 255}
)

func isChecker(c color.RGBA) bool { return c == gray70 || c == gray110 }

func TestRenderedPanels(t *testing.T) {
	g, c, _ := setup(t, windowWidth, windowHeight)
	img := render(t, g, c)

	// Outside every panel the window stays black.
	sdltest.AssertPixel(t, img, 2, 2, color.RGBA{0, 0, 0, 255})

	// Panel 0 (NONE): squares replace the background; blue is on top of the overlap.
	sdltest.AssertPixel(t, img, int(g.panels[0].X+40), int(g.panels[0].Y+40), color.RGBA{255, 0, 0, 255})
	if got := at(img, g, 0, 60, 60); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("NONE green-only = %v", got)
	}
	if got := at(img, g, 0, 78, 78); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("NONE overlap = %v", got)
	}
	// Panel 1 (BLEND) at full alpha is opaque too.
	if got := at(img, g, 1, 40, 40); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("BLEND red = %v", got)
	}
	// ADD: red added to the grey checker.
	if got := at(img, g, 2, 40, 40); got.R != 255 || got.G != got.B || !isChecker(color.RGBA{got.G, got.G, got.B, 255}) {
		t.Errorf("ADD red-only = %v", got)
	}
	// MOD: red modulates the grey checker.
	if got := at(img, g, 3, 40, 40); got.G != 0 || got.B != 0 || (got.R != 70 && got.R != 110) {
		t.Errorf("MOD red-only = %v", got)
	}
	// Checker backgrounds are visible away from the squares.
	for i := range g.panels {
		if got := at(img, g, i, 5, 5); !isChecker(got) {
			t.Errorf("panel %d background = %v", i, got)
		}
	}
}

func TestUnsupportedMarker(t *testing.T) {
	g, c, _ := setup(t, windowWidth, windowHeight)
	supported := sdltest.Call(t, func() bool { return sdl.SetTextureBlendMode(g.textures[0], g.blendModes[5]) })
	img := render(t, g, c)
	p := g.panels[5]
	box := image.Rect(int(p.X+(p.W-104)/2), int(p.Y+p.H-8), int(p.X+(p.W+104)/2), int(p.Y+p.H))
	black, white := 0, 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			switch sdltest.PixelAt(img, x, y) {
			case color.RGBA{0, 0, 0, 255}:
				black++
			case color.RGBA{255, 255, 255, 255}:
				white++
			}
		}
	}
	if supported {
		if white != 0 {
			t.Errorf("supported mode still drew [UNSUPPORTED] text (%d white pixels)", white)
		}
		return
	}
	if black == 0 || white == 0 {
		t.Errorf("unsupported marker missing: black=%d white=%d", black, white)
	}
	// Panels with built-in modes never show the marker box.
	q := g.panels[1]
	if got := sdltest.PixelAt(img, int(q.X+q.W/2), int(q.Y+q.H-4)); got == (color.RGBA{0, 0, 0, 255}) {
		t.Error("marker drawn on a supported panel")
	}
}

func countColor(img *image.RGBA, r image.Rectangle, c color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) == c {
				n++
			}
		}
	}
	return n
}

func TestLabelsAreDrawn(t *testing.T) {
	g, c, _ := setup(t, windowWidth, windowHeight)
	img := render(t, g, c)
	white := color.RGBA{255, 255, 255, 255}
	for i, p := range g.panels {
		r := image.Rect(int(p.X), int(p.Y)-15, int(p.X+p.W), int(p.Y)-7)
		if countColor(img, r, white) == 0 {
			t.Errorf("no label pixels above panel %d", i)
		}
	}
	if countColor(img, image.Rect(0, windowHeight-30, windowWidth, windowHeight), white) == 0 {
		t.Error("no help/alpha text")
	}
}

func TestAlphaChangesRendering(t *testing.T) {
	g, c, in := setup(t, windowWidth, windowHeight)
	full := render(t, g, c)
	// 31 DOWN presses reach the floor alpha 7.
	press(t, g, in, sdl.SCANCODE_DOWN, 31)
	low := render(t, g, c)

	// Readout text region changes.
	txt := image.Rect((windowWidth-80)/2, windowHeight-20, (windowWidth-80)/2+80, windowHeight-12)
	if countColor(full, txt, color.RGBA{255, 255, 255, 255}) == countColor(low, txt, color.RGBA{255, 255, 255, 255}) &&
		at(full, g, 1, 0, 0) == at(low, g, 1, 0, 0) {
		// Counts could coincide; compare actual pixels.
		same := true
		for y := txt.Min.Y; y < txt.Max.Y && same; y++ {
			for x := txt.Min.X; x < txt.Max.X; x++ {
				if sdltest.PixelAt(full, x, y) != sdltest.PixelAt(low, x, y) {
					same = false
					break
				}
			}
		}
		if same {
			t.Error("ALPHA readout did not change")
		}
	}
	// BLEND at alpha 7: the red square is almost invisible.
	got := at(low, g, 1, 40, 40)
	if got.R < 70 || got.R > 110+8 || got.G < 60 || got.G > 110 {
		t.Errorf("BLEND at alpha 7 red-only = %v, want near checker grey", got)
	}
	// ADD at alpha 7 only adds a little red.
	if got := at(low, g, 2, 40, 40); got.G != got.B || got.R < got.G || got.R > got.G+10 {
		t.Errorf("ADD at alpha 7 = %v", got)
	}
	// NONE ignores alpha modulation for RGB.
	if got := at(low, g, 0, 40, 40); got.R != 255 || got.G != 0 || got.B != 0 {
		t.Errorf("NONE at alpha 7 = %v", got)
	}
}

func TestLogicalPresentationScales(t *testing.T) {
	g, c, _ := setup(t, 2*windowWidth, 2*windowHeight)
	img := render(t, g, c)
	p := g.panels[0]
	// Logical coordinates are doubled in the 2x canvas.
	sdltest.AssertPixel(t, img, int(2*(p.X+40)), int(2*(p.Y+40)), color.RGBA{255, 0, 0, 255})
	sdltest.AssertRegion(t, img, image.Rect(2*int(p.X+40), 2*int(p.Y+40), 2*int(p.X+50), 2*int(p.Y+50)), color.RGBA{255, 0, 0, 255})
}

func TestLogicalPresentationMode(t *testing.T) {
	_, c, _ := setup(t, windowWidth, windowHeight)
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	sdltest.OnMain(t, func() { sdl.GetRenderLogicalPresentation(c.Renderer, &w, &h, &mode) })
	if w != windowWidth || h != windowHeight || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("logical presentation = %dx%d mode %v", w, h, mode)
	}
}

func TestCloseReleasesTextures(t *testing.T) {
	g, _, _ := setup(t, windowWidth, windowHeight)
	sdltest.OnMain(t, g.close)
	for i, tex := range g.textures {
		if tex != nil {
			t.Errorf("texture %d not released", i)
		}
	}
	sdltest.OnMain(t, g.close) // idempotent
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
