// Command 17-read-pixels draws a rotating texture, reads the rendered
// pixels back with RenderReadPixels, converts them to black and white (pure
// black becomes red), and draws the result in the top-left corner.
//
// It is a port of SDL/examples/renderer/17-read-pixels/read-pixels.c.
//
// Run it with:
//
//	go run ./examples/renderer/17-read-pixels [-frames N]
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
		fmt.Fprintln(os.Stderr, "17-read-pixels:", err)
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
	sdl.SetAppMetadata("Example Renderer Read Pixels", "1.0", "com.example.renderer-read-pixels")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/read-pixels", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
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
		if err := a.iterate(); err != nil {
			return err
		}
		sdl.RenderPresent(renderer)
	}
	return nil
}

// app is the per-program state shared by the event and frame callbacks.
type app struct {
	renderer      *sdl.Renderer
	now           func() uint64
	texture       *sdl.Texture
	textureWidth  int32
	textureHeight int32
	converted     *sdl.Texture
	convW, convH  int32
}

// newApp loads the embedded PNG into a texture owned by the returned app.
func newApp(renderer *sdl.Renderer, now func() uint64) (*app, error) {
	// IOFromBytes copies the data; LoadPNG_IO closes the stream.
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return nil, fmt.Errorf("couldn't open PNG data: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	a := &app{renderer: renderer, now: now, textureWidth: surface.W, textureHeight: surface.H}
	a.texture = sdl.CreateTextureFromSurface(renderer, surface)
	sdl.DestroySurface(surface)
	if a.texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return a, nil
}

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// rotation returns the texture angle in degrees for the tick count now.
func rotation(now uint64) float64 {
	return float64(int32(now%2000)) / 2000 * 360
}

// drawScene clears the target to black and draws the spinning texture.
func (a *app) drawScene() {
	renderer := a.renderer
	rot := rotation(a.now())

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	dst := sdl.FRect{
		X: float32(windowWidth-a.textureWidth) / 2,
		Y: float32(windowHeight-a.textureHeight) / 2,
		W: float32(a.textureWidth),
		H: float32(a.textureHeight),
	}
	center := sdl.FPoint{X: float32(a.textureWidth) / 2, Y: float32(a.textureHeight) / 2}
	sdl.RenderTextureRotated(renderer, a.texture, nil, &dst, rot, &center, sdl.FLIP_NONE)
}

// iterate draws one frame without presenting it.
func (a *app) iterate() error {
	renderer := a.renderer
	a.drawScene()

	// Very expensive: download the rendered pixels from the GPU.
	surface := sdl.RenderReadPixels(renderer, nil)
	if surface != nil && surface.Format != sdl.PIXELFORMAT_RGBA8888 && surface.Format != sdl.PIXELFORMAT_BGRA8888 {
		c := sdl.ConvertSurface(surface, sdl.PIXELFORMAT_RGBA8888)
		sdl.DestroySurface(surface)
		surface = c
	}

	if surface != nil {
		if surface.W != a.convW || surface.H != a.convH {
			if a.converted != nil {
				sdl.DestroyTexture(a.converted)
				a.converted = nil
			}
			a.converted = sdl.CreateTexture(renderer, sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_STREAMING, surface.W, surface.H)
			if a.converted == nil {
				sdl.DestroySurface(surface)
				return fmt.Errorf("couldn't (re)create conversion texture: %s", sdl.GetError())
			}
			a.convW, a.convH = surface.W, surface.H
		}

		// Turn each pixel into black or white; pure black becomes red.
		pixels := surface.PixelBytes()
		for y := int32(0); y < surface.H; y++ {
			row := pixels[int(y)*int(surface.Pitch):][:int(surface.W)*4]
			for x := 0; x < len(row); x += 4 {
				p := row[x : x+4 : x+4]
				avg := (uint32(p[1]) + uint32(p[2]) + uint32(p[3])) / 3
				if avg == 0 {
					p[0], p[3] = 0xFF, 0xFF
					p[1], p[2] = 0, 0
				} else {
					v := byte(0)
					if avg > 50 {
						v = 0xFF
					}
					p[1], p[2], p[3] = v, v, v
				}
			}
		}

		sdl.UpdateTextureBytes(a.converted, nil, pixels, surface.Pitch)
		sdl.DestroySurface(surface)

		dst := sdl.FRect{X: 0, Y: 0, W: windowWidth / 4, H: windowHeight / 4}
		sdl.RenderTexture(renderer, a.converted, nil, &dst)
	}
	return nil
}

// close releases the textures; the renderer is owned by the caller.
func (a *app) close() {
	if a.converted != nil {
		sdl.DestroyTexture(a.converted)
		a.converted = nil
	}
	if a.texture != nil {
		sdl.DestroyTexture(a.texture)
		a.texture = nil
	}
}
