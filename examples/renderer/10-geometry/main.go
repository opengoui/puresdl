// Command 10-geometry draws arbitrary polygons made of triangles every
// frame, including textured and indexed geometry.
//
// It is a port of SDL/examples/renderer/10-geometry/geometry.c. The PNG the C
// program loads from disk is embedded instead.
//
// Run it with:
//
//	go run ./examples/renderer/10-geometry [-frames N]
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
		fmt.Fprintln(os.Stderr, "10-geometry:", err)
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
	sdl.SetAppMetadata("Example Renderer Geometry", "1.0", "com.example.renderer-geometry")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/geometry", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	g := newScene(sdl.GetTicks)
	if err := g.loadTexture(renderer); err != nil {
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

// scene holds the state of the example: the texture and the time source.
type scene struct {
	texture *sdl.Texture
	now     func() uint64
}

// newScene returns a scene that reads the current time from now. The texture
// is loaded separately by loadTexture.
func newScene(now func() uint64) *scene { return &scene{now: now} }

// loadTexture decodes the embedded PNG and uploads it as a texture of r.
func (g *scene) loadTexture(renderer *sdl.Renderer) error {
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return fmt.Errorf("couldn't open embedded png: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	g.texture = sdl.CreateTextureFromSurface(renderer, surface)
	sdl.DestroySurface(surface)
	if g.texture == nil {
		return fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return nil
}

// handleEvent reports whether the event asks the program to quit.
func (g *scene) handleEvent(event *sdl.Event) (quit bool) {
	return sdl.EventType(event.Type()) == sdl.EVENT_QUIT
}

// close releases the texture.
func (g *scene) close() {
	if g.texture != nil {
		sdl.DestroyTexture(g.texture)
		g.texture = nil
	}
}

// triangleSize returns the edge size of the animated triangle at time now (ms).
func triangleSize(now uint64) float32 {
	direction := float32(-1)
	if now%2000 >= 1000 {
		direction = 1
	}
	scale := (float32(int(now%1000)-500) / 500) * direction
	return 200 + 200*scale
}

// white returns opaque white, leaving texture colors unmodulated.
func white() sdl.FColor { return sdl.FColor{R: 1, G: 1, B: 1, A: 1} }

// draw clears the renderer and draws the geometry; it does not present.
func (g *scene) draw(renderer *sdl.Renderer) {
	texture := g.texture
	size := triangleSize(g.now())

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// A single centered triangle with a different color at each vertex.
	vertices := [4]sdl.Vertex{
		{Position: sdl.FPoint{X: windowWidth / 2, Y: (windowHeight - size) / 2}, Color: sdl.FColor{R: 1, A: 1}},
		{Position: sdl.FPoint{X: (windowWidth + size) / 2, Y: (windowHeight + size) / 2}, Color: sdl.FColor{G: 1, A: 1}},
		{Position: sdl.FPoint{X: (windowWidth - size) / 2, Y: (windowHeight + size) / 2}, Color: sdl.FColor{B: 1, A: 1}},
	}
	sdl.RenderGeometrySlice(renderer, nil, vertices[:3], nil)

	// Map a texture to the geometry; texture coordinates run from 0 to 1.
	vertices = [4]sdl.Vertex{
		{Position: sdl.FPoint{X: 10, Y: 10}, Color: white(), TexCoord: sdl.FPoint{X: 0, Y: 0}},
		{Position: sdl.FPoint{X: 150, Y: 10}, Color: white(), TexCoord: sdl.FPoint{X: 1, Y: 0}},
		{Position: sdl.FPoint{X: 10, Y: 150}, Color: white(), TexCoord: sdl.FPoint{X: 0, Y: 1}},
	}
	sdl.RenderGeometrySlice(renderer, texture, vertices[:3], nil)

	// Two triangles sharing vertices through indices cover the whole texture.
	for i := 0; i < 3; i++ {
		vertices[i].Position.X += 450
	}
	vertices[3] = sdl.Vertex{Position: sdl.FPoint{X: 600, Y: 150}, Color: white(), TexCoord: sdl.FPoint{X: 1, Y: 1}}
	indices := [...]int32{0, 1, 2, 1, 2, 3}
	sdl.RenderGeometrySlice(renderer, texture, vertices[:], indices[:])
}
