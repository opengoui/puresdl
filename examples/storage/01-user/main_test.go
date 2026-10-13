package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	blue    = color.RGBA{0, 0, 255, 255}
	yellow  = color.RGBA{255, 255, 0, 255}
	cyan    = color.RGBA{0, 255, 255, 255}
	magenta = color.RGBA{255, 0, 255, 255}
	green   = color.RGBA{0, 255, 0, 255}
	red     = color.RGBA{255, 0, 0, 255}
)

type fixture struct {
	g   *saver
	in  *sdltest.Input
	dir string
}

// setup initializes SDL and builds a saver whose storage is a temp directory.
func setup(t *testing.T) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	f := &fixture{dir: t.TempDir(), in: sdltest.NewInput(t, 0)}
	open := func() *sdl.Storage { return sdl.OpenFileStorage(f.dir) }
	f.g = sdltest.Call(t, func() *saver { return newSaver(open) })
	t.Cleanup(func() { sdltest.OnMain(t, f.g.close) })
	return f
}

func (f *fixture) feed(t *testing.T) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if f.g.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func (f *fixture) click(t *testing.T, button uint8) {
	t.Helper()
	f.in.MouseButton(button, true, 10, 10)
	f.in.MouseButton(button, false, 10, 10)
	f.feed(t)
}

func (f *fixture) advance(t *testing.T) { t.Helper(); sdltest.OnMain(t, f.g.advance) }

func (f *fixture) state() int32 { return sdl.GetAtomicInt(&f.g.state) }

// waitState polls until the worker publishes want; it waits on a real thread,
// so a generous deadline is used only to fail instead of hanging.
func (f *fixture) waitState(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.state() != want {
		if time.Now().After(deadline) {
			t.Fatalf("state = %d, want %d", f.state(), want)
		}
		time.Sleep(100 * time.Microsecond)
	}
}

// frame advances once and returns the pixel colour of the drawn frame.
func (f *fixture) frame(t *testing.T) color.RGBA {
	t.Helper()
	f.advance(t)
	c := sdltest.NewCanvas(t, 64, 48)
	sdltest.OnMain(t, func() { f.g.draw(c.Renderer) })
	img := c.Capture(t)
	px := sdltest.PixelAt(img, 0, 0)
	sdltest.AssertRegion(t, img, img.Bounds(), px)
	return px
}

// runOperation clicks and drives the main-thread handshake to completion,
// asserting the colour of each intermediate state.
func (f *fixture) runOperation(t *testing.T, button uint8) color.RGBA {
	t.Helper()
	f.click(t, button)
	f.waitState(t, saveStatePreparingStorage)
	if got := f.frame(t); got != cyan {
		t.Fatalf("preparing frame = %v, want cyan", got)
	}
	f.waitState(t, saveStateFinalCheck)
	return f.frame(t)
}

func TestInitialFrameIsBlue(t *testing.T) {
	f := setup(t)
	if got := f.frame(t); got != blue {
		t.Errorf("initial colour = %v, want blue %v", got, blue)
	}
	if f.g.thread != nil {
		t.Error("worker thread running before any click")
	}
}

func TestLeftClickSavesAndTurnsGreen(t *testing.T) {
	f := setup(t)
	if got := f.runOperation(t, sdl.BUTTON_LEFT); got != green {
		t.Errorf("final colour = %v, want green", got)
	}
	if f.g.thread != nil || f.g.result != 0 {
		t.Errorf("thread=%v result=%d, want reaped thread and result 0", f.g.thread, f.g.result)
	}
	data, err := os.ReadFile(filepath.Join(f.dir, saveFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8 {
		t.Errorf("save file is %d bytes, want 8 (one uint64)", len(data))
	}
}

func TestStateColoursDuringOperation(t *testing.T) {
	f := setup(t)
	f.click(t, sdl.BUTTON_LEFT)
	f.waitState(t, saveStatePreparingStorage)
	// The worker blocks on the semaphore, so the state is stable until advance.
	if got := f.frame(t); got != cyan {
		t.Errorf("preparing colour = %v, want cyan", got)
	}
	if got := f.state(); got != saveStateProcessingStorageFile && got != saveStateFinalCheck {
		t.Errorf("after advance state = %d, want processing-file or later", got)
	}
	f.waitState(t, saveStateFinalCheck)
	if got := f.frame(t); got != green {
		t.Errorf("final colour = %v, want green", got)
	}
}

func TestProcessingColourMapping(t *testing.T) {
	// Pure colour mapping for states that are too short-lived to observe live.
	f := setup(t)
	for _, tc := range []struct {
		state int32
		want  color.RGBA
	}{
		{saveStateUnstarted, blue},
		{saveStateProcessingGameWorld, yellow},
		{saveStatePreparingStorage, cyan},
		{saveStateProcessingStorageFile, magenta},
	} {
		f.g.shown = tc.state
		c := sdltest.NewCanvas(t, 8, 8)
		sdltest.OnMain(t, func() { f.g.draw(c.Renderer) })
		img := c.Capture(t)
		sdltest.AssertRegion(t, img, img.Bounds(), tc.want)
	}
	f.g.shown, f.g.result = saveStateFinalCheck, 0
	c := sdltest.NewCanvas(t, 8, 8)
	sdltest.OnMain(t, func() { f.g.draw(c.Renderer) })
	sdltest.AssertPixel(t, c.Capture(t), 0, 0, green)
	f.g.result = -1
	sdltest.OnMain(t, func() { f.g.draw(c.Renderer) })
	sdltest.AssertPixel(t, c.Capture(t), 0, 0, red)
}

func TestLoadWithoutSaveFailsRed(t *testing.T) {
	f := setup(t)
	if got := f.runOperation(t, sdl.BUTTON_RIGHT); got != red {
		t.Errorf("colour = %v, want red", got)
	}
	if f.g.result != -1 || f.g.thread != nil {
		t.Errorf("result=%d thread=%v, want -1 and reaped", f.g.result, f.g.thread)
	}
}

func TestSaveThenLoadSucceedsForEveryNonLeftButton(t *testing.T) {
	for _, btn := range []uint8{sdl.BUTTON_RIGHT, sdl.BUTTON_MIDDLE, sdl.BUTTON_X1, sdl.BUTTON_X2} {
		f := setup(t)
		if got := f.runOperation(t, sdl.BUTTON_LEFT); got != green {
			t.Fatalf("save colour = %v", got)
		}
		before, _ := os.ReadFile(filepath.Join(f.dir, saveFileName))
		// Reset the displayed result so a stale green cannot satisfy the check.
		f.g.result = -1
		if got := f.runOperation(t, btn); got != green {
			t.Errorf("button %d: load colour = %v, want green", btn, got)
		}
		after, _ := os.ReadFile(filepath.Join(f.dir, saveFileName))
		if string(before) != string(after) {
			t.Errorf("button %d: load modified the save file", btn)
		}
	}
}

func TestLoadRejectsWrongSizedSave(t *testing.T) {
	f := setup(t)
	if err := os.WriteFile(filepath.Join(f.dir, saveFileName), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := f.runOperation(t, sdl.BUTTON_RIGHT); got != red {
		t.Errorf("colour = %v, want red for corrupted save", got)
	}
}

func TestStorageOpenFailureTurnsRed(t *testing.T) {
	for _, btn := range []uint8{sdl.BUTTON_LEFT, sdl.BUTTON_RIGHT} {
		sdltest.Setup(t, sdl.INIT_VIDEO)
		in := sdltest.NewInput(t, 0)
		g := sdltest.Call(t, func() *saver { return newSaver(func() *sdl.Storage { return nil }) })
		f := &fixture{g: g, in: in}
		f.click(t, btn)
		f.waitState(t, saveStateFinalCheck)
		if got := f.frame(t); got != red {
			t.Errorf("button %d: colour = %v, want red", btn, got)
		}
		if g.thread != nil {
			t.Errorf("button %d: thread not reaped", btn)
		}
		sdltest.OnMain(t, g.close)
	}
}

func TestClickDuringOperationIsIgnored(t *testing.T) {
	f := setup(t)
	f.click(t, sdl.BUTTON_LEFT)
	f.waitState(t, saveStatePreparingStorage)
	th := f.g.thread
	f.click(t, sdl.BUTTON_RIGHT)
	if f.g.thread != th {
		t.Error("second click replaced the running worker")
	}
	if got := f.state(); got != saveStatePreparingStorage {
		t.Errorf("ignored click changed state to %d", got)
	}
	f.waitState(t, saveStatePreparingStorage)
	f.advance(t)
	f.waitState(t, saveStateFinalCheck)
	if got := f.frame(t); got != green {
		t.Errorf("colour = %v, want green (save completed, load ignored)", got)
	}
}

func TestNewClickAfterFinishRestarts(t *testing.T) {
	f := setup(t)
	f.runOperation(t, sdl.BUTTON_RIGHT) // fails: red
	if got := f.runOperation(t, sdl.BUTTON_LEFT); got != green {
		t.Errorf("save after failed load = %v, want green", got)
	}
}

func TestQuitAndIrrelevantEvents(t *testing.T) {
	f := setup(t)
	f.in.MouseMotion(5, 5, 1, 1, 0)
	f.in.MouseWheel(0, 1)
	f.in.MouseButton(sdl.BUTTON_LEFT, false, 1, 1) // release alone starts nothing
	f.in.Text("x")
	if f.feed(t) {
		t.Error("irrelevant events requested quit")
	}
	if f.g.thread != nil || f.state() != saveStateUnstarted {
		t.Error("irrelevant events started an operation")
	}
	f.in.Quit()
	if !f.feed(t) {
		t.Error("EVENT_QUIT did not request quit")
	}
}

func TestMouseButtonEventRoundTrip(t *testing.T) {
	f := setup(t)
	f.in.MouseButton(sdl.BUTTON_RIGHT, true, 3, 4)
	var typ sdl.EventType
	var btn uint8
	var down bool
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if e.Type() == sdl.EVENT_MOUSE_BUTTON_DOWN {
				typ, btn, down = e.Type(), e.Button().Button, e.Button().Down
			}
		}
	})
	if typ != sdl.EVENT_MOUSE_BUTTON_DOWN || btn != sdl.BUTTON_RIGHT || !down {
		t.Errorf("round trip: type=%v button=%d down=%v", typ, btn, down)
	}
}

func TestCloseJoinsBlockedWorker(t *testing.T) {
	f := setup(t)
	f.click(t, sdl.BUTTON_LEFT)
	f.waitState(t, saveStatePreparingStorage)
	// The worker is blocked on the semaphore; close must release and join it.
	done := make(chan struct{})
	go func() { sdltest.OnMain(t, f.g.close); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not return with a blocked worker")
	}
	if f.g.thread != nil {
		t.Error("thread not cleared by close")
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3, false) })
	if err != nil {
		t.Fatal(err)
	}
}
