// Command primitives draws lines, rectangles and points every frame.
// It is a port of SDL/examples/renderer/02-primitives/primitives.c.
//
// Run it with:
//
//	go run ./examples/renderer/02-primitives [-frames N]
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

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "02-primitives:", err)
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
	sdl.SetAppMetadata("Example Renderer Primitives", "1.0", "com.example.renderer-primitives")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/primitives", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	s := newScene()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if s.handleEvent(&event) {
				return nil
			}
		}
		s.draw(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// scene is the example's state: the random points scattered over the canvas.
type scene struct {
	points [500]sdl.FPoint
}

// newScene sets up some random points.
func newScene() *scene {
	s := &scene{}
	for i := range s.points {
		s.points[i].X = sdl.Randf()*440 + 100
		s.points[i].Y = sdl.Randf()*280 + 100
	}
	return s
}

// handleEvent reports whether the program should end.
func (s *scene) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// draw renders one frame without presenting it.
func (s *scene) draw(renderer *sdl.Renderer) {
	// Rendering draws over whatever was drawn before it.
	sdl.SetRenderDrawColor(renderer, 33, 33, 33, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// Filled rectangle in the middle of the canvas.
	sdl.SetRenderDrawColor(renderer, 0, 0, 255, sdl.ALPHA_OPAQUE)
	rect := sdl.FRect{X: 100, Y: 100, W: 440, H: 280}
	sdl.RenderFillRect(renderer, &rect)

	// Points across the canvas.
	sdl.SetRenderDrawColor(renderer, 255, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderPointsSlice(renderer, s.points[:])

	// Unfilled rectangle inset a little bit.
	sdl.SetRenderDrawColor(renderer, 0, 255, 0, sdl.ALPHA_OPAQUE)
	rect.X += 30
	rect.Y += 30
	rect.W -= 60
	rect.H -= 60
	sdl.RenderRect(renderer, &rect)

	// Two lines in an X across the whole canvas.
	sdl.SetRenderDrawColor(renderer, 255, 255, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderLine(renderer, 0, 0, 640, 480)
	sdl.RenderLine(renderer, 0, 480, 640, 0)
}
