package sdl

import (
	"strings"
	"testing"
)

// TestOpenFirstMissingPath exercises the load failure path on the helper
// behind Load, so it runs whether or not the global library is loaded.
func TestOpenFirstMissingPath(t *testing.T) {
	h, err := openFirst([]string{"/nonexistent/libSDL3-missing", "/nonexistent/libSDL3-other"})
	if err == nil || h != 0 {
		t.Fatalf("openFirst of missing paths = %#x, %v", h, err)
	}
	if !strings.Contains(err.Error(), "cannot load SDL3") || !strings.Contains(err.Error(), "libSDL3-other") {
		t.Errorf("error %q lacks the failure summary or a candidate", err)
	}
}

func TestLoadAfterSuccessIsNoop(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	if err := Load("/nonexistent/libSDL3-missing"); err != nil {
		t.Errorf("Load after a successful load = %v, want nil", err)
	}
}
