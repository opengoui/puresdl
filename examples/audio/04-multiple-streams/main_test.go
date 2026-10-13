package main

import (
	"image/color"
	"os"
	"testing"
	"unsafe"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// newTestApp opens the app on the dummy audio device and pauses the device
// before anything is queued, so queue sizes stay deterministic.
func newTestApp(t *testing.T) *app {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	var a *app
	var err error
	sdltest.OnMain(t, func() {
		a, err = newApp()
		if err == nil {
			sdl.PauseAudioDevice(a.device)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a
}

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
	return quit
}

func queued(t *testing.T, s *sdl.AudioStream) int { return sdltest.Queued(t, s) }

func TestEmbeddedWAVsDecode(t *testing.T) {
	sdltest.Skip(t)
	for name, data := range map[string][]byte{"sample": sampleWAV, "sword": swordWAV} {
		var n int
		var err error
		sdltest.OnMain(t, func() {
			var b []byte
			_, b, err = sdl.LoadWAVBytes(data)
			n = len(b)
		})
		if err != nil || n == 0 {
			t.Errorf("%s: decoded %d bytes, err %v", name, n, err)
		}
	}
}

func TestInitBindsTwoStreamsToOneDevice(t *testing.T) {
	a := newTestApp(t)
	if !sdl.IsAudioDevicePlayback(a.device) {
		t.Error("device is not a playback device")
	}
	if a.sounds[0].stream == a.sounds[1].stream {
		t.Fatal("sounds share one stream")
	}
	for i, s := range a.sounds {
		if s.stream == nil || len(s.wav) == 0 {
			t.Fatalf("sound %d not loaded", i)
		}
		if got := sdltest.Call(t, func() sdl.AudioDeviceID { return sdl.GetAudioStreamDevice(s.stream) }); got != a.device {
			t.Errorf("sound %d bound to device %d, want %d", i, got, a.device)
		}
		if q := queued(t, s.stream); q != 0 {
			t.Errorf("sound %d has %d bytes queued before the first frame", i, q)
		}
	}
}

func TestIterateQueuesOneCopyThenStops(t *testing.T) {
	a := newTestApp(t)
	sdltest.OnMain(t, a.iterate)
	for i, s := range a.sounds {
		if got := queued(t, s.stream); got != len(s.wav) {
			t.Errorf("sound %d queued %d after first frame, want %d", i, got, len(s.wav))
		}
	}
	// A full copy is already queued, so further frames add nothing.
	sdltest.OnMain(t, a.iterate)
	sdltest.OnMain(t, a.iterate)
	for i, s := range a.sounds {
		if got := queued(t, s.stream); got != len(s.wav) {
			t.Errorf("sound %d queued %d after repeated frames, want %d", i, got, len(s.wav))
		}
	}
}

func TestIterateRefillsWhenLessThanFullCopyRemains(t *testing.T) {
	a := newTestApp(t)
	sdltest.OnMain(t, a.iterate)
	s := a.sounds[0]
	// Consume part of the queue the way a device would.
	const taken = 4096
	var got int32
	sdltest.OnMain(t, func() {
		sdl.FlushAudioStream(s.stream)
		buf := make([]byte, taken)
		got = sdl.GetAudioStreamData(s.stream, unsafe.Pointer(&buf[0]), taken)
	})
	if got <= 0 {
		t.Skipf("could not consume from device stream (device-bound streams reject reads): %d", got)
	}
	before := queued(t, s.stream)
	if before >= len(s.wav) {
		t.Fatalf("queue %d not below one copy %d after consuming", before, len(s.wav))
	}
	sdltest.OnMain(t, a.iterate)
	if after := queued(t, s.stream); after != before+len(s.wav) {
		t.Errorf("queued %d after refill, want %d", after, before+len(s.wav))
	}
	// The untouched sound did not change.
	if q := queued(t, a.sounds[1].stream); q != len(a.sounds[1].wav) {
		t.Errorf("other sound queued %d, want %d", q, len(a.sounds[1].wav))
	}
}

func TestQuitAndEscapeEndTheProgram(t *testing.T) {
	a := newTestApp(t)
	in := sdltest.NewInput(t, 0)

	in.Press(sdl.SCANCODE_A)
	in.Text("x")
	in.MouseButton(1, true, 1, 1)
	if feed(t, a) {
		t.Error("unrelated input requested quit")
	}
	in.Press(sdl.SCANCODE_ESCAPE)
	if !feed(t, a) {
		t.Error("Escape did not quit")
	}
	in.Quit()
	if !feed(t, a) {
		t.Error("SDL_EVENT_QUIT did not quit")
	}
}

func TestEscapeKeyUpDoesNotQuit(t *testing.T) {
	a := newTestApp(t)
	in := sdltest.NewInput(t, 0)
	in.Key(false, sdl.SCANCODE_ESCAPE, sdl.SDLK_ESCAPE, 0)
	if feed(t, a) {
		t.Error("Escape key release quit the program")
	}
}

func TestDrawBlanksScreenToOpaqueBlack(t *testing.T) {
	sdltest.Skip(t)
	a := &app{}
	c := sdltest.NewCanvas(t, 64, 48)
	// Dirty the canvas first so a missing clear is visible.
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 200, 10, 10, 255)
		sdl.RenderClear(c.Renderer)
		a.draw(c.Renderer)
	})
	img := c.Capture(t)
	sdltest.AssertRegion(t, img, img.Bounds(), color.RGBA{0, 0, 0, 255})
}

func TestCloseReleasesAndIsIdempotent(t *testing.T) {
	a := newTestApp(t)
	sdltest.OnMain(t, a.iterate)
	sdltest.OnMain(t, a.close)
	if a.device != 0 || a.sounds[0].stream != nil || a.sounds[1].stream != nil {
		t.Error("close left handles behind")
	}
	// A second close (cleanup) and iterate on a closed app must be safe.
	sdltest.OnMain(t, func() { a.iterate(); a.close() })
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
