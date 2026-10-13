package sdltest

import (
	"runtime"
	"testing"

	"github.com/opengoui/puresdl/sdl"
)

// Input injects synthetic input events into the SDL event queue with
// sdl.PushEvent, which SDL documents as safe to call from any goroutine. The
// events are delivered to whoever polls the queue next: the code under test,
// or [Pump] and [Drain].
//
// Injected events do not change polled device state (keyboard or mouse state
// arrays); only code that consumes events observes them. SDL must be
// initialized (see [Setup], or run the example's run function) before events
// are pushed, otherwise the queue is not available and the push is reported as
// a test error.
type Input struct {
	t      testing.TB
	window sdl.WindowID
}

// NewInput returns an injector whose window-bound events carry the given
// window ID (0 means no window).
func NewInput(t testing.TB, window sdl.WindowID) *Input {
	return &Input{t: t, window: window}
}

// push queues e, reporting an SDL error on t if the queue rejects it.
func (in *Input) push(e *sdl.Event) {
	in.t.Helper()
	if !sdl.PushEvent(e) {
		in.t.Errorf("sdl.PushEvent(%#x): %s", uint32(e.Type()), sdl.GetError())
	}
}

// newEvent returns a zeroed event of the given type.
func newEvent(typ sdl.EventType) *sdl.Event {
	e := new(sdl.Event)
	e.SetType(typ)
	return e
}

// Quit pushes EVENT_QUIT.
func (in *Input) Quit() {
	in.t.Helper()
	in.push(newEvent(sdl.EVENT_QUIT))
}

// CloseRequested pushes EVENT_WINDOW_CLOSE_REQUESTED for the input's window.
func (in *Input) CloseRequested() {
	in.t.Helper()
	e := newEvent(sdl.EVENT_WINDOW_CLOSE_REQUESTED)
	e.Window().WindowID = in.window
	in.push(e)
}

// Resized pushes EVENT_WINDOW_RESIZED and EVENT_WINDOW_PIXEL_SIZE_CHANGED for a
// new size. It does not resize a real window; see [ResizeWindow] for that.
func (in *Input) Resized(w, h int32) {
	in.t.Helper()
	for _, typ := range []sdl.EventType{sdl.EVENT_WINDOW_RESIZED, sdl.EVENT_WINDOW_PIXEL_SIZE_CHANGED} {
		e := newEvent(typ)
		we := e.Window()
		we.WindowID, we.Data1, we.Data2 = in.window, w, h
		in.push(e)
	}
}

// Key pushes a key down or up event with explicit scancode, keycode and modifiers.
func (in *Input) Key(down bool, sc sdl.Scancode, key sdl.Keycode, mod sdl.Keymod) {
	in.t.Helper()
	typ := sdl.EVENT_KEY_UP
	if down {
		typ = sdl.EVENT_KEY_DOWN
	}
	e := newEvent(typ)
	k := e.Key()
	k.WindowID, k.Scancode, k.Key, k.Mod, k.Down = in.window, sc, key, mod, down
	in.push(e)
}

// Press pushes a key down followed by a key up for sc, deriving the keycode
// from the active keyboard layout like SDL does (video must be initialized).
func (in *Input) Press(sc sdl.Scancode) {
	in.t.Helper()
	key := sdl.GetKeyFromScancode(sc, 0, false)
	in.Key(true, sc, key, 0)
	in.Key(false, sc, key, 0)
}

// Text pushes EVENT_TEXT_INPUT carrying s.
func (in *Input) Text(s string) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_TEXT_INPUT)
	e.Text().WindowID = in.window
	e.Text().Text = sdl.CString(s)
	in.push(e) // SDL copies the string while queueing.
	runtime.KeepAlive(e)
}

// MouseMotion pushes a motion event at (x, y) with relative motion (xrel, yrel).
func (in *Input) MouseMotion(x, y, xrel, yrel float32, buttons sdl.MouseButtonFlags) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_MOUSE_MOTION)
	m := e.Motion()
	m.WindowID, m.State, m.X, m.Y, m.Xrel, m.Yrel = in.window, buttons, x, y, xrel, yrel
	in.push(e)
}

// MouseButton pushes a button press or release (button is sdl.BUTTON_LEFT etc.).
func (in *Input) MouseButton(button uint8, down bool, x, y float32) {
	in.t.Helper()
	typ := sdl.EVENT_MOUSE_BUTTON_UP
	if down {
		typ = sdl.EVENT_MOUSE_BUTTON_DOWN
	}
	e := newEvent(typ)
	b := e.Button()
	b.WindowID, b.Button, b.Down, b.Clicks, b.X, b.Y = in.window, button, down, 1, x, y
	in.push(e)
}

// MouseWheel pushes a wheel event; positive y scrolls away from the user.
func (in *Input) MouseWheel(x, y float32) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_MOUSE_WHEEL)
	w := e.Wheel()
	w.WindowID, w.X, w.Y = in.window, x, y
	w.IntegerX, w.IntegerY = int32(x), int32(y)
	in.push(e)
}

// Drop pushes EVENT_DROP_FILE for the given path.
func (in *Input) Drop(path string) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_DROP_FILE)
	e.Drop().WindowID = in.window
	e.Drop().Data = sdl.CString(path)
	in.push(e)
	runtime.KeepAlive(e)
}

// ClipboardUpdate pushes EVENT_CLIPBOARD_UPDATE (the clipboard content itself
// is changed with sdl.SetClipboardText).
func (in *Input) ClipboardUpdate() {
	in.t.Helper()
	in.push(newEvent(sdl.EVENT_CLIPBOARD_UPDATE))
}

// PenProximity pushes EVENT_PEN_PROXIMITY_IN or _OUT.
func (in *Input) PenProximity(pen sdl.PenID, entering bool) {
	in.t.Helper()
	typ := sdl.EVENT_PEN_PROXIMITY_OUT
	if entering {
		typ = sdl.EVENT_PEN_PROXIMITY_IN
	}
	e := newEvent(typ)
	e.Pproximity().WindowID, e.Pproximity().Which = in.window, pen
	in.push(e)
}

// PenMotion pushes EVENT_PEN_MOTION; state carries sdl.PEN_INPUT_* flags.
func (in *Input) PenMotion(pen sdl.PenID, x, y float32, state sdl.PenInputFlags) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_PEN_MOTION)
	p := e.Pmotion()
	p.WindowID, p.Which, p.X, p.Y, p.PenState = in.window, pen, x, y, state
	in.push(e)
}

// PenTouch pushes EVENT_PEN_DOWN or EVENT_PEN_UP.
func (in *Input) PenTouch(pen sdl.PenID, down, eraser bool, x, y float32) {
	in.t.Helper()
	typ := sdl.EVENT_PEN_UP
	state := sdl.PenInputFlags(0)
	if down {
		typ, state = sdl.EVENT_PEN_DOWN, sdl.PEN_INPUT_DOWN
	}
	e := newEvent(typ)
	p := e.Ptouch()
	p.WindowID, p.Which, p.X, p.Y, p.Down, p.Eraser, p.PenState = in.window, pen, x, y, down, eraser, state
	in.push(e)
}

// PenButton pushes EVENT_PEN_BUTTON_DOWN or _UP.
func (in *Input) PenButton(pen sdl.PenID, button uint8, down bool, x, y float32) {
	in.t.Helper()
	typ := sdl.EVENT_PEN_BUTTON_UP
	if down {
		typ = sdl.EVENT_PEN_BUTTON_DOWN
	}
	e := newEvent(typ)
	p := e.Pbutton()
	p.WindowID, p.Which, p.Button, p.Down, p.X, p.Y = in.window, pen, button, down, x, y
	in.push(e)
}

// PenAxis pushes EVENT_PEN_AXIS (for example pressure).
func (in *Input) PenAxis(pen sdl.PenID, axis sdl.PenAxis, value, x, y float32) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_PEN_AXIS)
	p := e.Paxis()
	p.WindowID, p.Which, p.Axis, p.Value, p.X, p.Y = in.window, pen, axis, value, x, y
	in.push(e)
}

// GamepadButton pushes a gamepad button press or release event for the device
// instance. To drive a virtual device end to end use [VirtualJoystick] instead.
func (in *Input) GamepadButton(id sdl.JoystickID, button sdl.GamepadButton, down bool) {
	in.t.Helper()
	typ := sdl.EVENT_GAMEPAD_BUTTON_UP
	if down {
		typ = sdl.EVENT_GAMEPAD_BUTTON_DOWN
	}
	e := newEvent(typ)
	g := e.Gbutton()
	g.Which, g.Button, g.Down = id, uint8(button), down
	in.push(e)
}

// GamepadAxis pushes a gamepad axis motion event.
func (in *Input) GamepadAxis(id sdl.JoystickID, axis sdl.GamepadAxis, value int16) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_GAMEPAD_AXIS_MOTION)
	g := e.Gaxis()
	g.Which, g.Axis, g.Value = id, uint8(axis), value
	in.push(e)
}

// JoyButton pushes a raw joystick button event.
func (in *Input) JoyButton(id sdl.JoystickID, button uint8, down bool) {
	in.t.Helper()
	typ := sdl.EVENT_JOYSTICK_BUTTON_UP
	if down {
		typ = sdl.EVENT_JOYSTICK_BUTTON_DOWN
	}
	e := newEvent(typ)
	j := e.Jbutton()
	j.Which, j.Button, j.Down = id, button, down
	in.push(e)
}

// JoyAxis pushes a raw joystick axis motion event.
func (in *Input) JoyAxis(id sdl.JoystickID, axis uint8, value int16) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_JOYSTICK_AXIS_MOTION)
	j := e.Jaxis()
	j.Which, j.Axis, j.Value = id, axis, value
	in.push(e)
}

// JoyHat pushes a raw joystick hat event (value is sdl.HAT_*).
func (in *Input) JoyHat(id sdl.JoystickID, hat, value uint8) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_JOYSTICK_HAT_MOTION)
	j := e.Jhat()
	j.Which, j.Hat, j.Value = id, hat, value
	in.push(e)
}

// User pushes a user event of type sdl.EVENT_USER with the given code.
func (in *Input) User(code int32) {
	in.t.Helper()
	e := newEvent(sdl.EVENT_USER)
	e.User().Code = code
	in.push(e)
}

// Pump runs sdl.PumpEvents on the main thread, which makes SDL generate
// events from device state (virtual joysticks, window changes).
func Pump(t testing.TB) {
	t.Helper()
	OnMain(t, sdl.PumpEvents)
}

// Drain pumps and then removes every queued event, returning them in order.
// Pointer fields inside the returned events (text, drop paths) are only valid
// until the next poll and should not be read from the returned slice.
func Drain(t testing.TB) []sdl.Event {
	t.Helper()
	return Call(t, func() []sdl.Event {
		sdl.PumpEvents()
		var out []sdl.Event
		var e sdl.Event
		for sdl.PollEvent(&e) {
			out = append(out, e)
		}
		return out
	})
}

// EventsOfType returns the events whose type is typ.
func EventsOfType(events []sdl.Event, typ sdl.EventType) []sdl.Event {
	var out []sdl.Event
	for _, e := range events {
		if e.Type() == typ {
			out = append(out, e)
		}
	}
	return out
}
