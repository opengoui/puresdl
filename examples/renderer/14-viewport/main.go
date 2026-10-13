// Command 14-viewport draws a texture several times per frame while
// adjusting the render viewport, showing that a viewport moves the origin and
// clips drawing but does not scale it.
//
// It is a port of SDL/examples/renderer/14-viewport/viewport.c.
//
// Run it with:
//
//	go run ./examples/renderer/14-viewport [-frames N]
package main

import (
	_ "embed"
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
		fmt.Fprintln(os.Stderr, "14-viewport:", err)
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
	sdl.SetAppMetadata("Example Renderer Viewport", "1.0", "com.example.renderer-viewport")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/viewport", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	configure(renderer)

	a, err := newApp(renderer)
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
		a.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// configure fixes the logical size so the scene letterboxes in a resized window.
func configure(renderer *sdl.Renderer) {
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}

// app holds the example state: the sample texture and its size.
type app struct {
	texture       *sdl.Texture
	width, height float32
}

// newApp loads the embedded PNG and uploads it as a texture of renderer.
func newApp(renderer *sdl.Renderer) (*app, error) {
	src := sdl.IOFromBytes(samplePNG)
	surface := sdl.LoadPNG_IO(src, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	a := &app{width: float32(surface.W), height: float32(surface.H)}
	a.texture = sdl.CreateTextureFromSurface(renderer, surface)
	sdl.DestroySurface(surface)
	if a.texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return a, nil
}

// handleEvent reports whether the event asks the program to end.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// close destroys the texture.
func (a *app) close() { sdl.DestroyTexture(a.texture) }

// draw renders one frame without presenting it.
func (a *app) draw(renderer *sdl.Renderer) {
	texture := a.texture
	dst := sdl.FRect{X: 0, Y: 0, W: a.width, H: a.height}

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// Whole window as the viewport (nil).
	sdl.SetRenderViewport(renderer, nil)
	sdl.RenderTexture(renderer, texture, nil, &dst)

	// Bottom right quarter of the window.
	vp := sdl.Rect{X: windowWidth / 2, Y: windowHeight / 2, W: windowWidth / 2, H: windowHeight / 2}
	sdl.SetRenderViewport(renderer, &vp)
	sdl.RenderTexture(renderer, texture, nil, &dst)

	// Bottom 20% of the window. Note it clips the width.
	vp = sdl.Rect{X: 0, Y: windowHeight - windowHeight/5, W: windowWidth / 5, H: windowHeight / 5}
	sdl.SetRenderViewport(renderer, &vp)
	sdl.RenderTexture(renderer, texture, nil, &dst)

	// Drawing above the viewport clips.
	vp = sdl.Rect{X: 100, Y: 200, W: windowWidth, H: windowHeight}
	sdl.SetRenderViewport(renderer, &vp)
	dst.Y = -50
	sdl.RenderTexture(renderer, texture, nil, &dst)
}
