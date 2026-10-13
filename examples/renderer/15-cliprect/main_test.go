package main

import (
	"image/color"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var grey = color.RGBA{33, 33, 33, 255}

// feed drains the SDL event queue through g.handleEvent.
func feed(t *testing.T, g *demo) (quit bool) {
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if g.handleEvent(&e) {
				quit = true
			}
		}
	})
	return
}

func newTestDemo(t *testing.T) (*demo, *sdltest.Clock, *sdltest.Canvas) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	g := newDemo(clock.Ticks)
	var err error
	sdltest.OnMain(t, func() { g.texture, err = loadTexture(c.Renderer) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyTexture(g.texture) }) })
	return g, &clock, c
}

func TestInitialFrameClipsTopLeftSquare(t *testing.T) {
	g, _, c := newTestDemo(t)
	g.advance()
	if want := (sdl.Rect{X: 0, Y: 0, W: cliprectSize, H: cliprectSize}); g.clip != want {
		t.Fatalf("clip = %+v, want %+v", g.clip, want)
	}
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	img := c.Capture(t)

	// Outside the clip rectangle only the clear colour is visible.
	sdltest.AssertRegion(t, img, imageRect(cliprectSize, 0, windowWidth, windowHeight), grey)
	sdltest.AssertRegion(t, img, imageRect(0, cliprectSize, cliprectSize, windowHeight), grey)

	// Inside, the texture is drawn: it must match an unclipped render.
	ref := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var refTex *sdl.Texture
	var err error
	sdltest.OnMain(t, func() {
		refTex, err = loadTexture(ref.Renderer) // textures belong to one renderer
		if err != nil {
			return
		}
		sdl.SetRenderDrawColor(ref.Renderer, 33, 33, 33, sdl.ALPHA_OPAQUE)
		sdl.RenderClear(ref.Renderer)
		sdl.RenderTexture(ref.Renderer, refTex, nil, nil)
		sdl.DestroyTexture(refTex)
	})
	if err != nil {
		t.Fatal(err)
	}
	full := ref.Capture(t)
	differs := false
	bad := 0
	for y := 0; y < cliprectSize; y++ {
		for x := 0; x < cliprectSize; x++ {
			got, want := sdltest.PixelAt(img, x, y), sdltest.PixelAt(full, x, y)
			// SDL's software scaler samples clipped blits with a sub-pixel shift, so
			// allow a bounded share of mismatching pixels.
			if absDiff(got.R, want.R) > 8 || absDiff(got.G, want.G) > 8 || absDiff(got.B, want.B) > 8 {
				bad++
			}
			if got != grey {
				differs = true
			}
		}
	}
	if bad > cliprectSize*cliprectSize/6 {
		t.Fatalf("%d of %d clipped pixels differ from the unclipped render", bad, cliprectSize*cliprectSize)
	}
	if !differs {
		t.Fatal("texture not visible inside clip rectangle")
	}
}

func TestDrawAppliesClipRect(t *testing.T) {
	g, clock, c := newTestDemo(t)
	g.advance()
	clock.Advance(1000)
	g.advance() // pos = (200,200); clip still the old (0,0)
	clock.Advance(0)
	g.advance() // clip now (200,200)
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	var got sdl.Rect
	var enabled bool
	sdltest.OnMain(t, func() {
		enabled = sdl.RenderClipEnabled(c.Renderer)
		sdl.GetRenderClipRect(c.Renderer, &got)
	})
	if !enabled || got != (sdl.Rect{X: 200, Y: 200, W: cliprectSize, H: cliprectSize}) {
		t.Fatalf("enabled=%v clip=%+v", enabled, got)
	}
	img := c.Capture(t)
	sdltest.AssertRegion(t, img, imageRect(0, 0, 200, windowHeight), grey)
	sdltest.AssertRegion(t, img, imageRect(450, 0, windowWidth, windowHeight), grey)
	sdltest.AssertRegion(t, img, imageRect(0, 450, windowWidth, windowHeight), grey)
	if sdltest.PixelAt(img, 300, 300) == grey {
		t.Fatal("texture not drawn inside moved clip rectangle")
	}
}

func TestAdvanceMovesAtSpeedAndRounds(t *testing.T) {
	g, clock, _ := newTestDemo(t)
	clock.Advance(13) // 2.6 px
	g.advance()
	if g.posX < 2.59 || g.posX > 2.61 || g.posY != g.posX {
		t.Fatalf("pos = (%v,%v), want 2.6", g.posX, g.posY)
	}
	g.advance() // clip is built from the pre-move position, rounded
	if g.clip.X != 3 || g.clip.Y != 3 {
		t.Fatalf("clip = %+v, want origin (3,3)", g.clip)
	}
	clock.Advance(7) // 1.4 more -> 4.0
	g.advance()
	clock.Advance(1) // -> 4.2 (clip from 4.0)
	g.advance()
	if g.clip.X != 4 {
		t.Fatalf("clip.X = %d, want 4", g.clip.X)
	}
}

func TestBounceAtFarEdges(t *testing.T) {
	g, clock, _ := newTestDemo(t)
	clock.Advance(4000) // +800 px on both axes
	g.advance()
	if g.posX != windowWidth-1 || g.dirX != -1 {
		t.Fatalf("x = %v dir %v, want %d and -1", g.posX, g.dirX, windowWidth-1)
	}
	if g.posY != windowHeight-1 || g.dirY != -1 {
		t.Fatalf("y = %v dir %v, want %d and -1", g.posY, g.dirY, windowHeight-1)
	}
	clock.Advance(1000) // moves back by 200
	g.advance()
	if g.posX != 439 || g.posY != 279 {
		t.Fatalf("pos = (%v,%v), want (439,279)", g.posX, g.posY)
	}
}

func TestBounceAtNearEdges(t *testing.T) {
	g, clock, _ := newTestDemo(t)
	g.dirX, g.dirY = -1, -1
	clock.Advance(2000) // -400 < -250
	g.advance()
	if g.posX != -cliprectSize || g.dirX != 1 || g.posY != -cliprectSize || g.dirY != 1 {
		t.Fatalf("state = %+v", g)
	}
}

func TestAxesBounceIndependently(t *testing.T) {
	g, clock, _ := newTestDemo(t)
	g.posX = 600
	clock.Advance(1000) // x -> 800 (bounce), y -> 200 (fine)
	g.advance()
	if g.dirX != -1 || g.dirY != 1 || g.posY != 200 {
		t.Fatalf("dir = (%v,%v) posY = %v", g.dirX, g.dirY, g.posY)
	}
}

func TestQuitEventEndsProgram(t *testing.T) {
	g, _, _ := newTestDemo(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 5, 5)
	in.Resized(300, 200)
	if feed(t, g) {
		t.Fatal("non-quit events must not end the program")
	}
	in.Quit()
	if !feed(t, g) {
		t.Fatal("EVENT_QUIT did not end the program")
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

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
