// Command 07-streaming-textures updates a streaming texture every frame
// (a green strip moving over black) and draws it centered in the window.
//
// It is a port of SDL/examples/renderer/07-streaming-textures/streaming-textures.c.
//
// Run it with:
//
//	go run ./examples/renderer/07-streaming-textures [-frames N]
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

// textureSize is the edge of the square streaming texture; the other
// constants are the window dimensions in logical pixels.
const (
	textureSize  = 150
	windowWidth  = 640
	windowHeight = 480
)

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "streaming-textures:", err)
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
	sdl.SetAppMetadata("Example Renderer Streaming Textures", "1.0", "com.example.renderer-streaming-textures")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/streaming-textures", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	a, err := newApp(renderer, sdl.GetTicks)
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
		a.draw()
		sdl.RenderPresent(renderer)
	}
	return nil
}

// app is the per-run state: the renderer, the streaming texture and the time
// source that drives the animation.
type app struct {
	renderer *sdl.Renderer
	texture  *sdl.Texture
	now      func() uint64
}

// newApp sets up letterboxed logical presentation on renderer and creates the
// streaming texture.
func newApp(renderer *sdl.Renderer, now func() uint64) (*app, error) {
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
	texture := sdl.CreateTexture(renderer, sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_STREAMING, textureSize, textureSize)
	if texture == nil {
		return nil, fmt.Errorf("couldn't create streaming texture: %s", sdl.GetError())
	}
	return &app{renderer: renderer, texture: texture, now: now}, nil
}

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// close destroys the streaming texture.
func (a *app) close() { sdl.DestroyTexture(a.texture) }

// draw clears and draws one frame without presenting it.
func (a *app) draw() {
	renderer, texture := a.renderer, a.texture
	now := a.now()

	// Have some color move around over a few seconds.
	direction := float32(-1)
	if now%2000 >= 1000 {
		direction = 1
	}
	scale := float32(int(now%1000)-500) / 500 * direction

	// Locking a streaming texture is write-only: every locked pixel must be
	// written. LockTextureToSurface wraps the pixels in a temporary surface.
	var surface *sdl.Surface
	if sdl.LockTextureToSurface(texture, nil, &surface) {
		details := sdl.GetPixelFormatDetails(surface.Format)
		sdl.FillSurfaceRect(surface, nil, sdl.MapRGB(details, nil, 0, 0, 0))
		h := int32(textureSize / 10)
		r := sdl.Rect{X: 0, Y: int32(float32(textureSize-h) * ((scale + 1) / 2)), W: textureSize, H: h}
		sdl.FillSurfaceRect(surface, &r, sdl.MapRGB(details, nil, 0, 255, 0))
		sdl.UnlockTexture(texture) // uploads the changes and frees the temporary surface
	}

	sdl.SetRenderDrawColor(renderer, 66, 66, 66, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	dst := sdl.FRect{
		X: float32(windowWidth-textureSize) / 2,
		Y: float32(windowHeight-textureSize) / 2,
		W: textureSize,
		H: textureSize,
	}
	sdl.RenderTexture(renderer, texture, nil, &dst)
}
