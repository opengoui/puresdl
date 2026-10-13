// Command 06-textures loads a PNG into a texture and draws it a few times per
// frame, moving two of the copies around.
//
// It is a port of SDL/examples/renderer/06-textures/textures.c. The PNG is
// embedded and decoded from memory instead of read from the base path.
//
// Run it with:
//
//	go run ./examples/renderer/06-textures [-frames N]
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
		fmt.Fprintln(os.Stderr, "06-textures:", err)
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
	sdl.SetAppMetadata("Example Renderer Textures", "1.0", "com.example.renderer-textures")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/textures", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	setLogicalPresentation(renderer)

	// Decode the PNG into a CPU-side surface, then upload it to a texture.
	scene, err := newScene(renderer, sdl.GetTicks)
	if err != nil {
		return err
	}
	defer scene.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if scene.handleEvent(&event) {
				return nil
			}
		}
		scene.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// scene holds the texture and the time source that drive the animation.
type scene struct {
	texture    *sdl.Texture
	texW, texH float32
	now        func() uint64
}

// newScene decodes the embedded PNG and uploads it to a texture on r.
func newScene(r *sdl.Renderer, now func() uint64) (*scene, error) {
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return nil, fmt.Errorf("couldn't open png memory: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load png: %s", sdl.GetError())
	}
	defer sdl.DestroySurface(surface)
	texture := sdl.CreateTextureFromSurface(r, surface)
	if texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return &scene{texture: texture, texW: float32(surface.W), texH: float32(surface.H), now: now}, nil
}

// close destroys the texture.
func (s *scene) close() { sdl.DestroyTexture(s.texture) }

// handleEvent reports whether the event asks the program to quit.
func (s *scene) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// placements returns the three destination rectangles for time now (ms).
func (s *scene) placements(now uint64) [3]sdl.FRect {
	// Textures move around over a few seconds.
	direction := float32(-1)
	if now%2000 >= 1000 {
		direction = 1
	}
	scale := float32(int(now%1000)-500) / 500 * direction
	return [3]sdl.FRect{
		{X: 100 * scale, Y: 0, W: s.texW, H: s.texH},
		{X: (windowWidth - s.texW) / 2, Y: (windowHeight - s.texH) / 2, W: s.texW, H: s.texH},
		{X: (windowWidth - s.texW) - 100*scale, Y: windowHeight - s.texH, W: s.texW, H: s.texH},
	}
}

// draw clears r and stamps the texture three times; it does not present.
func (s *scene) draw(r *sdl.Renderer) {
	sdl.SetRenderDrawColor(r, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(r)
	for _, dst := range s.placements(s.now()) {
		sdl.RenderTexture(r, s.texture, nil, &dst)
	}
}

// setLogicalPresentation makes the renderer scale a windowWidth x windowHeight
// logical canvas into the window, letterboxed.
func setLogicalPresentation(r *sdl.Renderer) bool {
	return sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}
