// Command 11-color-mods loads a PNG into a texture and draws it three times
// per frame with different color modulations.
//
// It is a port of SDL/examples/renderer/11-color-mods/color-mods.c.
//
// Run it with:
//
//	go run ./examples/renderer/11-color-mods [-frames N]
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// Window dimensions in logical pixels.
const (
	windowWidth  = 640
	windowHeight = 480
)

//go:embed sample.png
var samplePNG []byte

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
		fmt.Fprintln(os.Stderr, "11-color-mods:", err)
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
	sdl.SetAppMetadata("Example Renderer Color Mods", "1.0", "com.example.renderer-color-mods")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/color-mods", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	g, err := newGame(sdl.GetTicks, renderer)
	if err != nil {
		return err
	}
	defer g.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if g.handleEvent(&event) {
				return nil
			}
		}
		g.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// game holds the texture and the time source, like the C program's globals.
type game struct {
	now     func() uint64
	texture *sdl.Texture
	w, h    float32
}

// newGame decodes the embedded PNG and uploads it as a texture of renderer.
func newGame(now func() uint64, renderer *sdl.Renderer) (*game, error) {
	// Load the embedded PNG through an in-memory stream; LoadPNG_IO closes it.
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return nil, fmt.Errorf("couldn't open PNG data: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	g := &game{now: now, w: float32(surface.W), h: float32(surface.H)}
	g.texture = sdl.CreateTextureFromSurface(renderer, surface)
	if g.texture == nil {
		sdl.DestroySurface(surface)
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	sdl.DestroySurface(surface)
	return g, nil
}

// handleEvent reports whether the program should quit.
func (g *game) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// close destroys the texture.
func (g *game) close() { sdl.DestroyTexture(g.texture) }

// centerMod returns the cycling modulation of the center texture at t seconds.
func centerMod(t float64) (r, g, b float32) {
	// The sine wave trick makes the center texture fade between colors smoothly.
	return float32(0.5 + 0.5*math.Sin(t)),
		float32(0.5 + 0.5*math.Sin(t+math.Pi*2/3)),
		float32(0.5 + 0.5*math.Sin(t+math.Pi*4/3))
}

// draw clears and draws one frame; the caller presents it.
func (g *game) draw(renderer *sdl.Renderer) {
	w, h := g.w, g.h
	texture := g.texture
	red, green, blue := centerMod(float64(g.now()) / 1000.0)

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// Top left: blue only.
	sdl.SetTextureColorModFloat(texture, 0, 0, 1)
	sdl.RenderTexture(renderer, texture, nil, &sdl.FRect{X: 0, Y: 0, W: w, H: h})

	// Center: cycling modulation.
	sdl.SetTextureColorModFloat(texture, red, green, blue)
	sdl.RenderTexture(renderer, texture, nil, &sdl.FRect{X: (windowWidth - w) / 2, Y: (windowHeight - h) / 2, W: w, H: h})

	// Bottom right: red only.
	sdl.SetTextureColorModFloat(texture, 1, 0, 0)
	sdl.RenderTexture(renderer, texture, nil, &sdl.FRect{X: windowWidth - w, Y: windowHeight - h, W: w, H: h})
}
