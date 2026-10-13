package sdl

import (
	"os"
	"path/filepath"
	"testing"
)

// loadedFile returns an absolute path of the library file that is loaded now.
func loadedFile(t *testing.T) string {
	t.Helper()
	if LoadError() != nil {
		t.Skip("SDL3 not loaded")
	}
	cands := append([]string{LoadedPath(), os.Getenv(LibraryEnv)}, libraryCandidates()...)
	for _, c := range cands {
		if c == "" || !filepath.IsAbs(c) {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	t.Skip("cannot locate the loaded library file to copy")
	return ""
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestLoadSwitchesLibrary loads a copy of the current library from another
// path and checks that the binding follows, then restores the original.
// The copy is a separate image, so it has its own SDL state.
func TestLoadSwitchesLibrary(t *testing.T) {
	orig := loadedFile(t)
	origName := LoadedPath()
	wantVersion := GetVersion()

	other := filepath.Join(t.TempDir(), filepath.Base(orig))
	copyFile(t, orig, other)
	t.Cleanup(func() {
		if err := Load(origName); err != nil {
			t.Errorf("restoring %q: %v", origName, err)
		}
	})

	getVersion := GetVersion // captured function value must follow the switch
	if err := Load(other); err != nil {
		t.Fatalf("Load(copy) = %v", err)
	}
	if got := LoadedPath(); got != other {
		t.Fatalf("LoadedPath = %q, want %q", got, other)
	}
	if LoadError() != nil {
		t.Fatalf("LoadError after switch = %v", LoadError())
	}
	if v := getVersion(); v != wantVersion || GetVersion() != wantVersion {
		t.Errorf("version after switch = %d / %d, want %d", v, GetVersion(), wantVersion)
	}
	if !Available("SDL_Init") || Available("SDL_NoSuchFunction") {
		t.Error("Available is inconsistent after switch")
	}
	if len(missing) != 0 {
		t.Errorf("missing symbols recorded after switching to a full library: %v", missing)
	}

	// Same path again: no-op.
	if err := Load(other); err != nil || LoadedPath() != other {
		t.Errorf("Load(same path) = %v, path %q", err, LoadedPath())
	}

	// Bad path: error, previous (the copy) keeps working.
	if err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("Load of a missing file succeeded")
	}
	if LoadedPath() != other || LoadError() != nil || GetVersion() != wantVersion {
		t.Errorf("failed switch disturbed the loaded library: %q, %v", LoadedPath(), LoadError())
	}

	// Back to the original.
	if err := Load(origName); err != nil || LoadedPath() != origName {
		t.Fatalf("switch back: %v, path %q", err, LoadedPath())
	}
	if GetVersion() != wantVersion {
		t.Error("version differs after switching back")
	}
}
