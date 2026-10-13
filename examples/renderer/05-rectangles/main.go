// Command 05-rectangles draws outlined and filled rectangles that grow and
// shrink over time. It is a port of SDL/examples/renderer/05-rectangles/rectangles.c.
//
// Run it with:
//
//	go run ./examples/renderer/05-rectangles [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// Window dimensions in logical pixels.
const (
	windowWidth  = 640
	windowHeight = 480
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
		fmt.Fprintln(os.Stderr, "05-rectangles:", err)
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
	sdl.SetAppMetadata("Example Renderer Rectangles", "1.0", "com.example.renderer-rectangles")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/rectangles", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	configure(renderer)

	a := newApp(sdl.GetTicks)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.draw(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// configure applies the fixed logical size shared by all frames.
func configure(renderer *sdl.Renderer) {
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}

// app holds the example state: only a time source, as the animation is a
// pure function of time.
type app struct {
	now func() uint64
}

// newApp returns an app that reads the current time from now.
func newApp(now func() uint64) *app { return &app{now: now} }

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// draw renders one frame at a.now() without presenting it.
func (a *app) draw(renderer *sdl.Renderer) {
	var rects [16]sdl.FRect
	now := a.now()

	// The rectangles grow and shrink over a few seconds.
	direction := float32(-1)
	if now%2000 >= 1000 {
		direction = 1
	}
	scale := float32(int(now%1000)-500) / 500 * direction

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// A single outlined rectangle (square, really).
	rects[0].X, rects[0].Y = 100, 100
	rects[0].W = 100 + 100*scale
	rects[0].H = rects[0].W
	sdl.SetRenderDrawColor(renderer, 255, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderRect(renderer, &rects[0])

	// Several outlined rectangles with one call.
	for i := 0; i < 3; i++ {
		size := float32(i+1) * 50
		rects[i].W = size + size*scale
		rects[i].H = rects[i].W
		rects[i].X = (windowWidth - rects[i].W) / 2
		rects[i].Y = (windowHeight - rects[i].H) / 2
	}
	sdl.SetRenderDrawColor(renderer, 0, 255, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderRectsSlice(renderer, rects[:3])

	// A filled rectangle.
	rects[0] = sdl.FRect{X: 400, Y: 50, W: 100 + 100*scale, H: 50 + 50*scale}
	sdl.SetRenderDrawColor(renderer, 0, 0, 255, sdl.ALPHA_OPAQUE)
	sdl.RenderFillRect(renderer, &rects[0])

	// Many filled rectangles at once.
	w := float32(windowWidth) / float32(len(rects))
	for i := range rects {
		h := float32(i) * 8
		rects[i] = sdl.FRect{X: float32(i) * w, Y: windowHeight - h, W: w, H: h}
	}
	sdl.SetRenderDrawColor(renderer, 255, 255, 255, sdl.ALPHA_OPAQUE)
	sdl.RenderFillRectsSlice(renderer, rects[:])
}
