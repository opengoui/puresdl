package sdl

import "testing"

// Compile-time checks: each constant must have the typedef the API expects.
var (
	_ InitFlags        = INIT_VIDEO | INIT_EVENTS
	_ WindowFlags      = WINDOW_RESIZABLE | WINDOW_HIDDEN | WINDOW_HIGH_PIXEL_DENSITY
	_ MouseButtonFlags = BUTTON_LMASK | BUTTON_RMASK
	_ Keymod           = KMOD_LSHIFT | KMOD_LCTRL
	_ Keycode          = SDLK_A
	_ BlendMode        = BLENDMODE_BLEND
	_ SurfaceFlags     = SURFACE_PREALLOCATED
	_ MouseID          = TOUCH_MOUSEID
	_ TouchID          = MOUSE_TOUCHID
	_ AudioDeviceID    = AUDIO_DEVICE_DEFAULT_PLAYBACK
)

// Untyped on purpose: window positions are int coordinates, not flags.
var _ int32 = WINDOWPOS_CENTERED

func TestTypedFlagConstants(t *testing.T) {
	if !Init(INIT_VIDEO) {
		t.Fatalf("Init: %s", GetError())
	}
	defer Quit()
	if WasInit(INIT_VIDEO)&INIT_VIDEO == 0 {
		t.Errorf("WasInit(INIT_VIDEO) = %#x", WasInit(INIT_VIDEO))
	}
	w := CreateWindow("typed", 64, 48, WINDOW_HIDDEN|WINDOW_RESIZABLE)
	if w == nil {
		t.Fatalf("CreateWindow: %s", GetError())
	}
	defer DestroyWindow(w)
	flags := GetWindowFlags(w)
	if flags&WINDOW_HIDDEN == 0 || flags&WINDOW_RESIZABLE == 0 || flags&WINDOW_FULLSCREEN != 0 {
		t.Errorf("GetWindowFlags = %#x", uint64(flags))
	}
	if got := GetModState() & (KMOD_CTRL | KMOD_ALT); got != KMOD_NONE {
		t.Errorf("GetModState = %#x, want no modifiers", uint16(got))
	}
	if BUTTON_MASK(BUTTON_X2) != BUTTON_X2MASK {
		t.Error("BUTTON_MASK(BUTTON_X2) != BUTTON_X2MASK")
	}
	s := CreateSurface(2, 2, PIXELFORMAT_RGBA32)
	if s == nil {
		t.Fatalf("CreateSurface: %s", GetError())
	}
	defer DestroySurface(s)
	var mode BlendMode
	SetSurfaceBlendMode(s, BLENDMODE_ADD)
	if !GetSurfaceBlendMode(s, &mode) || mode != BLENDMODE_ADD {
		t.Errorf("blend mode = %#x, want %#x", uint32(mode), uint32(BLENDMODE_ADD))
	}
}
