//go:build !windows

package sdl

import (
	"slices"
	"testing"
)

func TestLibraryCandidates(t *testing.T) {
	c := libraryCandidates()
	if len(c) == 0 {
		t.Fatal("no library candidates")
	}
	for i, p := range c {
		if p == "" || slices.Contains(c[:i], p) {
			t.Errorf("candidate %d %q is empty or duplicated", i, p)
		}
	}
	if !slices.Contains(c, "/usr/local/lib/libSDL3.dylib") && !slices.Contains(c, "libSDL3.so.0") {
		t.Errorf("candidates %q lack the platform's standard names", c)
	}
}

func TestLinuxLibDirs(t *testing.T) {
	dirs := linuxLibDirs()
	for _, want := range []string{"/usr/local/lib", "/usr/lib"} {
		if !slices.Contains(dirs, want) {
			t.Errorf("linuxLibDirs() = %q, missing %q", dirs, want)
		}
	}
}
