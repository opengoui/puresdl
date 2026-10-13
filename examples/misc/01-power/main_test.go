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
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// render draws one frame for info on a 640x480 canvas and returns the pixels.
func render(t *testing.T, info powerInfo) *image.RGBA {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	a := newApp(func() powerInfo { return info })
	sdltest.OnMain(t, func() { a.iterate(c.Renderer) })
	return c.Capture(t)
}

// count returns how many pixels in r equal want.
func count(img *image.RGBA, r image.Rectangle, want color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) == want {
				n++
			}
		}
	}
	return n
}

var (
	msgRow   = image.Rect(0, 184, 640, 192) // y = frame.Y - 2*8
	msg2Row  = image.Rect(0, 168, 640, 176) // y = frame.Y - 4*8
	belowRow = image.Rect(0, 288, 640, 296) // y = frame.Y + frame.H + 8
	barRect  = image.Rect(102, 202, 538, 278)
)

func TestPowerStatePalettes(t *testing.T) {
	tests := []struct {
		name         string
		state        sdl.PowerState
		clear, bar   color.RGBA
		wantMsgGlyph bool
	}{
		{"unknown", sdl.POWERSTATE_UNKNOWN, rgb(50, 50, 50), black, true},
		{"unexpected value treated as unknown", sdl.PowerState(99), rgb(50, 50, 50), black, true},
		{"on battery", sdl.POWERSTATE_ON_BATTERY, black, rgb(255, 0, 0), true},
		{"no battery", sdl.POWERSTATE_NO_BATTERY, rgb(0, 50, 0), black, true},
		{"charging", sdl.POWERSTATE_CHARGING, black, rgb(0, 255, 255), true},
		{"charged", sdl.POWERSTATE_CHARGED, black, rgb(0, 255, 0), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			img := render(t, powerInfo{state: tc.state, seconds: -1, percent: 100})
			// 100 percent: the whole bar interior is filled with the bar colour.
			sdltest.AssertRegion(t, img, barRect, tc.bar)
			sdltest.AssertPixel(t, img, 20, 20, tc.clear)
			sdltest.AssertPixel(t, img, 100, 240, white) // frame outline
			if count(img, msgRow, white) == 0 {
				t.Error("status message not drawn above the bar")
			}
			if count(img, msg2Row, white) != 0 {
				t.Error("secondary message drawn for a non-error state")
			}
		})
	}
}

func TestPercentBarWidth(t *testing.T) {
	tests := []struct {
		percent int32
		fillTo  int // exclusive x of the bar fill
	}{{0, 100}, {25, 210}, {50, 320}, {100, 540}}
	for _, tc := range tests {
		img := render(t, powerInfo{state: sdl.POWERSTATE_ON_BATTERY, seconds: 60, percent: tc.percent})
		red := rgb(255, 0, 0)
		if tc.fillTo > 103 {
			sdltest.AssertRegion(t, img, image.Rect(102, 202, tc.fillTo-2, 278), red)
		}
		if tc.fillTo < 536 {
			sdltest.AssertRegion(t, img, image.Rect(tc.fillTo+2, 202, 538, 278), black)
		}
		// The frame is drawn on top of the bar, regardless of fill.
		sdltest.AssertPixel(t, img, 100, 240, white)
		sdltest.AssertPixel(t, img, 300, 200, white)
		sdltest.AssertPixel(t, img, 300, 279, white)
	}
}

func TestNegativePercentDrawsNoBar(t *testing.T) {
	img := render(t, powerInfo{state: sdl.POWERSTATE_NO_BATTERY, seconds: -1, percent: -1})
	green := rgb(0, 50, 0)
	sdltest.AssertRegion(t, img, image.Rect(90, 195, 550, 290), green) // no frame, no bar, no text below
	if count(img, belowRow, white) != 0 {
		t.Error("battery text drawn although percent < 0")
	}
	if count(img, msgRow, white) == 0 {
		t.Error("status message must still be drawn")
	}
}

func TestBatteryTextVisibleWhenPercentKnown(t *testing.T) {
	img := render(t, powerInfo{state: sdl.POWERSTATE_CHARGING, seconds: 3661, percent: 42})
	if count(img, belowRow, white) == 0 {
		t.Fatal("battery text missing below the bar")
	}
	// "Battery: ..." strings are centred on the bar frame: ink must straddle x=320.
	left := count(img, image.Rect(100, 288, 320, 296), white)
	right := count(img, image.Rect(320, 288, 540, 296), white)
	if left == 0 || right == 0 {
		t.Errorf("battery text not centred: left ink %d, right ink %d", left, right)
	}
}

func TestBatteryTextLayoutByLength(t *testing.T) {
	// The text is centred, so a longer remaining-time string widens the ink
	// span symmetrically. "unknown time" (12 chars) vs "00:01:00" (8 chars).
	span := func(info powerInfo) (min, max int) {
		img := render(t, info)
		min, max = 1<<30, -1
		for y := 288; y < 296; y++ {
			for x := 0; x < 640; x++ {
				if img.RGBAAt(x, y) == white {
					if x < min {
						min = x
					}
					if x > max {
						max = x
					}
				}
			}
		}
		return
	}
	// len("Battery:  50 percent, unknown time remaining") = 44 chars -> x0 = 100+(440-352)/2 = 144
	// len("Battery:  50 percent, 00:01:00 remaining")     = 40 chars -> x0 = 100+(440-320)/2 = 160
	umin, umax := span(powerInfo{state: sdl.POWERSTATE_ON_BATTERY, seconds: -1, percent: 50})
	kmin, kmax := span(powerInfo{state: sdl.POWERSTATE_ON_BATTERY, seconds: 60, percent: 50})
	if umin < 144 || umax >= 144+44*8 || kmin < 160 || kmax >= 160+40*8 {
		t.Errorf("text outside centred bounds: unknown [%d,%d] known [%d,%d]", umin, umax, kmin, kmax)
	}
	if !(umin < kmin) || !(umax > kmax) {
		t.Errorf("longer text must extend further both sides: unknown [%d,%d] known [%d,%d]", umin, umax, kmin, kmax)
	}
}

func TestErrorStateDrawsBothMessagesOnRed(t *testing.T) {
	img := render(t, powerInfo{state: sdl.POWERSTATE_ERROR, seconds: -1, percent: -1, err: "boom"})
	sdltest.AssertPixel(t, img, 20, 20, rgb(255, 0, 0))
	if count(img, msg2Row, white) == 0 {
		t.Error("ERROR GETTING POWER STATE header missing")
	}
	if count(img, msgRow, white) == 0 {
		t.Error("error message missing")
	}
	// "ERROR GETTING POWER STATE" is 25 chars, centred: x0 = 100+(440-200)/2 = 220.
	for _, col := range []int{219, 420} {
		if count(img, image.Rect(col, 168, col+1, 176), white) != 0 {
			t.Errorf("header ink outside its centred 220..419 span at column %d", col)
		}
	}
	if count(img, image.Rect(220, 168, 420, 176), white) == 0 {
		t.Error("header ink not inside its centred span")
	}
}

func TestErrorStateWithEmptyMessage(t *testing.T) {
	img := render(t, powerInfo{state: sdl.POWERSTATE_ERROR, seconds: -1, percent: -1})
	if count(img, msgRow, white) != 0 {
		t.Error("empty error string must draw nothing at the message row")
	}
	if count(img, msg2Row, white) == 0 {
		t.Error("header still expected")
	}
}

func TestQueryPowerMatchesSDL(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	info := sdltest.Call(t, queryPower)
	switch info.state {
	case sdl.POWERSTATE_ERROR, sdl.POWERSTATE_UNKNOWN, sdl.POWERSTATE_ON_BATTERY,
		sdl.POWERSTATE_NO_BATTERY, sdl.POWERSTATE_CHARGING, sdl.POWERSTATE_CHARGED:
	default:
		t.Fatalf("unexpected power state %d", info.state)
	}
	if info.state != sdl.POWERSTATE_ERROR && info.err != "" {
		t.Errorf("err set for non-error state: %q", info.err)
	}
	if info.percent > 100 {
		t.Errorf("percent %d out of range", info.percent)
	}
	if info.state == sdl.POWERSTATE_NO_BATTERY && info.percent >= 0 {
		t.Errorf("no-battery state reports percent %d", info.percent)
	}
}

// feed delivers all pending SDL events to the app and reports quit.
func feed(t *testing.T, a *app) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return
}

func TestQuitEventEndsProgram(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(queryPower)
	in := sdltest.NewInput(t, 0)
	in.Quit()
	if !feed(t, a) {
		t.Fatal("EVENT_QUIT must end the program")
	}
}

func TestOtherEventsDoNotQuit(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(queryPower)
	in := sdltest.NewInput(t, 0)
	// Like the C source, only SDL_EVENT_QUIT ends the program: not Escape,
	// not a window close request, not input.
	in.Press(sdl.SCANCODE_ESCAPE)
	in.CloseRequested()
	in.Resized(800, 600)
	in.MouseMotion(5, 5, 1, 1, 0)
	in.MouseButton(1, true, 5, 5)
	in.MouseWheel(0, 1)
	in.Text("q")
	if feed(t, a) {
		t.Fatal("non-quit events must not end the program")
	}
}

func TestQuitAmongOtherEvents(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := newApp(queryPower)
	in := sdltest.NewInput(t, 0)
	in.MouseMotion(1, 1, 0, 0, 0)
	in.Quit()
	in.Press(sdl.SCANCODE_A)
	if !feed(t, a) {
		t.Fatal("quit must be noticed in a mixed batch")
	}
}

func TestLogicalPresentationLetterboxes(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 1280, 480)
	var r *sdl.Renderer
	sdltest.OnMain(t, func() { r = sdl.CreateRenderer(win, "software") })
	if r == nil {
		t.Fatal("CreateRenderer failed")
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	var rect sdl.FRect
	var x0, y0, x1, y1 float32
	sdltest.OnMain(t, func() {
		configure(r)
		sdl.GetRenderLogicalPresentation(r, &w, &h, &mode)
		sdl.GetRenderLogicalPresentationRect(r, &rect)
		sdl.RenderCoordinatesFromWindow(r, 320, 0, &x0, &y0)
		sdl.RenderCoordinatesFromWindow(r, 960, 480, &x1, &y1)
	})
	if w != 640 || h != 480 || mode != sdl.LOGICAL_PRESENTATION_LETTERBOX {
		t.Errorf("logical presentation = %dx%d mode %d", w, h, mode)
	}
	// 640x480 centred in a 1280x480 window: 320px bars each side.
	if rect != (sdl.FRect{X: 320, Y: 0, W: 640, H: 480}) {
		t.Errorf("presentation rect = %+v", rect)
	}
	if x0 != 0 || y0 != 0 || x1 != 640 || y1 != 480 {
		t.Errorf("window->logical mapping: (%v,%v) (%v,%v)", x0, y0, x1, y1)
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
