// Command 02-simple-playback-callback ports
// SDL/examples/audio/02-simple-playback-callback/simple-playback-callback.c.
//
// It opens an audio stream with a callback that generates a 440Hz sine wave
// on demand, while the main loop only keeps a blank window alive.
//
//	SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy go run ./examples/audio/02-simple-playback-callback -frames 30
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"unsafe"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to the process main thread. SDL video and
// event functions must run there (mandatory on macOS), and locking in init
// makes main.main start on it.
func init() {
	// SDL video and event functions must run on the process main thread.
	runtime.LockOSThread()
}

const (
	// sampleRate is the stream's sample rate in Hz.
	sampleRate = 8000
	// toneFreq is the pitch of the generated tone in Hz.
	toneFreq = 440
	// chunkSize is the maximum number of samples generated per
	// PutAudioStreamBytes call.
	chunkSize = 128 // samples generated per PutAudioStreamBytes call
)

// synth generates the 440Hz tone. Its position is only touched from the audio
// callback, which SDL serializes through the stream lock.
type synth struct {
	current int
}

// feed is the audio stream callback body. additional is how many more bytes
// the stream needs than it currently has queued.
func (s *synth) feed(astream *sdl.AudioStream, additional int) {
	n := additional / 4             // bytes to float32 samples
	var samples [chunkSize * 4]byte // native-endian float32 samples
	for n > 0 {
		total := min(n, chunkSize)
		// Generate a 440Hz pure tone.
		for i := 0; i < total; i++ {
			phase := float32(s.current*toneFreq) / float32(sampleRate)
			sample := float32(math.Sin(float64(phase * 2 * math.Pi)))
			binary.NativeEndian.PutUint32(samples[i*4:], math.Float32bits(sample))
			s.current++
		}
		// Wrap around to avoid floating-point errors.
		s.current %= sampleRate
		sdl.PutAudioStreamBytes(astream, samples[:total*4])
		n -= total
	}
}

// app holds the window, renderer and callback-driven audio stream.
type app struct {
	window   *sdl.Window
	renderer *sdl.Renderer
	stream   *sdl.AudioStream
	synth    synth
}

// newApp creates the window, renderer and (paused) audio stream. SDL must be
// initialized with video and audio.
func newApp() (*app, error) {
	a := &app{}
	if !sdl.CreateWindowAndRenderer("examples/audio/simple-playback-callback", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &a.window, &a.renderer) {
		return nil, fmt.Errorf("Couldn't create window/renderer: %s", sdl.GetError())
	}
	sdl.SetRenderLogicalPresentation(a.renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	// Always feed mono float32 at 8000Hz; the stream converts for the hardware.
	spec := sdl.AudioSpec{Channels: 1, Format: sdl.AUDIO_F32, Freq: sampleRate}
	cb := sdl.NewAudioStreamCallback(func(_ unsafe.Pointer, astream *sdl.AudioStream, additional, _ int32) {
		a.synth.feed(astream, int(additional))
	})
	a.stream = sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, cb, nil)
	if a.stream == nil {
		err := fmt.Errorf("Couldn't create audio stream: %s", sdl.GetError())
		a.close()
		return nil, err
	}
	return a, nil
}

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// draw blanks the window; it does not present.
func (a *app) draw() {
	sdl.RenderClear(a.renderer)
}

// close destroys the stream, renderer and window that were created. It is
// safe to call on a partially initialized app and more than once.
func (a *app) close() {
	if a.stream != nil {
		sdl.DestroyAudioStream(a.stream)
		a.stream = nil
	}
	if a.renderer != nil {
		sdl.DestroyRenderer(a.renderer)
		a.renderer = nil
	}
	if a.window != nil {
		sdl.DestroyWindow(a.window)
		a.window = nil
	}
}

// main parses the command-line flags and calls run, exiting with status 1
// and a message on stderr if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 runs until the window is closed)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "02-simple-playback-callback:", err)
		os.Exit(1)
	}
}

// run initializes SDL, starts callback-driven playback and runs the event
// loop until the window is closed or maxFrames frames have been drawn (0 means
// no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Simple Audio Playback Callback", "1.0", "com.example.audio-simple-playback-callback")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO) {
		return fmt.Errorf("Couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.close()

	// OpenAudioDeviceStream starts the device paused.
	sdl.ResumeAudioStreamDevice(a.stream)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		// Nothing to draw; just blank the window.
		a.draw()
		sdl.RenderPresent(a.renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}
