package sdltest

import (
	"testing"

	"github.com/opengoui/puresdl/sdl"
)

// NewWindow creates a hidden window of the given size on the dummy video
// driver (SDL must be initialized, see [Setup]) and destroys it on cleanup.
func NewWindow(t testing.TB, w, h int32) *sdl.Window {
	t.Helper()
	win := Call(t, func() *sdl.Window { return sdl.CreateWindow(t.Name(), w, h, sdl.WINDOW_HIDDEN|sdl.WINDOW_RESIZABLE) })
	if win == nil {
		t.Fatalf("SDL_CreateWindow: %s", sdl.GetError())
	}
	t.Cleanup(func() { OnMain(t, func() { sdl.DestroyWindow(win) }) })
	return win
}

// WindowID returns the SDL ID of win, for [NewInput].
func WindowID(t testing.TB, win *sdl.Window) sdl.WindowID {
	t.Helper()
	return Call(t, func() sdl.WindowID { return sdl.GetWindowID(win) })
}

// ResizeWindow resizes a real window and waits until the change is applied, so
// the resize events SDL generates are in the queue afterwards.
func ResizeWindow(t testing.TB, win *sdl.Window, w, h int32) {
	t.Helper()
	ok := Call(t, func() bool { return sdl.SetWindowSize(win, w, h) && sdl.SyncWindow(win) })
	if !ok {
		t.Fatalf("resize window to %dx%d: %s", w, h, sdl.GetError())
	}
}
