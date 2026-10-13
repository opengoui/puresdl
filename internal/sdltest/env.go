package sdltest

import (
	"os"
	"testing"

	"github.com/opengoui/puresdl/sdl"
)

// RequireEnv names the environment variable that turns a missing libSDL3 from
// a skip into a failure. Set it to a non-empty value in CI so the suite cannot
// pass vacuously on a machine without the library.
const RequireEnv = "PURESDL_REQUIRE_SDL"

// Skip skips the test when libSDL3 cannot be loaded (or fails it when
// [RequireEnv] is set) and otherwise re-applies
// [Env], so that code which initializes SDL itself (an example's run
// function) gets the dummy drivers even if a test changed the hints.
func Skip(t testing.TB) {
	t.Helper()
	if err := sdl.LoadError(); err != nil {
		if os.Getenv(RequireEnv) != "" {
			t.Fatalf("libSDL3 not available but %s is set: %v", RequireEnv, err)
		}
		t.Skipf("libSDL3 not available: %v", err)
	}
	OnMain(t, Env)
}

// Setup skips the test when libSDL3 is missing, initializes the
// given SDL subsystems on the main thread and registers a cleanup that
// quits SDL there. Use it for tests that drive SDL directly; tests of an
// example's run function should only call [Skip], because run initializes SDL
// itself.
func Setup(t testing.TB, flags sdl.InitFlags) {
	t.Helper()
	Skip(t)
	var initErr string
	OnMain(t, func() {
		if !sdl.Init(flags) {
			initErr = sdl.GetError()
		}
	})
	if initErr != "" {
		t.Fatalf("SDL_Init(%#x): %s", uint32(flags), initErr)
	}
	t.Cleanup(func() { OnMain(t, sdl.Quit) })
}
