// Command 01-joystick-polling polls the state of the first joystick once
// per frame and draws a visual representation of its axes, buttons and hats.
// It is a port of SDL/examples/input/01-joystick-polling/joystick-polling.c.
//
// With no joystick attached it shows a "Plug in a joystick, please." prompt.
//
// Run it with:
//
//	go run ./examples/input/01-joystick-polling
package main

import (
	"flag"
	"fmt"
	"math"
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
		fmt.Fprintln(os.Stderr, "01-joystick-polling:", err)
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
	sdl.SetAppMetadata("Example Input Joystick Polling", "1.0", "com.example.input-joystick-polling")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/input/joystick-polling", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	g := newPoller()
	defer g.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if g.handleEvent(&event) {
				return nil
			}
		}
		var w, h int32 = 640, 480
		sdl.GetWindowSize(window, &w, &h)
		g.draw(renderer, w, h)
		sdl.RenderPresent(renderer)
	}
	return nil
}

// poller holds the state shared between events and frames: the first opened
// joystick and the per-index colours.
type poller struct {
	joystick *sdl.Joystick
	colors   [64]sdl.Color
}

// newPoller returns a poller with a random colour assigned to each joystick
// element index.
func newPoller() *poller {
	g := &poller{}
	for i := range g.colors {
		g.colors[i] = sdl.Color{R: uint8(sdl.Rand(255)), G: uint8(sdl.Rand(255)), B: uint8(sdl.Rand(255)), A: 255}
	}
	return g
}

// handleEvent mirrors SDL_AppEvent and reports whether the program should quit.
func (g *poller) handleEvent(event *sdl.Event) (quit bool) {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_JOYSTICK_ADDED:
		// Sent for each hotplugged stick and for each already-connected one at init.
		if g.joystick == nil {
			id := event.Jdevice().Which
			if g.joystick = sdl.OpenJoystick(id); g.joystick == nil {
				sdl.Log("Failed to open joystick ID %d: %s", uint32(id), sdl.GetError())
			}
		}
	case sdl.EVENT_JOYSTICK_REMOVED:
		if g.joystick != nil && sdl.GetJoystickID(g.joystick) == event.Jdevice().Which {
			sdl.CloseJoystick(g.joystick)
			g.joystick = nil
		}
	}
	return false
}

// close releases the joystick, if any.
func (g *poller) close() {
	if g.joystick != nil {
		sdl.CloseJoystick(g.joystick)
		g.joystick = nil
	}
}

// draw clears renderer and draws the joystick state for a w x h window.
// It does not present.
func (g *poller) draw(renderer *sdl.Renderer, w, h int32) {
	joystick, colors := g.joystick, &g.colors
	text := "Plug in a joystick, please."
	if joystick != nil {
		text = sdl.GetJoystickName(joystick)
	}

	sdl.SetRenderDrawColor(renderer, 0, 0, 0, 255)
	sdl.RenderClear(renderer)
	winw, winh := float32(w), float32(h)

	if joystick != nil {
		const size = 30.0

		// Axes as bars across the middle of the screen.
		total := sdl.GetNumJoystickAxes(joystick)
		y := (winh - float32(total)*size) / 2
		x := winw / 2
		for i := int32(0); i < total; i++ {
			c := colors[int(i)%len(colors)]
			val := float32(sdl.GetJoystickAxis(joystick, i)) / 32767.0
			dx := x + val*x
			dst := sdl.FRect{X: dx, Y: y, W: x - float32(math.Abs(float64(dx))), H: size}
			sdl.SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A)
			sdl.RenderFillRect(renderer, &dst)
			y += size
		}

		// Buttons as blocks across the top of the window.
		total = sdl.GetNumJoystickButtons(joystick)
		x = (winw - float32(total)*size) / 2
		for i := int32(0); i < total; i++ {
			c := colors[int(i)%len(colors)]
			dst := sdl.FRect{X: x, Y: 0, W: size, H: size}
			if sdl.GetJoystickButton(joystick, i) {
				sdl.SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A)
			} else {
				sdl.SetRenderDrawColor(renderer, 0, 0, 0, 255)
			}
			sdl.RenderFillRect(renderer, &dst)
			sdl.SetRenderDrawColor(renderer, 255, 255, 255, c.A)
			sdl.RenderRect(renderer, &dst)
			x += size
		}

		// Hats across the bottom of the screen.
		total = sdl.GetNumJoystickHats(joystick)
		x = (winw-float32(total)*(size*2))/2 + size/2
		y = winh - size
		for i := int32(0); i < total; i++ {
			c := colors[int(i)%len(colors)]
			third := float32(size) / 3
			cross := [2]sdl.FRect{
				{X: x, Y: y + third, W: size, H: third},
				{X: x + third, Y: y, W: third, H: size},
			}
			hat := sdl.GetJoystickHat(joystick, i)

			sdl.SetRenderDrawColor(renderer, 90, 90, 90, 255)
			sdl.RenderFillRectsSlice(renderer, cross[:])

			sdl.SetRenderDrawColor(renderer, c.R, c.G, c.B, c.A)
			if hat&sdl.HAT_UP != 0 {
				sdl.RenderFillRect(renderer, &sdl.FRect{X: x + third, Y: y, W: third, H: third})
			}
			if hat&sdl.HAT_RIGHT != 0 {
				sdl.RenderFillRect(renderer, &sdl.FRect{X: x + third*2, Y: y + third, W: third, H: third})
			}
			if hat&sdl.HAT_DOWN != 0 {
				sdl.RenderFillRect(renderer, &sdl.FRect{X: x + third, Y: y + third*2, W: third, H: third})
			}
			if hat&sdl.HAT_LEFT != 0 {
				sdl.RenderFillRect(renderer, &sdl.FRect{X: x, Y: y + third, W: third, H: third})
			}
			x += size * 2
		}
	}

	tx := (winw - float32(len(text))*sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE) / 2
	ty := (winh - sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE) / 2
	sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
	sdl.RenderDebugText(renderer, tx, ty, text)
}
