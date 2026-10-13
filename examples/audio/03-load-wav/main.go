// Command load-wav loads a .wav file and pushes it through an audio stream in
// a loop. It's a port of SDL/examples/audio/03-load-wav/load-wav.c. The sample
// is embedded in the binary instead of read from the base path.
//
// Run it with:
//
//	go run ./examples/audio/03-load-wav
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// sampleWAV is the embedded .wav file that is played in a loop.
//
//go:embed sample.wav
var sampleWAV []byte

// init locks the main goroutine to the process main thread. SDL video and
// event functions must run there (mandatory on macOS), and locking in init
// makes main.main start on it.
func init() {
	runtime.LockOSThread()
}

// main parses the command-line flags and calls run, exiting with status 1
// and a message on stderr if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "load-wav:", err)
		os.Exit(1)
	}
}

// run initializes SDL, decodes the embedded .wav and keeps it queued on a
// playback stream until the program quits or maxFrames frames have been drawn
// (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Audio Load Wave", "1.0", "com.example.audio-load-wav")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	// We don't need a window for audio-only things but it's good policy to have one.
	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/audio/load-wav", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	// Decode the embedded .wav; the result is a Go copy.
	spec, wav, err := sdl.LoadWAVBytes(sampleWAV)
	if err != nil {
		return fmt.Errorf("couldn't load .wav file: %w", err)
	}

	// Create the stream in the wav's format; it converts to what the hardware wants.
	stream := sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, sdl.NoAudioStreamCallback, nil)
	if stream == nil {
		return fmt.Errorf("couldn't create audio stream: %s", sdl.GetError())
	}
	defer sdl.DestroyAudioStream(stream)

	// OpenAudioDeviceStream starts the device paused.
	sdl.ResumeAudioStreamDevice(stream)

	p := &player{stream: stream, wav: wav}
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if p.handleEvent(&event) {
				return nil
			}
		}
		p.iterate()
		p.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// player is the per-frame state: the decoded sample and the stream it is
// looped into.
type player struct {
	stream *sdl.AudioStream
	wav    []byte
}

// handleEvent reports whether the event asks the program to quit.
func (p *player) handleEvent(e *sdl.Event) (quit bool) {
	switch sdl.EventType(e.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_KEY_DOWN:
		return e.Key().Key == sdl.SDLK_ESCAPE
	}
	return false
}

// iterate pushes another copy of the sample if less than the whole sample is
// queued, so there is always plenty of data queued for playback.
func (p *player) iterate() {
	if sdl.GetAudioStreamQueued(p.stream) < int32(len(p.wav)) {
		sdl.PutAudioStreamBytes(p.stream, p.wav)
	}
}

// draw blanks the window; there is nothing else to show.
func (p *player) draw(r *sdl.Renderer) {
	sdl.RenderClear(r)
}
