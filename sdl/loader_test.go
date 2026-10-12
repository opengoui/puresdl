package sdl

import (
	"errors"
	"testing"
)

func TestLoadExplicitMissingPath(t *testing.T) {
	if LoadError() == nil {
		t.Skip("library already loaded; Load is a no-op")
	}
	err := Load("/nonexistent/libSDL3-missing")
	if err == nil {
		t.Fatal("Load of a missing path succeeded")
	}
	if errors.Is(LoadError(), ErrNotLoaded) {
		t.Errorf("LoadError() = %v, want the load failure", LoadError())
	}
}
