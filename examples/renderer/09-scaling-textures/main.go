// Command 09-scaling-textures loads a PNG into a texture and draws it
// every frame, growing and shrinking it over a couple of seconds.
//
// It is a port of SDL/examples/renderer/09-scaling-textures/scaling-textures.c.
//
// Run it with:
//
//	go run ./examples/renderer/09-scaling-textures [-frames N]
package main

import (
	_ "embed"
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

//go:embed sample.png
var samplePNG []byte

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "09-scaling-textures:", err)
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
	sdl.SetAppMetadata("Example Renderer Scaling Textures", "1.0", "com.example.renderer-scaling-textures")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/scaling-textures", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

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
		a.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// app is the example state: the texture and the time source.
type app struct {
	texture    *sdl.Texture
	texW, texH float32
	now        func() uint64
}

// newApp decodes the embedded PNG into a texture owned by the app.
func newApp(renderer *sdl.Renderer, now func() uint64) (*app, error) {
	// IOFromBytes copies the data; the stream is closed by LoadPNG_IO.
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return nil, fmt.Errorf("couldn't open PNG data: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	defer sdl.DestroySurface(surface)
	texture := sdl.CreateTextureFromSurface(renderer, surface)
	if texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return &app{texture: texture, texW: float32(surface.W), texH: float32(surface.H), now: now}, nil
}

// handleEvent reports whether the program should end.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// close destroys the texture.
func (a *app) close() { sdl.DestroyTexture(a.texture) }

// dstRect returns the centered destination rectangle at time now (ms).
func dstRect(now uint64, texW, texH float32) sdl.FRect {
	// The texture grows and shrinks over a few seconds.
	direction := float32(-1)
	if now%2000 >= 1000 {
		direction = 1
	}
	scale := float32(int(now%1000)-500) / 500 * direction

	var dst sdl.FRect
	dst.W = texW + texW*scale
	dst.H = texH + texH*scale
	dst.X = (windowWidth - dst.W) / 2
	dst.Y = (windowHeight - dst.H) / 2
	return dst
}

// draw clears and draws one frame without presenting it.
func (a *app) draw(renderer *sdl.Renderer) {
	dst := dstRect(a.now(), a.texW, a.texH)
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)
	sdl.RenderTexture(renderer, a.texture, nil, &dst)
}
