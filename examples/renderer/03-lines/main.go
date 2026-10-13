// Command lines draws a Christmas-tree-like shape with nothing but lines
// every frame.
//
// It is a port of SDL/examples/renderer/03-lines/lines.c.
//
// Run it with:
//
//	go run ./examples/renderer/03-lines [-frames N]
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

// linePoints is the connected tree outline drawn with a single RenderLines call.
var linePoints = []sdl.FPoint{
	{X: 100, Y: 354}, {X: 220, Y: 230}, {X: 140, Y: 230}, {X: 320, Y: 100}, {X: 500, Y: 230},
	{X: 420, Y: 230}, {X: 540, Y: 354}, {X: 400, Y: 354}, {X: 100, Y: 354},
}

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "lines:", err)
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
	sdl.SetAppMetadata("Example Renderer Lines", "1.0", "com.example.renderer-lines")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/lines", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	configureRenderer(renderer)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if handleEvent(&event) {
				return nil
			}
		}
		draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// configureRenderer applies the fixed 640x480 letterboxed logical size.
func configureRenderer(renderer *sdl.Renderer) {
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}

// handleEvent reports whether the event asks the program to quit.
func handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// draw renders one frame; the caller presents it.
func draw(renderer *sdl.Renderer) {
	// Rendering draws over whatever was drawn before it.
	sdl.SetRenderDrawColor(renderer, 100, 100, 100, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// Lines drawn one at a time: the brown trunk.
	sdl.SetRenderDrawColor(renderer, 127, 49, 32, sdl.ALPHA_OPAQUE)
	sdl.RenderLine(renderer, 240, 450, 400, 450)
	sdl.RenderLine(renderer, 240, 356, 400, 356)
	sdl.RenderLine(renderer, 240, 356, 240, 450)
	sdl.RenderLine(renderer, 400, 356, 400, 450)

	// A series of connected lines in a single batch.
	sdl.SetRenderDrawColor(renderer, 0, 255, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderLinesSlice(renderer, linePoints)

	// Lines radiating from a center point in a circle, with a random color
	// per line so it functions as animation.
	const size = float32(30)
	const x = float32(320)
	const y = 95 - size/2
	for i := 0; i < 360; i++ {
		r := float64(i) * (math.Pi / 180)
		sdl.SetRenderDrawColor(renderer, uint8(sdl.Rand(256)), uint8(sdl.Rand(256)), uint8(sdl.Rand(256)), sdl.ALPHA_OPAQUE)
		sdl.RenderLine(renderer, x, y, x+float32(math.Cos(r))*size, y+float32(math.Sin(r))*size)
	}
}
