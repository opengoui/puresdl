package sdl

import (
	"runtime"
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

// nonSDLLibrary returns a system library that dlopens but is not SDL3.
func nonSDLLibrary() string {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/lib/libSystem.B.dylib"
	case "windows":
		return "kernel32.dll"
	default:
		return "libc.so.6"
	}
}

func TestLoadNonSDLLibraryKeepsPreviousLibrary(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	before, version := LoadedPath(), GetVersion()
	err := Load(nonSDLLibrary())
	if err == nil || !strings.Contains(err.Error(), "not an SDL3 library") {
		t.Fatalf("Load of a non-SDL library = %v", err)
	}
	if LoadError() != nil || LoadedPath() != before || GetVersion() != version {
		t.Errorf("previous library not intact: LoadError=%v path %q, version %d", LoadError(), LoadedPath(), GetVersion())
	}
	if !Available("SDL_Init") {
		t.Error("SDL_Init no longer available")
	}
}

func TestOpenFirstSkipsNonSDLLibrary(t *testing.T) {
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	_, _, err := openFirst([]string{nonSDLLibrary()})
	if err == nil {
		t.Fatal("openFirst accepted a non-SDL library")
	}
}
