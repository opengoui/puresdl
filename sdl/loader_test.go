package sdl

import (
	"strings"
	"testing"
)

// TestOpenFirstMissingPath exercises the load failure path on the helper
// behind Load, so it runs whether or not the global library is loaded.
func TestOpenFirstMissingPath(t *testing.T) {
	h, _, err := openFirst([]string{"/nonexistent/libSDL3-missing", "/nonexistent/libSDL3-other"})
	if err == nil || h != 0 {
		t.Fatalf("openFirst of missing paths = %#x, %v", h, err)
	}
	if !strings.Contains(err.Error(), "cannot load SDL3") || !strings.Contains(err.Error(), "libSDL3-other") {
		t.Errorf("error %q lacks the failure summary or a candidate", err)
	}
}

func TestLoadEmptyPathAfterSuccessIsNoop(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	before := LoadedPath()
	if err := Load(""); err != nil || LoadedPath() != before {
		t.Errorf("Load(\"\") after a successful load = %v, path %q -> %q", err, before, LoadedPath())
	}
}

func TestLoadBadPathKeepsPreviousLibrary(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	before := LoadedPath()
	if err := Load("/nonexistent/libSDL3-missing"); err == nil {
		t.Fatal("Load of a missing path succeeded")
	}
	if LoadError() != nil || LoadedPath() != before {
		t.Errorf("after failed switch: LoadError=%v path %q, want nil and %q", LoadError(), LoadedPath(), before)
	}
	if GetVersion() == 0 {
		t.Error("previous library no longer works")
	}
}

func TestLoadedPathNames(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	if LoadedPath() == "" {
		t.Error("LoadedPath is empty although a library is loaded")
	}
}
