// Command gamepad-rumble rumbles gamepads on button presses.
//
// It is a port of SDL/examples/input/05-gamepad-rumble/gamepad-rumble.c.
// Pass -frames N to exit after N frames; Esc or closing the window also quits.
//
// Run it with:
//
//	go run ./examples/input/05-gamepad-rumble
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

// gamepadInfo tracks one connected gamepad; id 0 marks an empty slot.
type gamepadInfo struct {
	id     sdl.JoystickID
	action string
}

// app is the per-run state: up to 16 tracked gamepads; any more are ignored.
type app struct {
	gamepads [16]gamepadInfo
}

// findGamepadInfo returns the tracked slot for the gamepad which, or nil if
// there is none. Looking up id 0 returns an empty slot.
func (a *app) findGamepadInfo(which sdl.JoystickID) *gamepadInfo {
	for i := range a.gamepads {
		if a.gamepads[i].id == which {
			return &a.gamepads[i]
		}
	}
	return nil
}

// main parses the -frames flag and runs the example, exiting with status 1
// and a message on stderr if run fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "gamepad-rumble:", err)
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
	sdl.SetAppMetadata("Example Input Gamepad Rumble", "1.0", "com.example.input-gamepad-rumble")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMEPAD) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/input/gamepad-rumble", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	a := &app{}
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

// handleEvent applies one event and reports whether the program should quit.
func (a *app) handleEvent(event *sdl.Event) (quit bool) {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_KEY_DOWN:
		if event.Key().Key == sdl.SDLK_ESCAPE {
			return true
		}
	case sdl.EVENT_GAMEPAD_ADDED:
		// Sent for each hotplugged gamepad and for each already-connected one at init.
		which := event.Gdevice().Which
		sdl.OpenGamepad(which)
		if info := a.findGamepadInfo(0); info != nil {
			info.id = which
			info.action = "idle"
		}
	case sdl.EVENT_GAMEPAD_REMOVED:
		which := event.Gdevice().Which
		if g := sdl.GetGamepadFromID(which); g != nil {
			sdl.CloseGamepad(g)
		}
		if info := a.findGamepadInfo(which); info != nil {
			info.id = 0
		}
	case sdl.EVENT_GAMEPAD_BUTTON_DOWN:
		b := event.Gbutton()
		gamepad := sdl.GetGamepadFromID(b.Which)
		info := a.findGamepadInfo(b.Which)
		switch sdl.GamepadButton(b.Button) {
		case sdl.GAMEPAD_BUTTON_SOUTH:
			sdl.RumbleGamepad(gamepad, 0xFFFF, 0x0000, 5000)
			if info != nil {
				info.action = "rumble high frequency"
			}
		case sdl.GAMEPAD_BUTTON_EAST:
			sdl.RumbleGamepad(gamepad, 0x0000, 0xFFFF, 5000)
			if info != nil {
				info.action = "rumble low frequency"
			}
		}
	case sdl.EVENT_GAMEPAD_BUTTON_UP:
		b := event.Gbutton()
		sdl.RumbleGamepad(sdl.GetGamepadFromID(b.Which), 0, 0, 0)
		if info := a.findGamepadInfo(b.Which); info != nil {
			info.action = "idle"
		}
	}
	return false
}

// drawCenteredText draws str horizontally centred in a render output rw pixels
// wide at *y, then advances *y by two character heights. An empty str only
// advances *y.
func drawCenteredText(renderer *sdl.Renderer, rw int32, y *int32, str string) {
	const cs = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	x := (rw - int32(len(str))*cs) / 2
	if str != "" {
		sdl.RenderDebugText(renderer, float32(x), float32(*y), str)
	}
	*y += cs * 2
}

// draw clears the renderer and draws the status text without presenting.
func (a *app) draw(renderer *sdl.Renderer) {
	const cs = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	var rw, rh int32
	sdl.GetCurrentRenderOutputSize(renderer, &rw, &rh)
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, 255)
	sdl.RenderClear(renderer)

	y := int32(cs * 8)
	sdl.SetRenderDrawColor(renderer, 255, 255, 0, 255)
	drawCenteredText(renderer, rw, &y, "Connect gamepads and press buttons to rumble.")
	y += cs * 3

	sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
	for i := range a.gamepads {
		which := a.gamepads[i].id
		if which == 0 {
			drawCenteredText(renderer, rw, &y, "")
		} else {
			drawCenteredText(renderer, rw, &y, fmt.Sprintf("%s: %s", sdl.GetGamepadNameForID(which), a.gamepads[i].action))
		}
	}
}
