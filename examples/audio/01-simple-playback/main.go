// Command 01-simple-playback ports SDL/examples/audio/01-simple-playback
// (simple-playback.c). It creates a simple audio stream and feeds it a
// generated 440Hz sine wave, the simplest way to get procedural sound.
//
// Run it with:
//
//	go run ./examples/audio/01-simple-playback
//
// The -frames flag makes it exit after N frames, which allows running it
// headless:
//
//	SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy go run ./examples/audio/01-simple-playback -frames 30
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to the process main thread. SDL video and
// event functions must run there (mandatory on macOS), and locking in init
// makes main.main start on it.
func init() {
	// SDL video and event functions must run on the process main thread
	// (mandatory on macOS). Locking in init makes main.main run there.
	runtime.LockOSThread()
}

// Audio format and tone parameters for the generated sine wave.
const (
	// sampleRate is the stream's sample rate in Hz.
	sampleRate = 8000
	// toneFreq is the pitch of the generated tone in Hz.
	toneFreq = 440
	// minimumAudio is half a second of queued float32 samples, in bytes.
	minimumAudio = sampleRate * 4 / 2
)

// main parses the command-line flags and calls run, exiting with status 1
// and a message on stderr if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 runs until the window is closed)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "01-simple-playback:", err)
		os.Exit(1)
	}
}

// run initializes SDL, opens a playback stream and keeps it topped up
// with a sine wave until the window is closed or maxFrames frames have been
// drawn (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Audio Simple Playback", "1.0", "com.example.audio-simple-playback")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	// We don't need a window for audio-only things but it's good policy to have one.
	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/audio/simple-playback", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	// Always feed mono float32 data at 8000Hz; the stream converts it to
	// whatever the hardware wants on the other side.
	spec := sdl.AudioSpec{Channels: 1, Format: sdl.AUDIO_F32, Freq: sampleRate}
	stream := sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, sdl.NoAudioStreamCallback, nil)
	if stream == nil {
		return fmt.Errorf("couldn't create audio stream: %s", sdl.GetError())
	}
	defer sdl.DestroyAudioStream(stream)

	// OpenAudioDeviceStream starts the device paused; start it.
	sdl.ResumeAudioStreamDevice(stream)

	p := &player{stream: stream}
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if p.handleEvent(&event) {
				return nil
			}
		}
		p.feed()
		p.draw(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1) // keep headless runs from spinning when vsync is a no-op
		}
	}
	return nil
}

// player holds the per-frame state, mirroring the C SDL_AppEvent and
// SDL_AppIterate callbacks.
type player struct {
	stream            *sdl.AudioStream
	currentSineSample int
	samples           [512]float32 // fed each frame until the queue is full enough
}

// handleEvent reports whether the program should quit.
func (p *player) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// feed queues another block of the 440Hz tone if less than half a second is
// queued.
func (p *player) feed() {
	if sdl.GetAudioStreamQueued(p.stream) >= minimumAudio {
		return
	}
	for i := range p.samples {
		phase := float32(p.currentSineSample*toneFreq) / sampleRate
		p.samples[i] = float32(math.Sin(float64(phase * 2 * math.Pi)))
		p.currentSineSample++
	}
	// Wrap around to avoid floating-point errors.
	p.currentSineSample %= sampleRate
	sdl.PutAudioStreamSlice(p.stream, p.samples[:])
}

// draw blanks the window; there is nothing else to show. It does not present.
func (p *player) draw(r *sdl.Renderer) {
	sdl.RenderClear(r)
}
