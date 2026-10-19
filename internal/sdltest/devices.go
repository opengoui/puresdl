package sdltest

import (
	"sync"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/opengoui/puresdl/sdl"
)

// RumbleCall is one rumble request received by a virtual device.
type RumbleCall struct{ Low, High uint16 }

// JoystickSpec describes a virtual joystick. Zero counts mean none.
type JoystickSpec struct {
	Name                       string
	Axes, Buttons, Hats, Balls int
}

// VirtualJoystick is an attached virtual joystick that is also open, so its
// state can be driven. It is detached and closed by the test cleanup.
type VirtualJoystick struct {
	t        testing.TB
	ID       sdl.JoystickID
	Joystick *sdl.Joystick

	mu     sync.Mutex
	rumble []RumbleCall
}

// rumbleOnce guards the lazy creation of rumbleCB, the shared native callback.
var (
	rumbleOnce sync.Once
	rumbleCB   uintptr
)

// rumbleCallback returns the native rumble callback. The purego callback is
// never freed, so one shared callback is created per process and routes to the
// device through an sdl userdata handle.
func rumbleCallback() uintptr {
	rumbleOnce.Do(func() {
		// The C result is bool; it is returned as a uintptr because
		// syscall.NewCallback, which purego uses on Windows, accepts only
		// a uintptr-sized result.
		rumbleCB = purego.NewCallback(func(userdata unsafe.Pointer, low, high uint16) uintptr {
			if d, ok := sdl.UserdataHandleValue(userdata).(*VirtualJoystick); ok {
				d.mu.Lock()
				d.rumble = append(d.rumble, RumbleCall{low, high})
				d.mu.Unlock()
			}
			return 1
		})
	})
	return rumbleCB
}

// AttachJoystick attaches a virtual joystick (SDL_AttachVirtualJoystick) and
// opens it. SDL must be initialized with INIT_JOYSTICK. The device announces
// itself with EVENT_JOYSTICK_ADDED on the next pump, and its state changes
// become events on the pump after each Set call. On its first update SDL also
// reports the initial state (0) of each axis, so assert on the last event of
// an axis.
func AttachJoystick(t testing.TB, spec JoystickSpec) *VirtualJoystick {
	t.Helper()
	return attach(t, spec, sdl.JOYSTICK_TYPE_UNKNOWN, 0, 0)
}

// AttachGamepad attaches a virtual joystick of type gamepad that exposes the
// standard layout (all buttons, all axes), so SDL treats it as a gamepad with
// an automatic mapping and sends EVENT_GAMEPAD_* events for it. SDL must be
// initialized with INIT_GAMEPAD.
func AttachGamepad(t testing.TB, name string) *VirtualJoystick {
	t.Helper()
	buttons := uint32(1)<<uint(sdl.GAMEPAD_BUTTON_COUNT) - 1
	axes := uint32(1)<<uint(sdl.GAMEPAD_AXIS_COUNT) - 1
	spec := JoystickSpec{Name: name, Axes: int(sdl.GAMEPAD_AXIS_COUNT), Buttons: int(sdl.GAMEPAD_BUTTON_COUNT)}
	return attach(t, spec, sdl.JOYSTICK_TYPE_GAMEPAD, buttons, axes)
}

// attach creates a virtual joystick described by spec and registers its
// removal with t.Cleanup. buttonMask and axisMask select the gamepad controls
// that are reported as present.
func attach(t testing.TB, spec JoystickSpec, typ sdl.JoystickType, buttonMask, axisMask uint32) *VirtualJoystick {
	t.Helper()
	d := &VirtualJoystick{t: t}
	handle := sdl.NewUserdataHandle(d)

	name := spec.Name
	if name == "" {
		name = "sdltest virtual joystick"
	}
	desc := sdl.VirtualJoystickDesc{
		Type:       uint16(typ),
		Naxes:      uint16(spec.Axes),
		Nbuttons:   uint16(spec.Buttons),
		Nhats:      uint16(spec.Hats),
		Nballs:     uint16(spec.Balls),
		ButtonMask: buttonMask,
		AxisMask:   axisMask,
		Name:       sdl.CString(name),
		Userdata:   handle,
		Rumble:     rumbleCallback(),
	}
	desc.Version = uint32(unsafe.Sizeof(desc))

	var errMsg string
	OnMain(t, func() {
		d.ID = sdl.AttachVirtualJoystick(&desc)
		if d.ID == 0 {
			errMsg = sdl.GetError()
			return
		}
		d.Joystick = sdl.OpenJoystick(d.ID)
		if d.Joystick == nil {
			errMsg = sdl.GetError()
		}
	})
	if errMsg != "" {
		t.Fatalf("attach virtual joystick: %s", errMsg)
	}
	t.Cleanup(func() {
		OnMain(t, func() {
			sdl.CloseJoystick(d.Joystick)
			sdl.DetachVirtualJoystick(d.ID)
		})
		sdl.FreeUserdataHandle(handle)
	})
	return d
}

// set runs f on the main thread and fails the test with what and the SDL error
// when it reports failure.
func (d *VirtualJoystick) set(what string, f func() bool) {
	d.t.Helper()
	if !Call(d.t, f) {
		d.t.Fatalf("%s: %s", what, sdl.GetError())
	}
}

// SetButton presses or releases a raw joystick button.
func (d *VirtualJoystick) SetButton(button int32, down bool) {
	d.t.Helper()
	d.set("SetJoystickVirtualButton", func() bool { return sdl.SetJoystickVirtualButton(d.Joystick, button, down) })
}

// SetAxis sets a raw joystick axis value.
func (d *VirtualJoystick) SetAxis(axis int32, value int16) {
	d.t.Helper()
	d.set("SetJoystickVirtualAxis", func() bool { return sdl.SetJoystickVirtualAxis(d.Joystick, axis, value) })
}

// SetHat sets a hat position (sdl.HAT_*).
func (d *VirtualJoystick) SetHat(hat int32, value uint8) {
	d.t.Helper()
	d.set("SetJoystickVirtualHat", func() bool { return sdl.SetJoystickVirtualHat(d.Joystick, hat, value) })
}

// SetGamepadButton presses or releases a gamepad button; for a gamepad built
// with [AttachGamepad] the raw button index equals the sdl.GamepadButton value.
func (d *VirtualJoystick) SetGamepadButton(button sdl.GamepadButton, down bool) {
	d.t.Helper()
	d.SetButton(int32(button), down)
}

// SetGamepadAxis sets a gamepad axis; see [VirtualJoystick.SetGamepadButton].
func (d *VirtualJoystick) SetGamepadAxis(axis sdl.GamepadAxis, value int16) {
	d.t.Helper()
	d.SetAxis(int32(axis), value)
}

// Rumbles returns the rumble requests the device received so far, which is how
// a test observes sdl.RumbleJoystick and sdl.RumbleGamepad.
func (d *VirtualJoystick) Rumbles() []RumbleCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]RumbleCall(nil), d.rumble...)
}
