// Command 15-cliprect stretches a texture across the window while a
// sliding clipping rectangle reveals only a small square of it.
//
// It is a port of SDL/examples/renderer/15-cliprect/cliprect.c. The PNG is
// embedded and decoded from memory instead of read from the base path.
//
// Run it with:
//
//	go run ./examples/renderer/15-cliprect [-frames N]
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

// Window dimensions in logical pixels, plus the clipping rectangle's edge
// length and its speed in pixels per second.
const (
	windowWidth   = 640
	windowHeight  = 480
	cliprectSize  = 250
	cliprectSpeed = 200 // pixels per second
)

//go:embed sample.png
var samplePNG []byte

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "15-cliprect:", err)
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
	sdl.SetAppMetadata("Example Renderer Clipping Rectangle", "1.0", "com.example.renderer-cliprect")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/cliprect", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	texture, err := loadTexture(renderer)
	if err != nil {
		return err
	}
	defer sdl.DestroyTexture(texture)

	g := newDemo(sdl.GetTicks)
	g.texture = texture

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if g.handleEvent(&event) {
				return nil
			}
		}
		g.advance()
		g.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// loadTexture decodes the embedded sample PNG into a texture of r.
func loadTexture(r *sdl.Renderer) (*sdl.Texture, error) {
	io := sdl.IOFromBytes(samplePNG)
	if io == nil {
		return nil, fmt.Errorf("couldn't open embedded png: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(io, true)
	if surface == nil {
		return nil, fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	defer sdl.DestroySurface(surface)
	texture := sdl.CreateTextureFromSurface(r, surface)
	if texture == nil {
		return nil, fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return texture, nil
}

// demo is the per-frame state, mirroring the C SDL_AppEvent/SDL_AppIterate
// split.
type demo struct {
	texture  *sdl.Texture
	posX     float32
	posY     float32
	dirX     float32
	dirY     float32
	clip     sdl.Rect // clipping rectangle for the next draw
	lastTime uint64
	now      func() uint64
}

// newDemo returns a demo whose clipping rectangle starts moving diagonally
// from the origin; time is read from now.
func newDemo(now func() uint64) *demo {
	return &demo{dirX: 1, dirY: 1, lastTime: now(), now: now}
}

// handleEvent reports whether the program should quit.
func (g *demo) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// advance moves the clipping rectangle by the time elapsed since the last
// call. The rectangle used for drawing is the position before the move.
func (g *demo) advance() {
	g.clip = sdl.Rect{X: int32(math.Round(float64(g.posX))), Y: int32(math.Round(float64(g.posY))), W: cliprectSize, H: cliprectSize}
	now := g.now()
	elapsed := float32(now-g.lastTime) / 1000
	distance := elapsed * cliprectSpeed

	// Slide the clipping rectangle, bouncing off the window edges.
	g.posX += distance * g.dirX
	if g.posX < -cliprectSize {
		g.posX, g.dirX = -cliprectSize, 1
	} else if g.posX >= windowWidth {
		g.posX, g.dirX = windowWidth-1, -1
	}
	g.posY += distance * g.dirY
	if g.posY < -cliprectSize {
		g.posY, g.dirY = -cliprectSize, 1
	} else if g.posY >= windowHeight {
		g.posY, g.dirY = windowHeight-1, -1
	}
	g.lastTime = now
}

// draw clears and draws the texture through the clipping rectangle; it does
// not present.
func (g *demo) draw(r *sdl.Renderer) {
	sdl.SetRenderClipRect(r, &g.clip)
	// RenderClear is not affected by the clipping rectangle.
	sdl.SetRenderDrawColor(r, 33, 33, 33, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(r)
	// Only the piece inside the clipping rectangle is actually drawn.
	sdl.RenderTexture(r, g.texture, nil, nil)
}
