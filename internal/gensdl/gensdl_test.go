package gensdl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/sdlsync"
)

// TestCheckedInBindingsAreCurrent regenerates the bindings from the headers of
// the release in SDL_VERSION into a temp dir and requires them to equal the
// checked-in sdl/*_gen*.go and docs/skipped.txt (the same check as
// `sdlsync -check`), and the output to be free of local paths.
func TestCheckedInBindingsAreCurrent(t *testing.T) {
	inc := os.Getenv("SDL_INCLUDE")
	if inc == "" {
		var err error
		if inc, err = sdlsync.PinnedInclude("../.."); err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("pinned SDL headers unavailable: %v", err)
			}
			t.Skipf("pinned SDL headers unavailable: %v", err)
		}
	}
	tagBytes, err := os.ReadFile("../../SDL_VERSION")
	if err != nil {
		t.Fatal(err)
	}
	tag, err := sdlsync.ParseTag(string(tagBytes), false)
	if err != nil {
		t.Fatal(err)
	}
	// The revision is part of the file header; take it from the checked-in
	// file so that the test also works with a bare SDL_INCLUDE.
	head, err := os.ReadFile("../../sdl/error_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	revision := ""
	if i := strings.Index(string(head), ", revision "); i >= 0 {
		rest := string(head)[i+len(", revision "):]
		revision, _, _ = strings.Cut(rest, ")")
	}

	out := t.TempDir()
	skipped := filepath.Join(out, "skipped.txt")
	err = Run(Config{Include: inc, Out: out, HandDir: "../../sdl", Skipped: skipped, Tag: tag.Name, Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := sdlsync.Compare(out, skipped, "../../sdl", "../../docs/skipped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) > 0 {
		t.Errorf("generated code differs from the checked-in files (run `go generate ./sdl`):\n  %s", strings.Join(diffs, "\n  "))
	}
	// determinism: nothing may depend on where the headers live
	files, _ := filepath.Glob(filepath.Join(out, "*_gen*.go"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), inc) {
			t.Errorf("%s contains the header path %s", filepath.Base(f), inc)
		}
	}
}
