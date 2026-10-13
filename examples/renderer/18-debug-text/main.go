// Command 18-debug-text draws some text with RenderDebugText every frame.
//
// It is a port of SDL/examples/renderer/18-debug-text/debug-text.c.
//
// Run it with:
//
//	go run ./examples/renderer/18-debug-text [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to its OS thread, which SDL requires for
// window and event handling.
func init() {
	runtime.LockOSThread()
}

// Window dimensions in logical pixels.
const (
	windowWidth  = 640
	windowHeight = 480
)

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "18-debug-text:", err)
		os.Exit(1)
	}
}

// run initializes SDL, creates the window and renderer, and drives the frame
// loop until a quit event or maxFrames frames (0 means no limit). It returns
// the first setup error; SDL resources are released on return.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Renderer Debug Texture", "1.0", "com.example.renderer-debug-text")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/debug-text", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	d := newDemo(sdl.GetTicks)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if d.handleEvent(&event) {
				return nil
			}
		}
		d.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// demo is the example state: only a time source, since the scene is stateless.
type demo struct {
	now func() uint64
}

// newDemo returns a demo that needs no state.
func newDemo(now func() uint64) *demo { return &demo{now: now} }

// handleEvent mirrors SDL_AppEvent and reports whether the program should quit.
func (d *demo) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// draw mirrors SDL_AppIterate minus the present: it clears and draws one frame.
func (d *demo) draw(renderer *sdl.Renderer) {
	const charsize = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE

	// Rendering draws over whatever was drawn before it.
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	sdl.SetRenderDrawColor(renderer, 255, 255, 255, sdl.ALPHA_OPAQUE)
	sdl.RenderDebugText(renderer, 272, 100, "Hello world!")
	sdl.RenderDebugText(renderer, 224, 150, "This is some debug text.")

	sdl.SetRenderDrawColor(renderer, 51, 102, 255, sdl.ALPHA_OPAQUE) // light blue
	sdl.RenderDebugText(renderer, 184, 200, "You can do it in different colors.")
	sdl.SetRenderDrawColor(renderer, 255, 255, 255, sdl.ALPHA_OPAQUE)

	sdl.SetRenderScale(renderer, 4, 4)
	sdl.RenderDebugText(renderer, 14, 65, "It can be scaled.")
	sdl.SetRenderScale(renderer, 1, 1)
	sdl.RenderDebugText(renderer, 64, 350, "This only does ASCII chars. So this laughing emoji won't draw: 🤣")

	sdl.RenderDebugTextFormat(renderer, float32(windowWidth-charsize*46)/2, 400,
		"(This program has been running for %d seconds.)", int(d.now()/1000))
}
