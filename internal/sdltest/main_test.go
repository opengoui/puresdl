package sdltest_test

import (
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestOnMainRunsOnMainThread(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	// IsMainThread is SDL's own judgement and is what the video drivers assert.
	if !sdltest.Call(t, sdl.IsMainThread) {
		t.Fatal("OnMain did not run on the SDL main thread")
	}
	if sdl.IsMainThread() {
		t.Error("test goroutine must not be the SDL main thread")
	}
}

func TestOnMainRepanicsOnTestGoroutine(t *testing.T) {
	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.Contains(msg, "boom") || !strings.Contains(msg, "main-thread stack") {
			t.Fatalf("recovered %v, want boom with main-thread stack", r)
		}
	}()
	sdltest.OnMain(t, func() { panic("boom") })
	t.Fatal("OnMain swallowed the panic")
}

func TestEnvSelectsDummyDrivers(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	if got := sdltest.Call(t, sdl.GetCurrentVideoDriver); got != "dummy" {
		t.Errorf("video driver = %q, want dummy", got)
	}
	if got := sdltest.Call(t, sdl.GetCurrentAudioDriver); got != "dummy" {
		t.Errorf("audio driver = %q, want dummy", got)
	}
}

func TestClockAdvance(t *testing.T) {
	var c sdltest.Clock
	if c.Ticks() != 0 {
		t.Fatal("zero clock must start at 0")
	}
	c.Advance(125)
	if got := c.Advance(25); got != 150 || c.Ticks() != 150 {
		t.Fatalf("Ticks = %d (Advance returned %d), want 150", c.Ticks(), got)
	}
}
