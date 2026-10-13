// Command 02-joystick-events looks for joystick input in the event loop
// and reports any changes as a flood of floating text messages.
// It is a port of SDL/examples/input/02-joystick-events/joystick-events.c.
//
// Joysticks are low-level interfaces: a bunch of buttons, axes and hats in no
// understood order. SDL's gamepad API sits on top of them for common devices.
//
// Run it with:
//
//	go run ./examples/input/02-joystick-events [-frames N]
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
	motionEventCooldown = 40     // ms between reported axis/ball motion events
	msgLifetime         = 3500.0 // ms a message lives for
)

// color is an RGBA colour with 8-bit components.
type color struct{ r, g, b, a uint8 }

// eventMessage is one floating line of text. The C version uses a linked
// list; a slice is the idiomatic equivalent (FIFO, oldest first).
type eventMessage struct {
	str        string
	color      color
	startTicks uint64
}

// app is the example's state: the window, the colour table, the queue of
// floating messages and the throttling timestamps for motion events. now is
// injectable so tests can control time.
type app struct {
	window   *sdl.Window
	renderer *sdl.Renderer
	colors   [64]color
	messages []*eventMessage

	axisCooldown uint64
	ballCooldown uint64

	now func() uint64 // time source in milliseconds; sdl.GetTicks in the real program
}

// newApp returns an app with the colour table and first hint message set up,
// mirroring the tail of SDL_AppInit.
func newApp(now func() uint64) *app {
	a := &app{now: now}
	a.colors[0] = color{255, 255, 255, 255}
	for i := 1; i < len(a.colors); i++ {
		a.colors[i] = color{uint8(sdl.Rand(255)), uint8(sdl.Rand(255)), uint8(sdl.Rand(255)), 255}
	}
	a.addMessage(0, "Please plug in a joystick.")
	return a
}

// hatStateString returns the name of the hat position state, or "UNKNOWN" for
// values that are not a defined hat position.
func hatStateString(state uint8) string {
	switch state {
	case sdl.HAT_CENTERED:
		return "CENTERED"
	case sdl.HAT_UP:
		return "UP"
	case sdl.HAT_RIGHT:
		return "RIGHT"
	case sdl.HAT_DOWN:
		return "DOWN"
	case sdl.HAT_LEFT:
		return "LEFT"
	case sdl.HAT_RIGHTUP:
		return "RIGHT+UP"
	case sdl.HAT_RIGHTDOWN:
		return "RIGHT+DOWN"
	case sdl.HAT_LEFTUP:
		return "LEFT+UP"
	case sdl.HAT_LEFTDOWN:
		return "LEFT+DOWN"
	}
	return "UNKNOWN"
}

// batteryStateString returns the name of a joystick power state, or "UNKNOWN"
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

// handleEvent returns false when the program should quit.
func (a *app) handleEvent(event *sdl.Event) bool {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return false
	case sdl.EVENT_JOYSTICK_ADDED:
		// Sent for each hotplugged stick, and for each already-connected one during Init.
		which := event.Jdevice().Which
		if joystick := sdl.OpenJoystick(which); joystick == nil {
			a.addMessage(which, "Joystick #%d add, but not opened: %s", which, sdl.GetError())
		} else {
			a.addMessage(which, "Joystick #%d ('%s') added", which, sdl.GetJoystickName(joystick))
		}
	case sdl.EVENT_JOYSTICK_REMOVED:
		which := event.Jdevice().Which
		if joystick := sdl.GetJoystickFromID(which); joystick != nil {
			sdl.CloseJoystick(joystick) // the joystick was unplugged.
		}
		a.addMessage(which, "Joystick #%d removed", which)
	case sdl.EVENT_JOYSTICK_AXIS_MOTION:
		// Spammy: only show every motionEventCooldown milliseconds.
		if now := a.now(); now >= a.axisCooldown {
			e := event.Jaxis()
			a.axisCooldown = now + motionEventCooldown
			a.addMessage(e.Which, "Joystick #%d axis %d -> %d", e.Which, e.Axis, e.Value)
		}
	case sdl.EVENT_JOYSTICK_BALL_MOTION:
		if now := a.now(); now >= a.ballCooldown {
			e := event.Jball()
			a.ballCooldown = now + motionEventCooldown
			a.addMessage(e.Which, "Joystick #%d ball %d -> %d, %d", e.Which, e.Ball, e.Xrel, e.Yrel)
		}
	case sdl.EVENT_JOYSTICK_HAT_MOTION:
		e := event.Jhat()
		a.addMessage(e.Which, "Joystick #%d hat %d -> %s", e.Which, e.Hat, hatStateString(e.Value))
	case sdl.EVENT_JOYSTICK_BUTTON_UP, sdl.EVENT_JOYSTICK_BUTTON_DOWN:
		e := event.Jbutton()
		state := "RELEASED"
		if e.Down {
			state = "PRESSED"
		}
		a.addMessage(e.Which, "Joystick #%d button %d -> %s", e.Which, e.Button, state)
	case sdl.EVENT_JOYSTICK_BATTERY_UPDATED:
		e := event.Jbattery()
		a.addMessage(e.Which, "Joystick #%d battery -> %s - %d%%", e.Which, batteryStateString(e.State), e.Percent)
	}
	return true
}

// iterate draws one frame at the current window size and presents it.
func (a *app) iterate() {
	var winw, winh int32 = 640, 480
	sdl.GetWindowSize(a.window, &winw, &winh)
	a.draw(a.renderer, winw, winh)
	sdl.RenderPresent(a.renderer)
}

// draw clears r and draws the live messages for a window of the given size.
// It does not present.
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
		if prevY != 0 && (prevY-y) < sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE {
			msg.startTicks = now
			break // wait for the previous message to tick up a little.
		}

		sdl.SetRenderDrawColor(r, msg.color.r, msg.color.g, msg.color.b,
			uint8(float32(msg.color.a)*(1-lifePercent)))
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
		fmt.Fprintln(os.Stderr, "02-joystick-events:", err)
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
	sdl.SetAppMetadata("Example Input Joystick Events", "1.0", "com.example.input-joystick-events")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	a := newApp(sdl.GetTicks)
	if !sdl.CreateWindowAndRenderer("examples/input/joystick-events", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &a.window, &a.renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(a.window)
	defer sdl.DestroyRenderer(a.renderer)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if !a.handleEvent(&event) {
				return nil
			}
		}
		a.iterate()
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}
