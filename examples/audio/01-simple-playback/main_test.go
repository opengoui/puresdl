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

var spec = sdl.AudioSpec{Channels: 1, Format: sdl.AUDIO_F32, Freq: sampleRate}

// newPlayer returns a player writing into an unbound probe stream, so queued
// bytes are deterministic (a resumed dummy device would drain in real time).
func newPlayer(t *testing.T) (*player, *sdltest.AudioProbe) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	probe := sdltest.NewAudioProbe(t, spec)
	return &player{stream: probe.Stream}, probe
}

func feed(t *testing.T, p *player) {
	t.Helper()
	sdltest.OnMain(t, p.feed)
}

func floats(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func wantSample(n int) float32 {
	return float32(math.Sin(2 * math.Pi * float64(n*toneFreq) / sampleRate))
}

func TestFeedFillsToHalfSecondThenStops(t *testing.T) {
	p, probe := newPlayer(t)
	block := len(p.samples) * 4
	// Half a second of mono float32 at 8kHz is 16000 bytes; blocks of 2048
	// bytes are added while the queue is below that, so 8 blocks are queued.
	for i := 1; i <= 7; i++ {
		feed(t, p)
		if got := probe.Queued(); got != i*block {
			t.Fatalf("after frame %d queued = %d, want %d", i, got, i*block)
		}
	}
	feed(t, p)
	want := 8 * block
	if got := probe.Queued(); got != want || want < minimumAudio {
		t.Fatalf("after frame 8 queued = %d, want %d (>= %d)", got, want, minimumAudio)
	}
	for range 5 {
		feed(t, p)
	}
	if got := probe.Queued(); got != want {
		t.Fatalf("queue grew past the threshold: %d, want %d", got, want)
	}
}

func TestFeedResumesWhenQueueDrains(t *testing.T) {
	p, probe := newPlayer(t)
	for range 8 {
		feed(t, p)
	}
	first := floats(probe.ReadAll())
	if len(first) != 8*512 {
		t.Fatalf("drained %d samples, want %d", len(first), 8*512)
	}
	feed(t, p)
	next := floats(probe.ReadAll())
	if len(next) != 512 {
		t.Fatalf("refill produced %d samples, want 512", len(next))
	}
	// The tone continues where it left off.
	for i, v := range next {
		if d := math.Abs(float64(v - wantSample(8*512+i))); d > 1e-3 {
			t.Fatalf("sample %d = %v, want %v", 8*512+i, v, wantSample(8*512+i))
		}
	}
}

func TestToneIs440HzMonoFloat32(t *testing.T) {
	p, probe := newPlayer(t)
	feed(t, p)
	got := floats(probe.ReadAll())
	if len(got) != 512 {
		t.Fatalf("got %d samples, want 512", len(got))
	}
	for i, v := range got {
		if d := math.Abs(float64(v - wantSample(i))); d > 1e-3 {
			t.Fatalf("sample %d = %v, want %v", i, v, wantSample(i))
		}
	}
	// 8000/440 samples per period: the sine starts at 0 and peaks near a quarter period.
	if got[0] != 0 {
		t.Errorf("first sample = %v, want 0", got[0])
	}
	if peak := got[4]; peak < 0.95 {
		t.Errorf("sample near quarter period = %v, want close to 1", peak)
	}
}

func TestSineIndexWrapsAtOneSecond(t *testing.T) {
	p, probe := newPlayer(t)
	p.currentSineSample = sampleRate - 100
	feed(t, p)
	if want := (sampleRate - 100 + 512) % sampleRate; p.currentSineSample != want {
		t.Fatalf("currentSineSample = %d, want %d", p.currentSineSample, want)
	}
	got := floats(probe.ReadAll())
	// The wrap is seamless because 440 whole cycles fit in one second.
	for i, v := range got {
		if d := math.Abs(float64(v - wantSample(sampleRate-100+i))); d > 1e-3 {
			t.Fatalf("sample %d across wrap = %v, want %v", i, v, wantSample(sampleRate-100+i))
		}
	}
}

func TestStreamReceivesBytesAsPut(t *testing.T) {
	// Round trip of the PutAudioStreamSlice binding: bytes in equal bytes out.
	sdltest.Setup(t, sdl.INIT_AUDIO)
	probe := sdltest.NewAudioProbe(t, spec)
	in := []float32{0, 0.5, -0.5, 1}
	sdltest.OnMain(t, func() { sdl.PutAudioStreamSlice(probe.Stream, in) })
	out := floats(probe.ReadAll())
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("sample %d = %v, want %v", i, out[i], in[i])
		}
	}
}

func TestOnlyQuitEventEndsProgram(t *testing.T) {
	p, _ := newPlayer(t)
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseMotion(1, 2, 0, 0, 0)
	in.Resized(100, 100)
	in.CloseRequested()
	var quit bool
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if p.handleEvent(&e) {
				quit = true
			}
		}
	})
	if quit {
		t.Fatal("non-quit events must not end the program")
	}
	in.Quit()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if p.handleEvent(&e) {
				quit = true
			}
		}
	})
	if !quit {
		t.Fatal("EVENT_QUIT must end the program")
	}
}

func TestDrawClearsToBlack(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 64, 48)
	p := &player{}
	// Dirty the canvas, then restore the default draw colour (black).
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 255, 255, 255, 255)
		sdl.RenderClear(c.Renderer)
		sdl.SetRenderDrawColor(c.Renderer, 0, 0, 0, 255)
		p.draw(c.Renderer)
	})
	img := c.Capture(t)
	sdltest.AssertRegion(t, img, img.Bounds(), color.RGBA{0, 0, 0, 255})
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
