// Command 01-drawing-lines reads pen/stylus input and draws lines, darker for
// harder pressure. It is a port of
// SDL/examples/pen/01-drawing-lines/drawing-lines.c.
//
// SDL can track multiple pens, but for simplicity this assumes any pen input
// comes from one device. Without a pen it just shows the gray canvas.
//
// Run it with:
//
//	go run ./examples/pen/01-drawing-lines [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to its OS thread, as SDL's video and event
// functions must be called from the thread that initialised them.
func init() {
	runtime.LockOSThread()
}

// main parses the -frames flag and runs the example, exiting with status 1
// and a message on stderr if run fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "01-drawing-lines:", err)
		os.Exit(1)
	}
}

// run initialises SDL, creates the window and renderer, and drives the event
// and draw loop until the user quits or maxFrames frames have been presented
// (0 means no limit). It releases every SDL resource it created before
// returning.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Pen Drawing Lines", "1.0", "com.example.pen-drawing-lines")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/pen/drawing-lines", 640, 480, sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	app, err := newApp(renderer)
	if err != nil {
		return err
	}
	defer app.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if app.handleEvent(&event) {
				return nil
			}
		}
		app.draw()
		sdl.RenderPresent(renderer)
	}
	return nil
}

// app holds the drawing state, mirroring the C program's globals.
type app struct {
	renderer     *sdl.Renderer
	target       *sdl.Texture
	pressure     float32
	prevX, prevY float32
	tiltX, tiltY float32
}

// newApp creates the render target (matching the output size, for hidpi) and
// blanks it to gray.
func newApp(renderer *sdl.Renderer) (*app, error) {
	var w, h int32
	sdl.GetRenderOutputSize(renderer, &w, &h)
	target := sdl.CreateTexture(renderer, sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_TARGET, w, h)
	if target == nil {
		return nil, fmt.Errorf("couldn't create render target: %s", sdl.GetError())
	}
	sdl.SetRenderTarget(renderer, target)
	sdl.SetRenderDrawColor(renderer, 100, 100, 100, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)
	sdl.SetRenderTarget(renderer, nil)
	sdl.SetRenderDrawBlendMode(renderer, sdl.BLENDMODE_BLEND)
	return &app{renderer: renderer, target: target, prevX: -1, prevY: -1}, nil
}

// close releases the render target.
func (a *app) close() { sdl.DestroyTexture(a.target) }

// handleEvent applies one event and reports whether the program should quit.
func (a *app) handleEvent(event *sdl.Event) (quit bool) {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_PEN_MOTION:
		m := event.Pmotion()
		// If pressure > 0 the pen is definitely touching.
		if a.pressure > 0 {
			if a.prevX >= 0 { // only draw while moving and touching
				// Alpha is the pressure: lighter presses give fainter lines.
				sdl.SetRenderTarget(a.renderer, a.target)
				sdl.SetRenderDrawColorFloat(a.renderer, 0, 0, 0, a.pressure)
				sdl.RenderLine(a.renderer, a.prevX, a.prevY, m.X, m.Y)
			}
			a.prevX, a.prevY = m.X, m.Y
		} else {
			a.prevX, a.prevY = -1, -1
		}
	case sdl.EVENT_PEN_AXIS:
		p := event.Paxis()
		switch p.Axis {
		case sdl.PEN_AXIS_PRESSURE:
			a.pressure = p.Value
		case sdl.PEN_AXIS_XTILT:
			a.tiltX = p.Value
		case sdl.PEN_AXIS_YTILT:
			a.tiltY = p.Value
		}
	}
	return false
}

// draw renders one frame (without presenting): the stroke texture and the
// tilt readout.
func (a *app) draw() {
	// Make sure we're drawing to the window and not the render target.
	sdl.SetRenderTarget(a.renderer, nil)
	sdl.SetRenderDrawColor(a.renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(a.renderer)
	sdl.RenderTexture(a.renderer, a.target, nil, nil)
	sdl.RenderDebugText(a.renderer, 0, 8, fmt.Sprintf("Tilt: %f %f", a.tiltX, a.tiltY))
}
