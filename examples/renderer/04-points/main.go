// Command 04-points moves a cloud of single points across the window.
// It ports SDL/examples/renderer/04-points/points.c.
//
// Run it with:
//
//	go run ./examples/renderer/04-points [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// Window geometry and point-cloud tuning constants.
const (
	windowWidth  = 640
	windowHeight = 480

	numPoints         = 500
	minPixelsPerSec   = 30 // move at least this many pixels per second
	maxPixelsPerSec   = 60 // move this many pixels per second at most
	logicalPresLetter = sdl.LOGICAL_PRESENTATION_LETTERBOX
)

// init pins the main goroutine to its OS thread, which SDL requires for
// window and event handling.
func init() {
	runtime.LockOSThread()
}

// randomSpeed returns a random speed in pixels per second between
// minPixelsPerSec and maxPixelsPerSec.
func randomSpeed() float32 {
	return minPixelsPerSec + sdl.Randf()*(maxPixelsPerSec-minPixelsPerSec)
}

// cloud is the example state. Points are tracked as parallel arrays so all
// coordinates can be passed to the renderer in a single call.
type cloud struct {
	points      [numPoints]sdl.FPoint
	pointSpeeds [numPoints]float32
	lastTime    uint64
	now         func() uint64
}

// newCloud scatters the points and starts the clock (SDL_AppInit).
func newCloud(now func() uint64) *cloud {
	c := &cloud{now: now}
	for i := range c.points {
		c.points[i].X = sdl.Randf() * windowWidth
		c.points[i].Y = sdl.Randf() * windowHeight
		c.pointSpeeds[i] = randomSpeed()
	}
	c.lastTime = now()
	return c
}

// handleEvent reports whether the program should quit (SDL_AppEvent).
func (c *cloud) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// advance moves the points by the time elapsed since the last call.
func (c *cloud) advance() {
	now := c.now()
	elapsed := float32(now-c.lastTime) / 1000
	for i := range c.points {
		distance := elapsed * c.pointSpeeds[i]
		c.points[i].X += distance
		c.points[i].Y += distance
		if c.points[i].X >= windowWidth || c.points[i].Y >= windowHeight {
			// Off the screen; restart it elsewhere.
			if sdl.Rand(2) != 0 {
				c.points[i].X = sdl.Randf() * windowWidth
				c.points[i].Y = 0
			} else {
				c.points[i].X = 0
				c.points[i].Y = sdl.Randf() * windowHeight
			}
			c.pointSpeeds[i] = randomSpeed()
		}
	}
	c.lastTime = now
}

// draw clears to black and plots the points in white without presenting.
func (c *cloud) draw(r *sdl.Renderer) {
	sdl.SetRenderDrawColor(r, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(r)
	sdl.SetRenderDrawColor(r, 255, 255, 255, sdl.ALPHA_OPAQUE)
	sdl.RenderPointsSlice(r, c.points[:])
}

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "04-points:", err)
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
	sdl.SetAppMetadata("Example Renderer Points", "1.0", "com.example.renderer-points")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/points", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, logicalPresLetter)

	c := newCloud(sdl.GetTicks)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if c.handleEvent(&event) {
				return nil
			}
		}
		c.advance()
		c.draw(renderer)
		sdl.RenderPresent(renderer)

		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}
