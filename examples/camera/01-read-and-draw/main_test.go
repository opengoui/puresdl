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
	gray   = color.RGBA{0x99, 0x99, 0x99, 0xff}
	red    = color.RGBA{0xff, 0, 0, 0xff}
	green  = color.RGBA{0, 0xff, 0, 0xff}
	blue   = color.RGBA{0, 0, 0xff, 0xff}
	yellow = color.RGBA{0xff, 0xff, 0, 0xff}
)

const frameW, frameH = 8, 6

// rig is an app wired to a hidden window, a software renderer and a scripted
// frame source standing in for the camera.
type rig struct {
	app      *app
	window   *sdl.Window
	renderer *sdl.Renderer

	frames   []*sdl.Surface // frames to hand out, one per acquire; nil entries mean "no frame"
	acquired int
	released []*sdl.Surface
}

func newRig(t *testing.T) *rig {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_CAMERA)
	r := &rig{window: sdltest.NewWindow(t, 640, 480)}
	var errMsg string
	sdltest.OnMain(t, func() {
		if r.renderer = sdl.CreateRenderer(r.window, "software"); r.renderer == nil {
			errMsg = sdl.GetError()
		}
	})
	if errMsg != "" {
		t.Fatalf("CreateRenderer: %s", errMsg)
	}
	r.app = newApp(r.window, r.renderer, nil)
	r.app.acquire = func(ts *uint64) *sdl.Surface {
		i := r.acquired
		r.acquired++
		if i >= len(r.frames) {
			return nil
		}
		return r.frames[i]
	}
	r.app.release = func(f *sdl.Surface) { r.released = append(r.released, f) }
	t.Cleanup(func() {
		sdltest.OnMain(t, func() {
			if r.app.texture != nil {
				sdl.DestroyTexture(r.app.texture)
				r.app.texture = nil
			}
			sdl.DestroyRenderer(r.renderer)
		})
	})
	return r
}

// quadrants returns a frameW x frameH surface whose top-left, top-right,
// bottom-left and bottom-right quarters have the given colours.
func quadrants(t *testing.T, tl, tr, bl, br color.RGBA) *sdl.Surface {
	t.Helper()
	var s *sdl.Surface
	sdltest.OnMain(t, func() {
		s = sdl.CreateSurface(frameW, frameH, sdl.PIXELFORMAT_RGBA32)
		if s == nil {
			return
		}
		fill := func(x, y int32, c color.RGBA) {
			sdl.FillSurfaceRect(s, &sdl.Rect{X: x, Y: y, W: frameW / 2, H: frameH / 2}, sdl.MapSurfaceRGBA(s, c.R, c.G, c.B, c.A))
		}
		fill(0, 0, tl)
		fill(frameW/2, 0, tr)
		fill(0, frameH/2, bl)
		fill(frameW/2, frameH/2, br)
	})
	if s == nil {
		t.Fatalf("CreateSurface: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroySurface(s) }) })
	return s
}

// step runs one frame (iterate then capture) and returns the rendered image.
func (r *rig) step(t *testing.T) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, r.app.iterate)
	return sdltest.Capture(t, r.renderer)
}

func (r *rig) windowSize(t *testing.T) (w, h int32) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.SyncWindow(r.window)
		sdl.GetWindowSize(r.window, &w, &h)
	})
	return w, h
}

func assertQuadrants(t *testing.T, img *image.RGBA, tl, tr, bl, br color.RGBA) {
	t.Helper()
	if b := img.Bounds(); b.Dx() != frameW || b.Dy() != frameH {
		t.Fatalf("output is %v, want %dx%d", b, frameW, frameH)
	}
	sdltest.AssertRegion(t, img, image.Rect(0, 0, frameW/2, frameH/2), tl)
	sdltest.AssertRegion(t, img, image.Rect(frameW/2, 0, frameW, frameH/2), tr)
	sdltest.AssertRegion(t, img, image.Rect(0, frameH/2, frameW/2, frameH), bl)
	sdltest.AssertRegion(t, img, image.Rect(frameW/2, frameH/2, frameW, frameH), br)
}

func TestNoFrameClearsToGray(t *testing.T) {
	r := newRig(t)
	img := r.step(t)
	sdltest.AssertRegion(t, img, img.Bounds(), gray)
	if r.app.texture != nil {
		t.Error("texture created without a frame")
	}
	if w, h := r.windowSize(t); w != 640 || h != 480 {
		t.Errorf("window resized to %dx%d without a frame", w, h)
	}
	if len(r.released) != 0 {
		t.Errorf("released %d frames though none were acquired", len(r.released))
	}
}

func TestFirstFrameSizesWindowAndDrawsIt(t *testing.T) {
	r := newRig(t)
	f := quadrants(t, red, green, blue, yellow)
	r.frames = []*sdl.Surface{f}
	img := r.step(t)

	if w, h := r.windowSize(t); w != frameW || h != frameH {
		t.Errorf("window = %dx%d, want the frame size %dx%d", w, h, frameW, frameH)
	}
	var lw, lh int32
	var mode sdl.RendererLogicalPresentation
	sdltest.OnMain(t, func() { sdl.GetRenderLogicalPresentation(r.renderer, &lw, &lh, &mode) })
	if lw != frameW || lh != frameH || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("logical presentation = %dx%d mode %v, want %dx%d letterbox", lw, lh, mode, frameW, frameH)
	}
	if r.app.texture == nil {
		t.Fatal("no texture after the first frame")
	}
	var tw, th float32
	sdltest.OnMain(t, func() { sdl.GetTextureSize(r.app.texture, &tw, &th) })
	if tw != frameW || th != frameH {
		t.Errorf("texture is %vx%v, want %dx%d", tw, th, frameW, frameH)
	}
	assertQuadrants(t, img, red, green, blue, yellow)
	if len(r.released) != 1 || r.released[0] != f {
		t.Errorf("released = %v, want exactly the acquired frame", r.released)
	}
}

func TestNewFrameReplacesPreviousAndTextureIsReused(t *testing.T) {
	r := newRig(t)
	r.frames = []*sdl.Surface{
		quadrants(t, red, green, blue, yellow),
		quadrants(t, yellow, blue, green, red),
	}
	r.step(t)
	first := r.app.texture
	// A user resizing the window afterwards must not be undone by later frames.
	sdltest.ResizeWindow(t, r.window, 32, 24)
	img := r.step(t)
	if r.app.texture != first {
		t.Error("texture was recreated for the second frame")
	}
	if w, h := r.windowSize(t); w != 32 || h != 24 {
		t.Errorf("window = %dx%d, want the user's 32x24 untouched", w, h)
	}
	if len(r.released) != 2 {
		t.Errorf("released %d frames, want 2", len(r.released))
	}
	// 32x24 output with an 8x6 logical size scales by 4 and fills the window.
	sdltest.AssertPixel(t, img, 1, 1, yellow)
	sdltest.AssertPixel(t, img, 30, 1, blue)
	sdltest.AssertPixel(t, img, 1, 22, green)
	sdltest.AssertPixel(t, img, 30, 22, red)
}

func TestLastFrameStaysWhenCameraHasNothingNew(t *testing.T) {
	r := newRig(t)
	r.frames = []*sdl.Surface{quadrants(t, red, green, blue, yellow)} // later acquires return nil
	r.step(t)
	img := r.step(t)
	assertQuadrants(t, img, red, green, blue, yellow)
	if r.acquired != 2 || len(r.released) != 1 {
		t.Errorf("acquired %d, released %d; want 2 and 1 (nil frames are not released)", r.acquired, len(r.released))
	}
}

func TestLetterboxKeepsAspectRatio(t *testing.T) {
	r := newRig(t)
	r.frames = []*sdl.Surface{quadrants(t, red, green, blue, yellow)}
	r.step(t)
	// Twice as wide as the 8x6 frame needs: the picture is scaled by 4 to
	// 32x24 and centred, leaving 16 pixel bars on each side.
	sdltest.ResizeWindow(t, r.window, 64, 24)
	img := r.step(t)
	var rect sdl.FRect
	sdltest.OnMain(t, func() { sdl.GetRenderLogicalPresentationRect(r.renderer, &rect) })
	if rect.X != 16 || rect.Y != 0 || rect.W != 32 || rect.H != 24 {
		t.Errorf("presentation rect = %+v, want x=16 y=0 w=32 h=24", rect)
	}
	// RenderReadPixels returns the logical viewport only, so the bars are not
	// observable; the picture must fill that viewport undistorted (interiors only: the
	// default linear scaling blends colours across quadrant edges).
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 24 {
		t.Fatalf("output is %v, want 32x24", b)
	}
	sdltest.AssertRegion(t, img, image.Rect(2, 2, 12, 8), red)
	sdltest.AssertRegion(t, img, image.Rect(20, 2, 30, 8), green)
	sdltest.AssertRegion(t, img, image.Rect(2, 16, 12, 22), blue)
	sdltest.AssertRegion(t, img, image.Rect(20, 16, 30, 22), yellow)
}

func TestDrawLeavesPresentationToTheCaller(t *testing.T) {
	// iterate must only draw; run presents. Drawing twice into the same target
	// without a present in between must be idempotent.
	r := newRig(t)
	r.frames = []*sdl.Surface{quadrants(t, red, green, blue, yellow)}
	r.step(t)
	img := r.step(t)
	assertQuadrants(t, img, red, green, blue, yellow)
}

// feed pumps SDL and applies every queued event to the app like run does.
func feed(t *testing.T, a *app) (quit bool, err error) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) && !quit && err == nil {
			quit, err = a.handleEvent(&e)
		}
	})
	return quit, err
}

func pushCamera(t *testing.T, typ sdl.EventType, id sdl.CameraID) {
	t.Helper()
	var e sdl.Event
	e.SetType(typ)
	e.Cdevice().Which = id
	if !sdl.PushEvent(&e) {
		t.Fatalf("PushEvent: %s", sdl.GetError())
	}
}

func TestQuitAndEscapeEndTheProgram(t *testing.T) {
	r := newRig(t)
	in := sdltest.NewInput(t, sdltest.WindowID(t, r.window))
	in.Quit()
	if quit, err := feed(t, r.app); !quit || err != nil {
		t.Errorf("QUIT: quit=%v err=%v, want quit", quit, err)
	}
	in.Press(sdl.SCANCODE_ESCAPE)
	if quit, err := feed(t, r.app); !quit || err != nil {
		t.Errorf("Escape: quit=%v err=%v, want quit", quit, err)
	}
}

func TestOtherInputDoesNotQuit(t *testing.T) {
	r := newRig(t)
	in := sdltest.NewInput(t, sdltest.WindowID(t, r.window))
	in.Press(sdl.SCANCODE_Q)
	in.Press(sdl.SCANCODE_SPACE)
	in.MouseButton(1, true, 3, 3)
	in.Resized(100, 100)
	pushCamera(t, sdl.EVENT_CAMERA_DEVICE_ADDED, 5)
	pushCamera(t, sdl.EVENT_CAMERA_DEVICE_REMOVED, 5)
	if quit, err := feed(t, r.app); quit || err != nil {
		t.Errorf("quit=%v err=%v, want to carry on", quit, err)
	}
}

func TestCameraApprovalContinuesAndDenialFails(t *testing.T) {
	r := newRig(t)
	pushCamera(t, sdl.EVENT_CAMERA_DEVICE_APPROVED, 1)
	if quit, err := feed(t, r.app); quit || err != nil {
		t.Errorf("approved: quit=%v err=%v, want to carry on", quit, err)
	}
	pushCamera(t, sdl.EVENT_CAMERA_DEVICE_DENIED, 1)
	quit, err := feed(t, r.app)
	if err == nil || quit {
		t.Errorf("denied: quit=%v err=%v, want a failure", quit, err)
	}
}

func TestCameraEventRoundTrip(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_CAMERA)
	for _, typ := range []sdl.EventType{
		sdl.EVENT_CAMERA_DEVICE_ADDED, sdl.EVENT_CAMERA_DEVICE_REMOVED,
		sdl.EVENT_CAMERA_DEVICE_APPROVED, sdl.EVENT_CAMERA_DEVICE_DENIED,
	} {
		pushCamera(t, typ, 0x1234)
		evs := sdltest.Drain(t)
		var got []sdl.Event
		for _, e := range evs {
			if e.Type() == typ {
				got = append(got, e)
			}
		}
		if len(got) != 1 || got[0].Cdevice().Which != 0x1234 || got[0].Cdevice().Type != typ {
			t.Errorf("type %#x: got %d matching events, device id round trip failed", uint32(typ), len(got))
		}
	}
}

func TestCloseReleasesTexture(t *testing.T) {
	r := newRig(t)
	r.frames = []*sdl.Surface{quadrants(t, red, green, blue, yellow)}
	r.step(t)
	if r.app.texture == nil {
		t.Fatal("precondition: texture expected")
	}
	sdltest.OnMain(t, r.app.close)
	if r.app.texture != nil {
		t.Error("close left the texture in place")
	}
}

// TestRunWithoutCamera drives the real run loop. The dummy camera driver
// delivers no frames, so this only checks that startup, the frame loop and
// shutdown complete cleanly.
func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Errorf("run(3) = %v, want nil", err)
	}
}
