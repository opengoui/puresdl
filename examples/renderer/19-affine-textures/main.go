// Command 19-affine-textures draws a 3D cube using only affine-transformed
// 2D texture rendering every frame.
//
// It is a port of SDL/examples/renderer/19-affine-textures/affine-textures.c.
// The C code reads sample.png from disk; here it is embedded and decoded from
// memory.
//
// Run it with:
//
//	go run ./examples/renderer/19-affine-textures [-frames N]
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
		fmt.Fprintln(os.Stderr, "affine-textures:", err)
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
	sdl.SetAppMetadata("Example Renderer Affine Textures", "1.0", "com.example.renderer-affine-textures")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/affine-textures", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	// Decode the embedded PNG into a CPU surface, then upload a copy to the GPU.
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

// app holds the texture and time source of the example.
type app struct {
	texture *sdl.Texture
	now     func() uint64
}

// newApp decodes the embedded PNG and uploads it as a texture of renderer.
func newApp(renderer *sdl.Renderer, now func() uint64) (*app, error) {
	src := sdl.IOFromBytes(samplePNG)
	if src == nil {
		return nil, fmt.Errorf("couldn't open PNG data: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(src, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	texture := sdl.CreateTextureFromSurface(renderer, surface)
	sdl.DestroySurface(surface)
	if texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return &app{texture: texture, now: now}, nil
}

// handleEvent reports whether the event asks the program to quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// close destroys the texture.
func (a *app) close() { sdl.DestroyTexture(a.texture) }

// draw renders one frame without presenting it.
func (a *app) draw(renderer *sdl.Renderer) {
	texture := a.texture
	const (
		x0 = 0.5 * windowWidth
		y0 = 0.5 * windowHeight
		px = float32(min(windowWidth, windowHeight) / 1.7320508075688772) // min(w, h) / sqrt(3)
	)

	now := a.now()
	rad := float32(int(now%2000)) / 2000 * math.Pi * 2
	cos := float32(math.Cos(float64(rad)))
	sin := float32(math.Sin(float64(rad)))
	n := float32(math.Sqrt(50))
	k := [3]float32{3 / n, 4 / n, 5 / n}
	mat := [9]float32{
		cos + (1-cos)*k[0]*k[0], -sin*k[2] + (1-cos)*k[0]*k[1], sin*k[1] + (1-cos)*k[0]*k[2],
		sin*k[2] + (1-cos)*k[0]*k[1], cos + (1-cos)*k[1]*k[1], -sin*k[0] + (1-cos)*k[1]*k[2],
		-sin*k[1] + (1-cos)*k[0]*k[2], sin*k[0] + (1-cos)*k[1]*k[2], cos + (1-cos)*k[2]*k[2],
	}

	var corners [16]float32
	for i := range 8 {
		x, y, z := float32(0.5), float32(0.5), float32(0.5)
		if i&1 != 0 {
			x = -0.5
		}
		if i&2 != 0 {
			y = -0.5
		}
		if i&4 != 0 {
			z = -0.5
		}
		corners[2*i] = mat[0]*x + mat[1]*y + mat[2]*z
		corners[2*i+1] = mat[3]*x + mat[4]*y + mat[5]*z
	}

	sdl.SetRenderDrawColor(renderer, 0x42, 0x87, 0xf5, sdl.ALPHA_OPAQUE) // light blue background.
	sdl.RenderClear(renderer)

	point := func(index int) sdl.FPoint {
		return sdl.FPoint{X: x0 + px*corners[2*index], Y: y0 + px*corners[2*index+1]}
	}
	for i := 1; i < 7; i++ {
		d := i
		if i&4 != 0 {
			d = ^i
		}
		dir := 3 & d
		odd := (i & 1) ^ ((i & 2) >> 1) ^ ((i & 4) >> 2)
		sign := float32(-1)
		if odd != 0 {
			sign = 1
		}
		if sign*mat[5+dir] > 0 {
			continue // back face
		}
		originIndex := 1 << ((dir - 1) % 3)
		rightIndex := (1 << ((dir + odd) % 3)) | originIndex
		downIndex := (1 << ((dir + (odd ^ 1)) % 3)) | originIndex
		if odd == 0 {
			originIndex ^= 7
			rightIndex ^= 7
			downIndex ^= 7
		}
		origin, right, down := point(originIndex), point(rightIndex), point(downIndex)
		sdl.RenderTextureAffine(renderer, texture, nil, &origin, &right, &down)
	}
}
