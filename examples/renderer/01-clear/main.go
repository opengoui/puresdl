// Command 01-clear creates a window and renderer, then clears the window to a
// different color every frame so it smoothly fades between colors.
//
// It is a port of SDL/examples/renderer/01-clear/clear.c. The program exits on
// the quit event; -frames N exits after N frames (0 runs until quit).
//
// Run it with:
//
//	go run ./examples/renderer/01-clear
package main

import (
	"flag"
	"fmt"
	"math"
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
		fmt.Fprintln(os.Stderr, "01-clear:", err)
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
	sdl.SetAppMetadata("Example Renderer Clear", "1.0", "com.example.renderer-clear")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/clear", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	configure(renderer)

	c := newClearer(sdl.GetTicks)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if c.handleEvent(&event) {
				return nil
			}
		}
		c.draw(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// clearer holds the per-frame state: only the time source.
type clearer struct {
	now func() uint64 // milliseconds, like sdl.GetTicks
}

// newClearer returns a clearer that reads the current time from now.
func newClearer(now func() uint64) *clearer { return &clearer{now: now} }

// configure sets the fixed 640x480 letterbox presentation.
func configure(r *sdl.Renderer) {
	sdl.SetRenderLogicalPresentation(r, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}

// handleEvent reports whether the program should quit.
func (c *clearer) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// colorAt returns the fade color for a time in seconds.
func colorAt(now float64) (r, g, b float32) {
	// The sine wave trick makes the color fade smoothly.
	return float32(0.5 + 0.5*math.Sin(now)),
		float32(0.5 + 0.5*math.Sin(now+math.Pi*2/3)),
		float32(0.5 + 0.5*math.Sin(now+math.Pi*4/3))
}

// draw clears the target to the color for the current time; it does not present.
func (c *clearer) draw(r *sdl.Renderer) {
	red, green, blue := colorAt(float64(c.now()) / 1000.0)
	sdl.SetRenderDrawColorFloat(r, red, green, blue, 1.0)
	sdl.RenderClear(r)
}
