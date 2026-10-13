// Command 05-planar-data draws two clickable buttons. Each plays a sound
// through only the left or only the right audio channel, fed via separate
// ("planar") arrays. It is a port of SDL's
// SDL/examples/audio/05-planar-data/planar-data.c.
//
// Run it with:
//
//	go run ./examples/audio/05-planar-data
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// Raw PCM samples (Uint8, 1 channel, 4000Hz) taken from the C source.
var (
	// left holds the embedded sound played on the left channel.
	//go:embed left.raw
	left []byte
	// right holds the embedded sound played on the right channel.
	//go:embed right.raw
	right []byte
)

// Button rectangles in logical (640x480) render coordinates.
var (
	// rectLeftButton is the clickable area of the LEFT button.
	rectLeftButton = sdl.FRect{X: 100, Y: 170, W: 100, H: 100}
	// rectRightButton is the clickable area of the RIGHT button.
	rectRightButton = sdl.FRect{X: 440, Y: 170, W: 100, H: 100}
)

// init pins the main goroutine to the OS thread, as SDL requires.
func init() {
	runtime.LockOSThread()
}

// main parses flags and runs the example, exiting non-zero on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "05-planar-data:", err)
		os.Exit(1)
	}
}

// putPlanar queues a single-channel sound on one side of a stereo stream.
// A nil plane tells SDL that channel is silent.
func putPlanar(stream *sdl.AudioStream, data []byte, rightChannel bool) {
	planes := [][]byte{data, nil}
	if rightChannel {
		planes = [][]byte{nil, data}
	}
	sdl.PutAudioStreamPlanarDataSlices(stream, planes, int32(len(data)))
	sdl.FlushAudioStream(stream) // that's all we're playing until it completes
}

// Button fill colors while idle and while its sound plays.
var (
	// colorIdle is the fill color of a button that is not playing.
	colorIdle = sdl.Color{R: 0, G: 0, B: 255, A: 255}
	// colorPlaying is the fill color of the button whose sound is playing.
	colorPlaying = sdl.Color{R: 0, G: 255, B: 0, A: 255}
)

// app is the state the C program keeps in globals.
type app struct {
	renderer *sdl.Renderer
	stream   *sdl.AudioStream
	// playingSound is -1 if playing left, 1 if playing right, 0 if not playing.
	playingSound int
}

// renderButton fills rect with the idle or playing color, depending on whether
// buttonValue matches the current playingSound, and centers str on it.
func (a *app) renderButton(rect *sdl.FRect, str string, buttonValue int) {
	c := colorIdle
	if a.playingSound == buttonValue {
		c = colorPlaying
	}
	sdl.SetRenderDrawColor(a.renderer, c.R, c.G, c.B, c.A)
	sdl.RenderFillRect(a.renderer, rect)
	sdl.SetRenderDrawColor(a.renderer, 255, 255, 255, 255)

	x := rect.X + (rect.W-float32(sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE*len(str)))/2
	y := rect.Y + (rect.H-sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)/2
	sdl.RenderDebugText(a.renderer, x, y, str)
}

// handleEvent mirrors SDL_AppEvent. It reports whether the program should end.
func (a *app) handleEvent(event *sdl.Event) (quit bool) {
	sdl.ConvertEventToRenderCoordinates(a.renderer, event)
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_MOUSE_BUTTON_DOWN:
		if a.playingSound != 0 {
			break
		}
		b := event.Button()
		pt := sdl.FPoint{X: b.X, Y: b.Y}
		if sdl.PointInRectFloat(&pt, &rectLeftButton) {
			putPlanar(a.stream, left, false)
			a.playingSound = -1
		} else if sdl.PointInRectFloat(&pt, &rectRightButton) {
			putPlanar(a.stream, right, true)
			a.playingSound = 1
		}
	}
	return false
}

// advance is the state part of SDL_AppIterate: a finished sound frees the buttons.
func (a *app) advance() {
	if a.playingSound != 0 && sdl.GetAudioStreamQueued(a.stream) == 0 {
		a.playingSound = 0
	}
}

// draw clears and draws both buttons without presenting.
func (a *app) draw() {
	sdl.SetRenderDrawColor(a.renderer, 0, 0, 0, 255)
	sdl.RenderClear(a.renderer)
	a.renderButton(&rectLeftButton, "LEFT", -1)
	a.renderButton(&rectRightButton, "RIGHT", 1)
}

// run initializes SDL, opens the window and audio stream, and runs the event
// loop until quit or, if maxFrames is positive, that many frames have been drawn.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Audio Planar Data", "1.0", "com.example.audio-planar-data")
	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/audio/planar-data", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	spec := sdl.AudioSpec{Format: sdl.AUDIO_U8, Channels: 2, Freq: 4000}
	stream := sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, 0, nil)
	if stream == nil {
		return fmt.Errorf("SDL_OpenAudioDeviceStream: %s", sdl.GetError())
	}
	defer sdl.DestroyAudioStream(stream)
	sdl.ResumeAudioStreamDevice(stream) // the device starts paused

	a := &app{renderer: renderer, stream: stream}

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.advance()
		a.draw()
		sdl.RenderPresent(renderer)
	}
	return nil
}
