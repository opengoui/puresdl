package main

import (
	"encoding/binary"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var monoF32 = sdl.AudioSpec{Channels: 1, Format: sdl.AUDIO_F32, Freq: sampleRate}

func decode(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.NativeEndian.Uint32(b[i*4:]))
	}
	return out
}

// feedProbe runs the callback body against an unbound probe stream and
// returns the samples that were queued.
func feedProbe(t *testing.T, s *synth, additionalBytes int) []float32 {
	t.Helper()
	p := sdltest.NewAudioProbe(t, monoF32)
	sdltest.OnMain(t, func() { s.feed(p.Stream, additionalBytes) })
	return decode(p.ReadAll())
}

func wantSample(i int) float32 {
	return float32(math.Sin(2 * math.Pi * toneFreq * float64(i) / sampleRate))
}

func TestFeedProducesA440HzSine(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	var s synth
	// 300 samples crosses the 128-sample chunk boundary twice.
	got := feedProbe(t, &s, 300*4)
	if len(got) != 300 {
		t.Fatalf("got %d samples, want 300", len(got))
	}
	for i, v := range got {
		if d := math.Abs(float64(v - wantSample(i))); d > 1e-4 {
			t.Fatalf("sample %d = %v, want %v", i, v, wantSample(i))
		}
	}
	// One period of 440Hz at 8000Hz is not integral, but sample 0 is 0 and
	// the tone is non-trivial.
	if got[0] != 0 || got[1] <= 0.3 {
		t.Errorf("unexpected tone start: %v %v", got[0], got[1])
	}
	if s.current != 300 {
		t.Errorf("position = %d, want 300", s.current)
	}
}

func TestFeedContinuesAcrossCalls(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	var s synth
	a := feedProbe(t, &s, 100*4)
	b := feedProbe(t, &s, 100*4)
	for i, v := range b {
		if d := math.Abs(float64(v - wantSample(100+i))); d > 1e-4 {
			t.Fatalf("second call sample %d = %v, want %v", i, v, wantSample(100+i))
		}
	}
	if len(a) != 100 || len(b) != 100 {
		t.Fatalf("lengths %d %d", len(a), len(b))
	}
}

func TestFeedWrapsPositionAtOneSecond(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	s := synth{current: sampleRate - 2}
	got := feedProbe(t, &s, 5*4)
	if s.current != 3 {
		t.Errorf("position after wrap = %d, want 3", s.current)
	}
	// Because 8000 is a whole number of periods (440 cycles), the wrap is
	// seamless: samples continue the same sine.
	for i, v := range got {
		if d := math.Abs(float64(v - wantSample(sampleRate-2+i))); d > 1e-3 {
			t.Errorf("sample %d = %v, want %v", i, v, wantSample(sampleRate-2+i))
		}
	}
}

func TestFeedIgnoresPartialSamplesAndZeroRequest(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	var s synth
	if got := feedProbe(t, &s, 0); len(got) != 0 {
		t.Errorf("zero request queued %d samples", len(got))
	}
	if got := feedProbe(t, &s, 3); len(got) != 0 || s.current != 0 {
		t.Errorf("3-byte request queued %d samples, position %d", len(got), s.current)
	}
	if got := feedProbe(t, &s, 7); len(got) != 1 {
		t.Errorf("7-byte request queued %d samples, want 1", len(got))
	}
	if got := feedProbe(t, &s, -8); len(got) != 0 {
		t.Errorf("negative request queued %d samples", len(got))
	}
}

func setupApp(t *testing.T) *app {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	var a *app
	var err error
	sdltest.OnMain(t, func() { a, err = newApp() })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a
}

func TestNewAppStreamIsPausedMonoF32(t *testing.T) {
	a := setupApp(t)
	var paused bool
	var src, dst sdl.AudioSpec
	sdltest.OnMain(t, func() {
		paused = sdl.AudioStreamDevicePaused(a.stream)
		sdl.GetAudioStreamFormat(a.stream, &src, &dst)
	})
	if !paused {
		t.Error("stream device must start paused")
	}
	if src != monoF32 {
		t.Errorf("stream source spec = %+v, want %+v", src, monoF32)
	}
}

func TestResumeStartsDevice(t *testing.T) {
	a := setupApp(t)
	var paused bool
	sdltest.OnMain(t, func() {
		sdl.ResumeAudioStreamDevice(a.stream)
		paused = sdl.AudioStreamDevicePaused(a.stream)
	})
	if paused {
		t.Error("device still paused after resume")
	}
}

func TestQuitEventEndsProgram(t *testing.T) {
	a := setupApp(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(1, 1, 0, 0, 0)
	in.Quit()
	var quits, others int
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quits++
			} else {
				others++
			}
		}
	})
	if quits != 1 || others == 0 {
		t.Errorf("quits=%d others=%d; only EVENT_QUIT should quit", quits, others)
	}
}

func TestDrawBlanksWindowToOpaqueBlack(t *testing.T) {
	a := setupApp(t)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(a.renderer, 200, 10, 10, 255) // dirty the target
		sdl.RenderFillRect(a.renderer, nil)
		sdl.SetRenderDrawColor(a.renderer, 0, 0, 0, 255)
		a.draw()
	})
	img := sdltest.Capture(t, a.renderer)
	b := img.Bounds()
	sdltest.AssertPixel(t, img, b.Min.X, b.Min.Y, color.RGBA{0, 0, 0, 255})
	sdltest.AssertPixel(t, img, b.Max.X-1, b.Max.Y-1, color.RGBA{0, 0, 0, 255})
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
