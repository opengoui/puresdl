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
	blue  = color.RGBA{0, 0, 255, 255}
	green = color.RGBA{0, 255, 0, 255}
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}

	stereoU8 = sdl.AudioSpec{Format: sdl.AUDIO_U8, Channels: 2, Freq: 4000}
)

// fixture is the app on a real (hidden) window with the same logical
// presentation as run() and a paused device stream.
type fixture struct {
	a   *app
	in  *sdltest.Input
	win *sdl.Window
}

func newFixture(t *testing.T, stream *sdl.AudioStream) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	win := sdltest.NewWindow(t, 640, 480)
	r := sdltest.Call(t, func() *sdl.Renderer {
		r := sdl.CreateRenderer(win, "software")
		if r != nil {
			sdl.SetRenderLogicalPresentation(r, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
		}
		return r
	})
	if r == nil {
		t.Fatalf("CreateRenderer: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	if stream == nil {
		stream = sdltest.OpenPlayback(t, stereoU8)
	}
	return &fixture{
		a:   &app{renderer: r, stream: stream},
		in:  sdltest.NewInput(t, sdltest.WindowID(t, win)),
		win: win,
	}
}

// feed applies every pending event to the app like the run loop does.
func (f *fixture) feed(t *testing.T) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if f.a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func (f *fixture) click(t *testing.T, x, y float32) {
	t.Helper()
	f.in.MouseButton(sdl.BUTTON_LEFT, true, x, y)
	f.feed(t)
}

func (f *fixture) frame(t *testing.T) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, func() { f.a.advance(); f.a.draw() })
	return sdltest.Capture(t, f.a.renderer)
}

func (f *fixture) queued(t *testing.T) int { return sdltest.Queued(t, f.a.stream) }

// Sample points inside the buttons, away from the label text.
const (
	leftX, leftY   = 105, 175
	rightX, rightY = 445, 175
)

func TestInitialFrame(t *testing.T) {
	f := newFixture(t, nil)
	img := f.frame(t)
	sdltest.AssertPixel(t, img, leftX, leftY, blue)
	sdltest.AssertPixel(t, img, rightX, rightY, blue)
	sdltest.AssertRegion(t, img, image.Rect(100, 170, 104, 174), blue)
	sdltest.AssertPixel(t, img, 320, 240, black)
	sdltest.AssertPixel(t, img, 5, 5, black)
	sdltest.AssertPixel(t, img, 99, 220, black)
	sdltest.AssertPixel(t, img, 200, 220, black)
	if got := countColor(img, image.Rect(100, 170, 200, 270), white); got == 0 {
		t.Error("LEFT label not drawn")
	}
	if got := countColor(img, image.Rect(440, 170, 540, 270), white); got == 0 {
		t.Error("RIGHT label not drawn")
	}
	if f.queued(t) != 0 {
		t.Error("audio queued before any click")
	}
}

func countColor(img *image.RGBA, r image.Rectangle, c color.RGBA) (n int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) == c {
				n++
			}
		}
	}
	return n
}

func TestClickLeftPlaysLeftOnly(t *testing.T) {
	probe := sdltest.NewAudioProbe(t, stereoU8)
	f := newFixture(t, probe.Stream)
	f.click(t, 150, 220)
	if f.a.playingSound != -1 {
		t.Fatalf("playingSound = %d, want -1", f.a.playingSound)
	}
	img := f.frame(t)
	sdltest.AssertPixel(t, img, leftX, leftY, green)
	sdltest.AssertPixel(t, img, rightX, rightY, blue)

	got := probe.ReadAll()
	if len(got) != 2*len(left) {
		t.Fatalf("queued %d bytes, want %d (stereo)", len(got), 2*len(left))
	}
	const silenceU8 = 0x80
	for i, s := range left {
		if got[2*i] != s || got[2*i+1] != silenceU8 {
			t.Fatalf("frame %d = (%#x,%#x), want (%#x,%#x)", i, got[2*i], got[2*i+1], s, silenceU8)
		}
	}
}

func TestClickRightPlaysRightOnly(t *testing.T) {
	probe := sdltest.NewAudioProbe(t, stereoU8)
	f := newFixture(t, probe.Stream)
	f.click(t, 490, 220)
	if f.a.playingSound != 1 {
		t.Fatalf("playingSound = %d, want 1", f.a.playingSound)
	}
	img := f.frame(t)
	sdltest.AssertPixel(t, img, leftX, leftY, blue)
	sdltest.AssertPixel(t, img, rightX, rightY, green)

	got := probe.ReadAll()
	if len(got) != 2*len(right) {
		t.Fatalf("queued %d bytes, want %d (stereo)", len(got), 2*len(right))
	}
	const silenceU8 = 0x80
	for i, s := range right {
		if got[2*i] != silenceU8 || got[2*i+1] != s {
			t.Fatalf("frame %d = (%#x,%#x), want (%#x,%#x)", i, got[2*i], got[2*i+1], silenceU8, s)
		}
	}
}

func TestClicksIgnoredWhilePlaying(t *testing.T) {
	f := newFixture(t, nil)
	f.click(t, 150, 220)
	want := f.queued(t)
	if want != 2*len(left) {
		t.Fatalf("queued = %d, want %d", want, 2*len(left))
	}
	f.click(t, 490, 220) // other button
	f.click(t, 150, 220) // same button
	if f.a.playingSound != -1 || f.queued(t) != want {
		t.Errorf("playing=%d queued=%d, want -1 and %d", f.a.playingSound, f.queued(t), want)
	}
	// A frame with audio still queued must not release the buttons.
	img := f.frame(t)
	sdltest.AssertPixel(t, img, leftX, leftY, green)
	sdltest.AssertPixel(t, img, rightX, rightY, blue)
}

func TestFinishedSoundAllowsNewSound(t *testing.T) {
	f := newFixture(t, nil)
	f.click(t, 150, 220)
	sdltest.OnMain(t, func() { sdl.ClearAudioStream(f.a.stream) }) // device consumed everything
	img := f.frame(t)
	if f.a.playingSound != 0 {
		t.Fatalf("playingSound = %d after drain, want 0", f.a.playingSound)
	}
	sdltest.AssertPixel(t, img, leftX, leftY, blue)
	sdltest.AssertPixel(t, img, rightX, rightY, blue)

	f.click(t, 490, 220)
	if f.a.playingSound != 1 || f.queued(t) != 2*len(right) {
		t.Errorf("playing=%d queued=%d, want 1 and %d", f.a.playingSound, f.queued(t), 2*len(right))
	}
	img = f.frame(t)
	sdltest.AssertPixel(t, img, leftX, leftY, blue)
	sdltest.AssertPixel(t, img, rightX, rightY, green)
}

func TestClickHitBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float32
		want int
	}{
		{"left top-left corner is inside", 100, 170, -1},
		{"left right edge is inclusive", 200, 220, -1},
		{"left bottom edge is inclusive", 150, 270, -1},
		{"just past left right edge", 200.5, 220, 0},
		{"just past left bottom edge", 150, 270.5, 0},
		{"left just inside bottom-right", 199.5, 269.5, -1},
		{"left just outside left", 99.5, 220, 0},
		{"gap between buttons", 320, 220, 0},
		{"right top-left corner is inside", 440, 170, 1},
		{"right far edge is inclusive", 540, 220, 1},
		{"just past right far edge", 540.5, 220, 0},
		{"above buttons", 150, 169.5, 0},
		{"origin", 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			f.click(t, tc.x, tc.y)
			if f.a.playingSound != tc.want {
				t.Errorf("playingSound = %d, want %d", f.a.playingSound, tc.want)
			}
			if tc.want == 0 && f.queued(t) != 0 {
				t.Errorf("queued %d bytes for a miss", f.queued(t))
			}
		})
	}
}

func TestOnlyButtonDownStartsSound(t *testing.T) {
	f := newFixture(t, nil)
	f.in.MouseMotion(150, 220, 0, 0, 0)
	f.in.MouseButton(sdl.BUTTON_LEFT, false, 150, 220)
	f.in.MouseWheel(0, 1)
	f.feed(t)
	if f.a.playingSound != 0 || f.queued(t) != 0 {
		t.Errorf("non-press events started a sound: playing=%d queued=%d", f.a.playingSound, f.queued(t))
	}
}

func TestAnyMouseButtonPlays(t *testing.T) {
	f := newFixture(t, nil)
	f.in.MouseButton(sdl.BUTTON_RIGHT, true, 150, 220)
	f.feed(t)
	if f.a.playingSound != -1 {
		t.Errorf("right mouse button: playingSound = %d, want -1 (the C code ignores which button)", f.a.playingSound)
	}
}

func TestQuitEvents(t *testing.T) {
	f := newFixture(t, nil)
	f.in.Press(sdl.SCANCODE_ESCAPE)
	f.in.Press(sdl.SCANCODE_Q)
	f.in.Press(sdl.SCANCODE_SPACE)
	f.in.CloseRequested()
	if f.feed(t) {
		t.Error("keys or a window close request quit; only EVENT_QUIT does in the C program")
	}
	f.in.Quit()
	if !f.feed(t) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestLetterboxMapsWindowToLogicalCoordinates(t *testing.T) {
	f := newFixture(t, nil)
	// 1280x480: logical 640x480 is shown at scale 1 with a 320px bar each side.
	sdltest.ResizeWindow(t, f.win, 1280, 480)
	f.feed(t)
	// Window x 320+150 is logical 150 (left button); window x 150 is in the bar.
	f.click(t, 150, 220)
	if f.a.playingSound != 0 {
		t.Fatalf("click in letterbox bar hit a button (playing=%d)", f.a.playingSound)
	}
	f.click(t, 320+150, 220)
	if f.a.playingSound != -1 {
		t.Fatalf("click on letterboxed left button: playing=%d, want -1", f.a.playingSound)
	}
	img := f.frame(t)
	// RenderReadPixels covers the logical viewport, bars excluded, so the
	// captured image is addressed in logical coordinates.
	if got := img.Bounds().Size(); got != (image.Point{640, 480}) {
		t.Fatalf("capture size = %v, want the 640x480 logical area", got)
	}
	sdltest.AssertPixel(t, img, leftX, leftY, green)
	sdltest.AssertPixel(t, img, rightX, rightY, blue)
}

func TestLetterboxScalesUp(t *testing.T) {
	f := newFixture(t, nil)
	sdltest.ResizeWindow(t, f.win, 1280, 960) // exactly 2x
	f.feed(t)
	f.click(t, 2*450, 2*220)
	if f.a.playingSound != 1 {
		t.Errorf("click on 2x right button: playing=%d, want 1", f.a.playingSound)
	}
	img := f.frame(t)
	sdltest.AssertPixel(t, img, 2*rightX, 2*rightY, green)
	sdltest.AssertPixel(t, img, 2*leftX, 2*leftY, blue)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
