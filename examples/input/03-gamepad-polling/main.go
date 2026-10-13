// Command 03-gamepad-polling polls the first gamepad once per frame and draws
// a visual representation of its state. It is a port of
// SDL/examples/input/03-gamepad-polling/gamepad-polling.c.
//
// Run it with:
//
//	go run ./examples/input/03-gamepad-polling [-frames N]
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// Window dimensions in pixels; they match the gamepad background image.
const (
	windowWidth  = 640
	windowHeight = 480
)

// gamepadPNG is the embedded gamepad background image drawn behind the
// button and axis overlays.
//
//go:embed gamepad_front.png
var gamepadPNG []byte

// init locks the main goroutine to its OS thread, as SDL's video and event
// functions must be called from the thread that initialised them.
func init() {
	runtime.LockOSThread()
}

// buttons holds where to draw each button, indexed by sdl.GamepadButton.
var buttons = [...]sdl.FRect{
	{X: 497, Y: 266, W: 38, H: 38}, // SOUTH
	{X: 550, Y: 217, W: 38, H: 38}, // EAST
	{X: 445, Y: 221, W: 38, H: 38}, // WEST
	{X: 499, Y: 173, W: 38, H: 38}, // NORTH
	{X: 235, Y: 228, W: 32, H: 29}, // BACK
	{X: 287, Y: 195, W: 69, H: 69}, // GUIDE
	{X: 377, Y: 228, W: 32, H: 29}, // START
	{X: 91, Y: 234, W: 63, H: 63},  // LEFT_STICK
	{X: 381, Y: 354, W: 63, H: 63}, // RIGHT_STICK
	{X: 74, Y: 73, W: 102, H: 29},  // LEFT_SHOULDER
	{X: 468, Y: 73, W: 102, H: 29}, // RIGHT_SHOULDER
	{X: 207, Y: 316, W: 32, H: 32}, // DPAD_UP
	{X: 207, Y: 384, W: 32, H: 32}, // DPAD_DOWN
	{X: 173, Y: 351, W: 32, H: 32}, // DPAD_LEFT
	{X: 242, Y: 351, W: 32, H: 32}, // DPAD_RIGHT
	{X: 310, Y: 286, W: 23, H: 27}, // MISC1
}

// main parses the -frames flag and runs the example, exiting with status 1
// and a message on stderr if run fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "03-gamepad-polling:", err)
		os.Exit(1)
	}
}

// abs16 returns the absolute value of v as an int, avoiding int16 overflow
// for math.MinInt16.
func abs16(v int16) int {
	if v < 0 {
		return -int(v)
	}
	return int(v)
}

// app is the example's state: the open gamepad, the texture and the time of
// the last thumbstick movement. It mirrors the C SDL_AppEvent/SDL_AppIterate
// split.
type app struct {
	gamepad        *sdl.Gamepad
	texture        *sdl.Texture
	leftThumbLast  uint64
	rightThumbLast uint64
	now            func() uint64
}

// newApp returns an app whose thumbstick-activity timers are unset. now
// supplies milliseconds.
func newApp(now func() uint64) *app {
	return &app{leftThumbLast: 0xFFFFFFFF, rightThumbLast: 0xFFFFFFFF, now: now}
}

// loadTexture decodes the embedded PNG into a texture of renderer r. The PNG
// is read through an in-memory IOStream, which LoadPNG_IO closes.
func (a *app) loadTexture(r *sdl.Renderer) error {
	src := sdl.IOFromBytes(gamepadPNG)
	if src == nil {
		return fmt.Errorf("SDL_IOFromBytes: %s", sdl.GetError())
	}
	surface := sdl.LoadPNG_IO(src, true)
	if surface == nil {
		return fmt.Errorf("couldn't load bitmap: %s", sdl.GetError())
	}
	a.texture = sdl.CreateTextureFromSurface(r, surface)
	sdl.DestroySurface(surface)
	if a.texture == nil {
		return fmt.Errorf("couldn't create static texture: %s", sdl.GetError())
	}
	return nil
}

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(event *sdl.Event) (quit bool) {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_GAMEPAD_ADDED:
		// Sent for each hotplugged gamepad, and for each already
		// connected one during Init.
		if a.gamepad == nil {
			which := event.Gdevice().Which
			a.gamepad = sdl.OpenGamepad(which)
			if a.gamepad == nil {
				fmt.Fprintf(os.Stderr, "Failed to open gamepad ID %d: %s\n", which, sdl.GetError())
			}
		}
	case sdl.EVENT_GAMEPAD_REMOVED:
		if a.gamepad != nil && sdl.GetGamepadID(a.gamepad) == event.Gdevice().Which {
			sdl.CloseGamepad(a.gamepad)
			a.gamepad = nil
		}
	}
	return false
}

// close releases the texture and the gamepad.
func (a *app) close() {
	if a.texture != nil {
		sdl.DestroyTexture(a.texture)
		a.texture = nil
	}
	if a.gamepad != nil {
		sdl.CloseGamepad(a.gamepad)
		a.gamepad = nil
	}
}

// draw renders one frame without presenting it.
func (a *app) draw(renderer *sdl.Renderer) {
	gamepad := a.gamepad
	text := "Plug in a gamepad, please."
	now := a.now()
	if gamepad != nil {
		text = sdl.GetGamepadName(gamepad)
	}

	sdl.SetRenderDrawColor(renderer, 0xFF, 0xFF, 0xFF, 0xFF)
	sdl.RenderClear(renderer)

	if gamepad != nil {
		sdl.RenderTexture(renderer, a.texture, nil, nil)

		// Green boxes over pressed buttons.
		sdl.SetRenderDrawColor(renderer, 0x00, 0xFF, 0x00, 0xFF)
		for i := range buttons {
			if sdl.GetGamepadButton(gamepad, sdl.GamepadButton(i)) {
				sdl.RenderFillRect(renderer, &buttons[i])
			}
		}

		// Axes in blue.
		sdl.SetRenderDrawColor(renderer, 0x00, 0x00, 0xFF, 0xFF)

		axisX := sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_LEFTX)
		axisY := sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_LEFTY)
		if abs16(axisX) > 1000 || abs16(axisY) > 1000 {
			a.leftThumbLast = now
		}
		if now-a.leftThumbLast < 500 {
			box := sdl.FRect{X: 107 + float32(axisX)/32767*30, Y: 252 + float32(axisY)/32767*30, W: 30, H: 30}
			sdl.RenderFillRect(renderer, &box)
		}

		axisX = sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_RIGHTX)
		axisY = sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_RIGHTY)
		if abs16(axisX) > 1000 || abs16(axisY) > 1000 {
			a.rightThumbLast = now
		}
		if now-a.rightThumbLast < 500 {
			box := sdl.FRect{X: 397 + float32(axisX)/32767*30, Y: 370 + float32(axisY)/32767*30, W: 30, H: 30}
			sdl.RenderFillRect(renderer, &box)
		}

		// Triggers: zero is unpressed, but may be a little off zero.
		if v := sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_LEFT_TRIGGER); v > 1000 {
			h := float32(v) / 32767 * 65
			box := sdl.FRect{X: 127, Y: 1 + (65 - h), W: 37, H: h}
			sdl.RenderFillRect(renderer, &box)
		}
		if v := sdl.GetGamepadAxis(gamepad, sdl.GAMEPAD_AXIS_RIGHT_TRIGGER); v > 1000 {
			h := float32(v) / 32767 * 65
			box := sdl.FRect{X: 481, Y: 1 + (65 - h), W: 37, H: h}
			sdl.RenderFillRect(renderer, &box)
		}
	}

	x := (float32(windowWidth) - float32(len(text)*sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)) / 2
	var y float32
	if gamepad != nil {
		y = float32(windowHeight - (sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE + 2))
	} else {
		y = (float32(windowHeight) - sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE) / 2
	}
	sdl.SetRenderDrawColor(renderer, 0x00, 0x00, 0xFF, 0xFF)
	sdl.RenderDebugText(renderer, x, y, text)
}

// run initialises SDL, creates the window and renderer, and drives the event
// and draw loop until the user quits or maxFrames frames have been presented
// (0 means no limit). It releases every SDL resource it created before
// returning.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Input Gamepad Polling", "1.0", "com.example.input-gamepad-polling")
	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMEPAD) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/input/gamepad-polling", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	if !sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_STRETCH) {
		return fmt.Errorf("SDL_SetRenderLogicalPresentation: %s", sdl.GetError())
	}

	a := newApp(sdl.GetTicks)
	defer a.close()
	if err := a.loadTexture(renderer); err != nil {
		return err
	}

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
