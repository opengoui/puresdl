package sdltest_test

import (
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestVirtualGamepadButtonAxisAndRumble(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_GAMEPAD)
	pad := sdltest.AttachGamepad(t, "test pad")

	added := sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_GAMEPAD_ADDED)
	if len(added) != 1 || added[0].Gdevice().Which != pad.ID {
		t.Fatalf("EVENT_GAMEPAD_ADDED events = %d, want one for %d", len(added), pad.ID)
	}
	if !sdltest.Call(t, func() bool { return sdl.IsGamepad(pad.ID) }) {
		t.Fatal("virtual joystick of type gamepad is not recognized as a gamepad")
	}

	gp := sdltest.Call(t, func() *sdl.Gamepad { return sdl.OpenGamepad(pad.ID) })
	if gp == nil {
		t.Fatalf("OpenGamepad: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.CloseGamepad(gp) }) })

	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, true)
	pad.SetGamepadAxis(sdl.GAMEPAD_AXIS_LEFTX, 12000)
	ev := sdltest.Drain(t)
	down := sdltest.EventsOfType(ev, sdl.EVENT_GAMEPAD_BUTTON_DOWN)
	if len(down) != 1 || down[0].Gbutton().Button != uint8(sdl.GAMEPAD_BUTTON_SOUTH) || down[0].Gbutton().Which != pad.ID {
		t.Fatalf("button down events = %d, want one for SOUTH", len(down))
	}
	// SDL also reports the initial state of every axis; look at LEFTX only.
	var leftX []int16
	for _, e := range sdltest.EventsOfType(ev, sdl.EVENT_GAMEPAD_AXIS_MOTION) {
		if e.Gaxis().Axis == uint8(sdl.GAMEPAD_AXIS_LEFTX) {
			leftX = append(leftX, e.Gaxis().Value)
		}
	}
	if len(leftX) == 0 || leftX[len(leftX)-1] != 12000 {
		t.Fatalf("LEFTX axis values = %v, want to end with 12000", leftX)
	}
	if !sdltest.Call(t, func() bool { return sdl.GetGamepadButton(gp, sdl.GAMEPAD_BUTTON_SOUTH) }) {
		t.Error("polled button state is not down")
	}

	pad.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, false)
	if n := len(sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_GAMEPAD_BUTTON_UP)); n != 1 {
		t.Errorf("button up events = %d, want 1", n)
	}

	if !sdltest.Call(t, func() bool { return sdl.RumbleGamepad(gp, 1000, 2000, 50) }) {
		t.Fatalf("RumbleGamepad: %s", sdl.GetError())
	}
	if got := pad.Rumbles(); len(got) != 1 || got[0] != (sdltest.RumbleCall{Low: 1000, High: 2000}) {
		t.Fatalf("rumbles = %+v, want [{1000 2000}]", got)
	}
}

func TestVirtualJoystickHatAndRawEvents(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	js := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Axes: 2, Buttons: 3, Hats: 1})

	added := sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_JOYSTICK_ADDED)
	if len(added) != 1 || added[0].Jdevice().Which != js.ID {
		t.Fatalf("EVENT_JOYSTICK_ADDED events = %d", len(added))
	}
	if n := sdltest.Call(t, func() int32 { return sdl.GetNumJoystickHats(js.Joystick) }); n != 1 {
		t.Fatalf("hats = %d, want 1", n)
	}

	js.SetHat(0, sdl.HAT_UP)
	js.SetButton(2, true)
	js.SetAxis(1, -20000)
	ev := sdltest.Drain(t)
	if h := sdltest.EventsOfType(ev, sdl.EVENT_JOYSTICK_HAT_MOTION); len(h) != 1 || h[0].Jhat().Value != sdl.HAT_UP {
		t.Errorf("hat events = %d, want one HAT_UP", len(h))
	}
	if b := sdltest.EventsOfType(ev, sdl.EVENT_JOYSTICK_BUTTON_DOWN); len(b) != 1 || b[0].Jbutton().Button != 2 {
		t.Errorf("button events = %d, want one for button 2", len(b))
	}
	// SDL first reports the initial state of an axis (0), then the change.
	a := sdltest.EventsOfType(ev, sdl.EVENT_JOYSTICK_AXIS_MOTION)
	if len(a) == 0 || a[len(a)-1].Jaxis().Axis != 1 || a[len(a)-1].Jaxis().Value != -20000 {
		t.Errorf("last axis event = %+v, want axis 1 = -20000", a)
	}
}

func TestDetachedJoystickReportsRemoval(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	js := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Buttons: 1})
	sdltest.Drain(t)
	sdltest.OnMain(t, func() {
		sdl.CloseJoystick(js.Joystick)
		sdl.DetachVirtualJoystick(js.ID)
		js.Joystick = nil
	})
	if n := len(sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_JOYSTICK_REMOVED)); n != 1 {
		t.Fatalf("EVENT_JOYSTICK_REMOVED events = %d, want 1", n)
	}
}
