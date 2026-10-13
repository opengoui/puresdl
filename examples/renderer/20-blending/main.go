// Command 20-blending ports SDL/examples/renderer/20-blending/blending.c.
//
// Run it from the module root with:
//
//	go run ./examples/renderer/20-blending [-frames N]
//
// It shows texture blending with the built-in blend modes and a custom
// "screen" blend mode built with ComposeCustomBlendMode. UP/DOWN change the
// alpha modulation of the three overlapping colored squares in each panel.
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

// Window dimensions and the layout of the blend-mode grid, in logical pixels.
const (
	windowWidth  = 640
	windowHeight = 480

	rows = 2
	cols = 3

	gridSize    = float32((windowWidth - 1) / 18.0)
	panelSize   = gridSize * 4
	rowOffset   = (windowHeight - rows*panelSize) / 4
	colOffset   = gridSize * cols
	rectSize    = float32(50)
	redOffset   = gridSize
	greenOffset = rectSize/3 + gridSize
	blueOffset  = rectSize*2/3 + gridSize
)

// blendModeNames labels the grid panels, in row-major order.
var blendModeNames = [rows * cols]string{"NONE", "BLEND", "ADD", "MOD", "MUL", "SCREEN \"CUSTOM\""}

// main parses the -frames flag, runs the example, and exits with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "20-blending:", err)
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
	sdl.SetAppMetadata("Example Blending", "1.0", "com.example.blending")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/renderer/blending", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	g, err := newGame(renderer)
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

// game is the example state: panel layout, blend modes, textures and the
// current alpha modulation.
type game struct {
	panels     [rows * cols]sdl.FRect
	blendModes [rows * cols]sdl.BlendMode
	textures   [3]*sdl.Texture // red, green, blue
	alpha      uint8
}

// newGame builds the panels, the custom screen blend mode and the three
// colored textures on r.
func newGame(r *sdl.Renderer) (*game, error) {
	sdl.SetRenderLogicalPresentation(r, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)
	g := &game{alpha: 255}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			g.panels[col+row*cols] = sdl.FRect{
				X: float32(col)*panelSize + float32(col)*colOffset,
				Y: float32(row)*panelSize + float32(row+1)*rowOffset,
				W: panelSize, H: panelSize,
			}
		}
	}
	g.blendModes = [rows * cols]sdl.BlendMode{
		sdl.BLENDMODE_NONE, sdl.BLENDMODE_BLEND, sdl.BLENDMODE_ADD, sdl.BLENDMODE_MOD, sdl.BLENDMODE_MUL,
		// Screen blending: dstRGB := 1 - (1 - dstRGB) * (1 - srcRGB).
		sdl.ComposeCustomBlendMode(
			sdl.BLENDFACTOR_ONE_MINUS_DST_COLOR, sdl.BLENDFACTOR_ONE, sdl.BLENDOPERATION_ADD,
			sdl.BLENDFACTOR_ZERO, sdl.BLENDFACTOR_ONE, sdl.BLENDOPERATION_ADD),
	}

	surface := sdl.CreateSurface(int32(rectSize), int32(rectSize), sdl.PIXELFORMAT_RGBA8888)
	if surface == nil {
		return nil, fmt.Errorf("couldn't create surface: %s", sdl.GetError())
	}
	defer sdl.DestroySurface(surface)
	for i, color := range [3]uint32{0xFF0000FF, 0x00FF00FF, 0x0000FFFF} {
		sdl.FillSurfaceRect(surface, nil, color)
		g.textures[i] = sdl.CreateTextureFromSurface(r, surface)
		if g.textures[i] == nil {
			err := fmt.Errorf("couldn't create texture: %s", sdl.GetError())
			g.close()
			return nil, err
		}
	}
	return g, nil
}

// handleEvent applies one event and reports whether the app should quit.
func (g *game) handleEvent(e *sdl.Event) (quit bool) {
	switch sdl.EventType(e.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_KEY_DOWN:
		key := e.Key().Key
		if key == sdl.SDLK_UP && g.alpha <= 255-8 {
			g.alpha += 8
		}
		if key == sdl.SDLK_DOWN && g.alpha >= 8 {
			g.alpha -= 8
		}
	}
	return false
}

// close destroys the textures.
func (g *game) close() {
	for i, t := range g.textures {
		if t != nil {
			sdl.DestroyTexture(t)
			g.textures[i] = nil
		}
	}
}

// draw renders one frame without presenting it.
func (g *game) draw(renderer *sdl.Renderer) {
	panels, blendModes, textures, alpha := &g.panels, &g.blendModes, &g.textures, g.alpha
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)

	// Checkerboard panels.
	for i := range panels {
		p := panels[i]
		for y := p.Y; y < panelSize+p.Y; y += gridSize {
			for x := p.X; x < panelSize+p.X; x += gridSize {
				grid := sdl.FRect{X: x, Y: y, W: gridSize, H: gridSize}
				if int(x/gridSize+y/gridSize)%2 != 0 {
					sdl.SetRenderDrawColor(renderer, 70, 70, 70, 255)
				} else {
					sdl.SetRenderDrawColor(renderer, 110, 110, 110, 255)
				}
				sdl.RenderFillRect(renderer, &grid)
			}
		}
		// Label the blend mode.
		sdl.SetRenderDrawColor(renderer, 255, 255, 255, sdl.ALPHA_OPAQUE)
		sdl.RenderDebugText(renderer, p.X, p.Y-15, blendModeNames[i])
	}

	// Panel outlines and UI text.
	sdl.RenderRectsSlice(renderer, panels[:])
	sdl.RenderDebugText(renderer, (windowWidth-176)/2, windowHeight-30, "UP/DOWN: CHANGE ALPHA")
	sdl.RenderDebugTextFormat(renderer, (windowWidth-80)/2, windowHeight-20, "ALPHA: %d", alpha)

	for _, t := range textures {
		sdl.SetTextureAlphaMod(t, alpha)
	}

	offsets := [3]float32{redOffset, greenOffset, blueOffset}
	for i := range panels {
		p := panels[i]
		supported := true
		for j, t := range textures {
			// Only the first result matters: it tells whether the renderer
			// supports the mode.
			ok := sdl.SetTextureBlendMode(t, blendModes[i])
			if j == 0 {
				supported = ok
			}
		}
		for j, t := range textures {
			dst := sdl.FRect{X: p.X + offsets[j], Y: p.Y + offsets[j], W: rectSize, H: rectSize}
			sdl.RenderTexture(renderer, t, nil, &dst)
		}
		if !supported {
			const textWidth = 104.0
			dst := sdl.FRect{X: p.X + (p.W-textWidth)/2, Y: p.Y + (p.H - 8), W: textWidth, H: 8}
			sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
			sdl.RenderFillRect(renderer, &dst)
			sdl.SetRenderDrawColor(renderer, 255, 255, 255, sdl.ALPHA_OPAQUE)
			sdl.RenderDebugText(renderer, dst.X, dst.Y, "[UNSUPPORTED]")
		}
	}

}
