package main

import (
	"bytes"
	"image/color"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// decode loads the embedded sample, as run does.
func decode(t *testing.T) (sdl.AudioSpec, []byte) {
	t.Helper()
	sdltest.Skip(t)
	spec, wav, err := sdl.LoadWAVBytes(sampleWAV)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) == 0 {
		t.Fatal("decoded sample is empty")
	}
	return spec, wav
}

func newPlayer(t *testing.T) (*player, *sdltest.AudioProbe, []byte) {
	t.Helper()
	spec, wav := decode(t)
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	probe := sdltest.NewAudioProbe(t, spec)
	return &player{stream: probe.Stream, wav: wav}, probe, wav
}

func iterate(t *testing.T, p *player) { t.Helper(); sdltest.OnMain(t, p.iterate) }

// drain pumps SDL and applies every queued event to p.
func drain(t *testing.T, p *player) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if p.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func TestIterateQueuesOneWholeCopy(t *testing.T) {
	p, probe, wav := newPlayer(t)
	iterate(t, p)
	if got := probe.Queued(); got != len(wav) {
		t.Fatalf("queued %d bytes after first frame, want %d", got, len(wav))
	}
	// Plenty queued already: further frames must not add more.
	iterate(t, p)
	iterate(t, p)
	if got := probe.Queued(); got != len(wav) {
		t.Fatalf("queued %d bytes after idle frames, want %d", got, len(wav))
	}
	if got := probe.ReadAll(); !bytes.Equal(got, wav) {
		t.Fatalf("queued audio differs from the decoded sample (%d vs %d bytes)", len(got), len(wav))
	}
}

func TestIterateRefillsWhenLessThanOneCopyRemains(t *testing.T) {
	p, probe, wav := newPlayer(t)
	iterate(t, p)
	// Playback consumes part of the queue.
	consumed := sdltest.Call(t, func() int32 {
		buf := make([]byte, 1000)
		return sdl.GetAudioStreamBytes(probe.Stream, buf)
	})
	if consumed != 1000 {
		t.Fatalf("consumed %d bytes, want 1000", consumed)
	}
	iterate(t, p)
	if got, want := probe.Queued(), 2*len(wav)-1000; got != want {
		t.Fatalf("queued %d bytes after refill, want %d", got, want)
	}
	// Now a whole copy is queued again, so no further refill.
	iterate(t, p)
	if got, want := probe.Queued(), 2*len(wav)-1000; got != want {
		t.Fatalf("queued %d bytes, want unchanged %d", got, want)
	}
}

func TestQuitEvents(t *testing.T) {
	p, _, _ := newPlayer(t)
	in := sdltest.NewInput(t, 0)
	if drain(t, p) {
		t.Fatal("quit with no events")
	}
	in.Press(sdl.SCANCODE_A)
	in.MouseMotion(10, 10, 1, 1, 0)
	in.Text("x")
	if drain(t, p) {
		t.Fatal("unrelated events quit the program")
	}
	in.Press(sdl.SCANCODE_ESCAPE)
	if !drain(t, p) {
		t.Fatal("Escape did not quit")
	}
	in.Quit()
	if !drain(t, p) {
		t.Fatal("EVENT_QUIT did not quit")
	}
}

func TestDrawBlanksWindow(t *testing.T) {
	p, _, _ := newPlayer(t)
	c := sdltest.NewCanvas(t, 64, 48)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 200, 100, 50, 255)
		p.draw(c.Renderer)
	})
	img := c.Capture(t)
	// RenderClear fills with the draw colour.
	sdltest.AssertRegion(t, img, img.Bounds(), color.RGBA{200, 100, 50, 255})
}

func TestEmbeddedSampleIsValidWAV(t *testing.T) {
	spec, wav := decode(t)
	if spec.Freq <= 0 || spec.Channels <= 0 || spec.Format == 0 {
		t.Fatalf("bad spec %+v", spec)
	}
	frame := int(sdl.AUDIO_FRAMESIZE(spec))
	if len(wav)%frame != 0 {
		t.Fatalf("%d bytes is not a whole number of %d-byte frames", len(wav), frame)
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
