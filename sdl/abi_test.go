package sdl

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// pinnedVersion reads the release pinned in ../SDL_VERSION as an SDL
// version number.
func pinnedVersion(t *testing.T) (num int, tag string) {
	t.Helper()
	b, err := os.ReadFile("../SDL_VERSION")
	if err != nil {
		t.Fatal(err)
	}
	tag = strings.TrimSpace(string(b))
	var x, y, z int
	if _, err := fmt.Sscanf(tag, "release-%d.%d.%d", &x, &y, &z); err != nil {
		t.Fatalf("SDL_VERSION %q: %v", tag, err)
	}
	return x*1000000 + y*1000 + z, tag
}

// TestBindingsMatchPinnedRelease catches drift between the headers the
// bindings were generated from and the library they run against: when the
// loaded SDL3 is exactly the pinned release, every bound symbol must exist.
// (SDL's dynamic API table exports every function on every platform, with
// stubs for the platform-specific ones, so there are no exceptions.)
func TestBindingsMatchPinnedRelease(t *testing.T) {
	if err := LoadError(); err != nil {
		t.Skipf("SDL3 not loaded: %v", err)
	}
	want, tag := pinnedVersion(t)
	if got := GetVersion(); int(got) != want {
		t.Skipf("loaded SDL3 is %d.%d.%d but the bindings are generated from %s; ABI check needs the matching library",
			VERSIONNUM_MAJOR(int(got)), VERSIONNUM_MINOR(int(got)), VERSIONNUM_MICRO(int(got)), tag)
	}
	syms := allBindings()
	if len(syms) < 1000 {
		t.Fatalf("only %d bound symbols", len(syms))
	}
	var missing []string
	for _, s := range syms {
		if !Available(s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d bound symbols are missing from the loaded SDL3 %s: %s", len(missing), len(syms), tag, strings.Join(missing, ", "))
	}
}
