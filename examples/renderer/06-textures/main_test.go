package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// decodeSample decodes the embedded PNG with the Go standard library, giving
// an SDL-independent reference for the texture contents.
func decodeSample(t *testing.T) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(samplePNG))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// newTestScene builds a scene on an offscreen canvas with an injected clock.
func newTestScene(t *testing.T) (*scene, *sdltest.Canvas, *sdltest.Clock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	var clock sdltest.Clock
	var s *scene
	var err error
	sdltest.OnMain(t, func() { s, err = newScene(c.Renderer, clock.Ticks) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, s.close) })
	return s, c, &clock
}

func frameAt(t *testing.T, s *scene, c *sdltest.Canvas, clock *sdltest.Clock, ms uint64) *image.RGBA {
	t.Helper()
	clock.Advance(ms - clock.Ticks())
	sdltest.OnMain(t, func() { s.draw(c.Renderer) })
	return c.Capture(t)
}

func TestNewSceneUploadsWholePNG(t *testing.T) {
	s, _, _ := newTestScene(t)
	ref := decodeSample(t)
	if int(s.texW) != ref.Bounds().Dx() || int(s.texH) != ref.Bounds().Dy() {
		t.Errorf("scene size %vx%v, PNG is %v", s.texW, s.texH, ref.Bounds())
	}
	var w, h float32
	if !sdltest.Call(t, func() bool { return sdl.GetTextureSize(s.texture, &w, &h) }) {
		t.Fatal(sdl.GetError())
	}
	if w != s.texW || h != s.texH {
		t.Errorf("texture size %vx%v, want %vx%v", w, h, s.texW, s.texH)
	}
}

func TestPlacements(t *testing.T) {
	s := &scene{texW: 408, texH: 167}
	cx, cy := float32(640-408)/2, float32(480-167)/2
	for _, tc := range []struct {
		now        uint64
		topX, botX float32
	}{
		{0, 100, 132},  // direction -1, scale 1
		{250, 50, 182}, // scale 0.5
		{500, 0, 232},  // scale 0
		{999, -99.8, 331.8},
		{1000, -100, 332}, // direction flips to +1, scale -1
		{1500, 0, 232},
		{1999, 99.8, 132.2},
		{2000, 100, 132}, // period is 2000 ms
	} {
		p := s.placements(tc.now)
		if math.Abs(float64(p[0].X-tc.topX)) > 0.01 || math.Abs(float64(p[2].X-tc.botX)) > 0.01 {
			t.Errorf("now=%d: top X %v bottom X %v, want %v %v", tc.now, p[0].X, p[2].X, tc.topX, tc.botX)
		}
		if p[0].Y != 0 || p[2].Y != 480-167 {
			t.Errorf("now=%d: Y %v %v", tc.now, p[0].Y, p[2].Y)
		}
		if p[1] != (sdl.FRect{X: cx, Y: cy, W: 408, H: 167}) {
			t.Errorf("now=%d: centre %+v", tc.now, p[1])
		}
		for i, r := range p {
			if r.W != 408 || r.H != 167 {
				t.Errorf("now=%d: rect %d size %vx%v", tc.now, i, r.W, r.H)
			}
		}
	}
}

// expectedFrame composites the reference PNG onto black at integer offsets.
func expectedFrame(ref *image.NRGBA, topX, botX, centreY int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	w, h := ref.Bounds().Dx(), ref.Bounds().Dy()
	for _, p := range []image.Point{{topX, 0}, {(windowWidth - w) / 2, centreY}, {botX, windowHeight - h}} {
		draw.Draw(dst, image.Rectangle{p, p.Add(image.Pt(w, h))}, ref, image.Point{}, draw.Over)
	}
	return dst
}

func TestDrawMatchesReferenceComposite(t *testing.T) {
	s, c, clock := newTestScene(t)
	ref := decodeSample(t)
	// Times with integral offsets; the centre Y (156.5) is fractional, so rows
	// within one pixel of the centred copy's vertical edges are excluded.
	for _, tc := range []struct {
		now        uint64
		topX, botX int
	}{{0, 100, 132}, {500, 0, 232}, {1000, -100, 332}, {1500, 0, 232}, {2500, 0, 232}} {
		img := frameAt(t, s, c, clock, tc.now)
		want := expectedFrame(ref, tc.topX, tc.botX, 156)
		bad := 0
		for y := 0; y < windowHeight; y++ {
			if y == 156 || y == 157 || y == 156+167-1 || y == 156+167 {
				continue
			}
			for x := 0; x < windowWidth; x++ {
				if img.RGBAAt(x, y) != want.RGBAAt(x, y) {
					bad++
				}
			}
		}
		if bad != 0 {
			t.Errorf("now=%d: %d pixels differ from the reference composite", tc.now, bad)
		}
	}
}

func TestDrawClearsPreviousFrame(t *testing.T) {
	s, c, clock := newTestScene(t)
	frameAt(t, s, c, clock, 0)
	img := frameAt(t, s, c, clock, 1000) // top copy moved from x=100 to x=-100
	// Pixel (450,10) was covered at t=0 (copy spans 100..508) but not at 1000
	// (spans -100..308) and no other copy reaches row 10.
	sdltest.AssertPixel(t, img, 450, 10, color.RGBA{0, 0, 0, 255})
	sdltest.AssertRegion(t, img, image.Rect(320, 0, 640, 156), color.RGBA{0, 0, 0, 255})
}

func TestHandleEventQuitOnly(t *testing.T) {
	s, _, _ := newTestScene(t)
	in := sdltest.NewInput(t, 0)
	in.Key(true, sdl.SCANCODE_ESCAPE, sdl.SDLK_ESCAPE, 0)
	in.MouseButton(1, true, 5, 5)
	in.MouseMotion(1, 1, 1, 1, 0)
	in.CloseRequested()
	in.Resized(100, 100)
	in.Quit()
	got := map[sdl.EventType]bool{}
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			ty := sdl.EventType(e.Type())
			if q := s.handleEvent(&e); q != (ty == sdl.EVENT_QUIT) {
				got[ty] = q
				t.Errorf("event %v: quit=%v", ty, q)
			}
			got[ty] = true
		}
	})
	if !got[sdl.EVENT_QUIT] || !got[sdl.EVENT_KEY_DOWN] {
		t.Errorf("events not delivered: %v", got)
	}
}

func TestLetterboxOnWiderWindow(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 1280, 480)
	var r *sdl.Renderer
	var rect sdl.FRect
	var lw, lh int32
	var mode sdl.RendererLogicalPresentation
	ok := sdltest.Call(t, func() bool {
		r = sdl.CreateRenderer(win, "software")
		if r == nil {
			return false
		}
		return setLogicalPresentation(r) &&
			sdl.GetRenderLogicalPresentation(r, &lw, &lh, &mode) &&
			sdl.GetRenderLogicalPresentationRect(r, &rect)
	})
	if !ok {
		t.Fatal(sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	if lw != windowWidth || lh != windowHeight || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("logical %dx%d mode %v", lw, lh, mode)
	}
	// 640x480 scaled by 1 inside 1280x480 is centred with 320px bars.
	if rect != (sdl.FRect{X: 320, Y: 0, W: 640, H: 480}) {
		t.Errorf("presentation rect %+v", rect)
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
