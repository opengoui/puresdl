// Command multiple-streams loads two .wav files, puts them in audio streams
// and binds them to one playback device, repeating both sounds on loop. This
// shows several streams mixing into a single playback device. It is a port of
// SDL/examples/audio/04-multiple-streams/multiple-streams.c.
//
// Run it with:
//
//	go run ./examples/audio/04-multiple-streams
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// sampleWAV holds the embedded sample.wav file contents.
//
//go:embed sample.wav
var sampleWAV []byte

// swordWAV holds the embedded sword.wav file contents.
//
//go:embed sword.wav
var swordWAV []byte

// init locks the main goroutine to the OS thread, as SDL requires for
// video and event handling.
func init() {
	runtime.LockOSThread()
}

// sound is something that is playing: the audio stream itself, plus the
// original data so it can be refilled to loop.
type sound struct {
	wav    []byte
	stream *sdl.AudioStream
}

// main parses the -frames flag, runs the example and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "multiple-streams:", err)
		os.Exit(1)
	}
}

// initSound decodes an embedded .wav file, creates a stream for it and binds
// the stream to the audio device.
func initSound(device sdl.AudioDeviceID, name string, data []byte, s *sound) error {
	spec, wav, err := sdl.LoadWAVBytes(data)
	if err != nil {
		return fmt.Errorf("couldn't load .wav file %s: %w", name, err)
	}
	s.wav = wav

	// Source format is the wav's; the destination format is left nil and
	// becomes what the device wants once bound.
	s.stream = sdl.CreateAudioStream(&spec, nil)
	if s.stream == nil {
		return fmt.Errorf("couldn't create audio stream: %s", sdl.GetError())
	}
	// Once bound, it starts playing when there is data available.
	if !sdl.BindAudioStream(device, s.stream) {
		return fmt.Errorf("failed to bind '%s' stream to device: %s", name, sdl.GetError())
	}
	return nil
}

// app is the audio state: the playback device and the two looping sounds.
type app struct {
	device sdl.AudioDeviceID
	sounds [2]sound
}

// newApp opens the default playback device and binds one stream per sound.
// SDL audio must be initialized. On error everything opened so far is
// released.
func newApp() (*app, error) {
	a := &app{}
	// Open the default audio device in whatever format it prefers; the
	// streams adjust to it.
	a.device = sdl.OpenAudioDevice(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, nil)
	if a.device == 0 {
		return nil, fmt.Errorf("couldn't open audio device: %s", sdl.GetError())
	}
	if err := initSound(a.device, "sample.wav", sampleWAV, &a.sounds[0]); err != nil {
		a.close()
		return nil, err
	}
	if err := initSound(a.device, "sword.wav", swordWAV, &a.sounds[1]); err != nil {
		a.close()
		return nil, err
	}
	return a, nil
}

// handleEvent reports whether the event asks the program to quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	switch sdl.EventType(e.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_KEY_DOWN:
		return e.Key().Key == sdl.SDLK_ESCAPE
	}
	return false
}

// iterate tops up each sound: if less than a full copy of the audio is
// queued, another copy is put in.
func (a *app) iterate() {
	for i := range a.sounds {
		s := &a.sounds[i]
		if s.stream == nil {
			continue
		}
		if int(sdl.GetAudioStreamQueued(s.stream)) < len(s.wav) {
			sdl.PutAudioStreamBytes(s.stream, s.wav)
		}
	}
}

// draw just blanks the screen; it does not present.
func (a *app) draw(r *sdl.Renderer) {
	sdl.SetRenderDrawColor(r, 0, 0, 0, 255)
	sdl.RenderClear(r)
}

// close releases the device and the streams.
func (a *app) close() {
	if a.device != 0 {
		sdl.CloseAudioDevice(a.device)
		a.device = 0
	}
	for i := range a.sounds {
		if a.sounds[i].stream != nil {
			sdl.DestroyAudioStream(a.sounds[i].stream)
			a.sounds[i].stream = nil
		}
	}
}

// run initializes SDL, creates a window and renderer, opens the audio
// device with both looping sounds and runs the frame loop until a quit event
// arrives or maxFrames frames have been drawn (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Audio Multiple Streams", "1.0", "com.example.audio-multiple-streams")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/audio/multiple-streams", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.iterate()
		a.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}
