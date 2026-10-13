// Command 04-gamepad-events looks for gamepad input in the event handler
// and reports any changes as a flood of scrolling, fading messages.
// It's a port of SDL/examples/input/04-gamepad-events/gamepad-events.c.
//
// Run it with:
//
//	go run ./examples/input/04-gamepad-events [-frames N]
//
// Press Esc or close the window to quit.
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

// Timing constants, in milliseconds.
const (
	motionEventCooldown = 40     // milliseconds between reported axis events
	msgLifetime         = 3500.0 // milliseconds a message lives for
)

// eventMessage is one scrolling, fading line of text, coloured per gamepad and
// stamped with its creation time.
type eventMessage struct {
	str        string
	color      sdl.Color
	startTicks uint64
}

// app is the example's state: the colour table, the queue of messages and the
// throttle timestamp for axis events. now is injectable so tests can control
// time.
type app struct {
	now      func() uint64
	colors   [64]sdl.Color
	messages []*eventMessage

	axisMotionCooldownTime uint64
}

// newApp returns the initial state: random per-joystick colors (index 0 is
// white) and the "plug in a gamepad" hint. now supplies milliseconds.
func newApp(now func() uint64) *app {
	a := &app{now: now}
	a.colors[0] = sdl.Color{R: 255, G: 255, B: 255, A: 255}
	for i := 1; i < len(a.colors); i++ {
		a.colors[i] = sdl.Color{R: uint8(sdl.Rand(255)), G: uint8(sdl.Rand(255)), B: uint8(sdl.Rand(255)), A: 255}
	}
	a.addMessage(0, "Please plug in a gamepad.")
	return a
}

// batteryStateString returns the name of a gamepad power state, or "UNKNOWN"
// for values that are not defined.
func batteryStateString(state sdl.PowerState) string {
	switch state {
	case sdl.POWERSTATE_ERROR:
		return "ERROR"
	case sdl.POWERSTATE_UNKNOWN:
		return "UNKNOWN"
	case sdl.POWERSTATE_ON_BATTERY:
		return "ON BATTERY"
	case sdl.POWERSTATE_NO_BATTERY:
		return "NO BATTERY"
	case sdl.POWERSTATE_CHARGING:
		return "CHARGING"
	case sdl.POWERSTATE_CHARGED:
		return "CHARGED"
	}
	return "UNKNOWN"
}

// addMessage appends a formatted message coloured for joystick jid, stamped
// with the current time.
func (a *app) addMessage(jid sdl.JoystickID, format string, args ...any) {
	a.messages = append(a.messages, &eventMessage{
		str:        fmt.Sprintf(format, args...),
		color:      a.colors[int(jid)%len(a.colors)],
		startTicks: a.now(),
	})
}

// event handles one event; it reports false when the program should quit.
func (a *app) event(event *sdl.Event) bool {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return false
	case sdl.EVENT_KEY_DOWN:
		if event.Key().Key == sdl.SDLK_ESCAPE {
			return false
		}
	case sdl.EVENT_GAMEPAD_ADDED:
		// Sent for each hotplugged gamepad, and for each already-connected one during Init.
		which := event.Gdevice().Which
		gamepad := sdl.OpenGamepad(which)
		if gamepad == nil {
			a.addMessage(which, "Gamepad #%d add, but not opened: %s", which, sdl.GetError())
			break
		}
		a.addMessage(which, "Gamepad #%d ('%s') added", which, sdl.GetGamepadName(gamepad))
		if mapping := sdl.GetGamepadMapping(gamepad); mapping != "" {
			a.addMessage(which, "Gamepad #%d mapping: %s", which, mapping)
		}
	case sdl.EVENT_GAMEPAD_REMOVED:
		which := event.Gdevice().Which
		if gamepad := sdl.GetGamepadFromID(which); gamepad != nil {
			sdl.CloseGamepad(gamepad) // the gamepad was unplugged.
		}
		a.addMessage(which, "Gamepad #%d removed", which)
	case sdl.EVENT_GAMEPAD_AXIS_MOTION:
		// These are spammy, only show every motionEventCooldown milliseconds.
		now := a.now()
		if now >= a.axisMotionCooldownTime {
			e := event.Gaxis()
			a.axisMotionCooldownTime = now + motionEventCooldown
			a.addMessage(e.Which, "Gamepad #%d axis %s -> %d", e.Which, sdl.GetGamepadStringForAxis(sdl.GamepadAxis(e.Axis)), e.Value)
		}
	case sdl.EVENT_GAMEPAD_BUTTON_UP, sdl.EVENT_GAMEPAD_BUTTON_DOWN:
		e := event.Gbutton()
		state := "RELEASED"
		if e.Down {
			state = "PRESSED"
		}
		a.addMessage(e.Which, "Gamepad #%d button %s -> %s", e.Which, sdl.GetGamepadStringForButton(sdl.GamepadButton(e.Button)), state)
	case sdl.EVENT_JOYSTICK_BATTERY_UPDATED:
		e := event.Jbattery()
		// Only reported for joysticks, so make sure this one is really a gamepad.
		if sdl.IsGamepad(e.Which) {
			a.addMessage(e.Which, "Gamepad #%d battery -> %s - %d%%", e.Which, batteryStateString(e.State), e.Percent)
		}
	}
	return true
}

// draw clears r and draws the live messages for a winw x winh window without
// presenting.
func (a *app) draw(r *sdl.Renderer, winw, winh int32) {
	now := a.now()
	var prevY float32

	sdl.SetRenderDrawColor(r, 0, 0, 0, 255)
	sdl.RenderClear(r)

	for i := 0; i < len(a.messages); {
		msg := a.messages[i]
		lifePercent := float32(now-msg.startTicks) / msgLifetime
		if lifePercent >= 1 { // msg is done.
			a.messages = append(a.messages[:i], a.messages[i+1:]...)
			continue
		}
		x := (float32(winw) - float32(len(msg.str)*sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)) / 2
		y := float32(winh) * lifePercent
		if prevY != 0 && prevY-y < sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE {
			msg.startTicks = now
			break // wait for the previous message to tick up a little.
		}

		sdl.SetRenderDrawColor(r, msg.color.R, msg.color.G, msg.color.B, uint8(float32(msg.color.A)*(1-lifePercent)))
		sdl.RenderDebugText(r, x, y, msg.str)
		prevY = y
		i++
	}
}

// main parses the -frames flag and runs the example, exiting with status 1
// and a message on stderr if run fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "04-gamepad-events:", err)
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
	sdl.SetAppMetadata("Example Input Gamepad Events", "1.0", "com.example.input-gamepad-events")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMEPAD) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/input/gamepad-events", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	a := newApp(sdl.GetTicks)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if !a.event(&event) {
				return nil
			}
		}
		var winw, winh int32 = 640, 480
		sdl.GetWindowSize(window, &winw, &winh)
		a.draw(renderer, winw, winh)
		sdl.RenderPresent(renderer)
	}
	return nil
}
