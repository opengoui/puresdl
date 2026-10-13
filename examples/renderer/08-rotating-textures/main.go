// Command 08-rotating-textures draws a texture loaded from a PNG file,
// rotating around the center of the window.
//
// It is a port of SDL's examples/renderer/08-rotating-textures/rotating-textures.c
// (SDL/examples/renderer/08-rotating-textures/).
//
// Run it with:
//
//	go run ./examples/renderer/08-rotating-textures [-frames N]
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
		fmt.Fprintln(os.Stderr, "08-rotating-textures:", err)
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
	sdl.SetAppMetadata("Example Renderer Rotating Textures", "1.0", "com.example.renderer-rotating-textures")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/rotating-textures", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	d, err := newDemo(renderer, sdl.GetTicks)
	if err != nil {
		return err
	}
	defer d.close()

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

// demo is the program state: the uploaded texture and the time source.
type demo struct {
	texture       *sdl.Texture
	width, height float32
	now           func() uint64
}

// newDemo decodes the embedded PNG into a CPU surface, then uploads a copy to
// the GPU. The C code reads sample.png from disk; here it is read from memory.
func newDemo(r *sdl.Renderer, now func() uint64) (*demo, error) {
	src := sdl.IOFromBytes(samplePNG)
	if src == nil {
		return nil, fmt.Errorf("couldn't open PNG data: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(src, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	d := &demo{width: float32(surface.W), height: float32(surface.H), now: now}
	d.texture = sdl.CreateTextureFromSurface(r, surface)
	sdl.DestroySurface(surface)
	if d.texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return d, nil
}

// handleEvent reports whether the program should quit.
func (d *demo) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// rotationAt returns the angle in degrees for a tick count: one full turn
// every 2 seconds (2000 milliseconds).
func rotationAt(now uint64) float64 {
	return float64(int(now%2000)) / 2000 * 360
}

// draw clears to black and draws the centered, rotating texture. It does not
// present.
func (d *demo) draw(r *sdl.Renderer) {
	rotation := rotationAt(d.now())
	sdl.SetRenderDrawColor(r, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(r)

	// Center the texture and rotate it around its own center.
	dst := sdl.FRect{
		X: (windowWidth - d.width) / 2,
		Y: (windowHeight - d.height) / 2,
		W: d.width,
		H: d.height,
	}
	center := sdl.FPoint{X: d.width / 2, Y: d.height / 2}
	sdl.RenderTextureRotated(r, d.texture, nil, &dst, rotation, &center, sdl.FLIP_NONE)
}

// close destroys the texture and clears the reference.
func (d *demo) close() {
	sdl.DestroyTexture(d.texture)
	d.texture = nil
}
