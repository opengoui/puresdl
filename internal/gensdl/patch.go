package gensdl

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
)

// Patch is a hand-written, reviewable edit applied to one generated file after
// bindgen has written it. It is the escape hatch for changes that the
// bindgen rules cannot express; prefer fixing the rule in internal/bindgen.
// Generated files must never be edited by hand: the next run would overwrite
// the edit, and `sdlsync -check` would report drift.
type Patch struct {
	// Name identifies the patch in error messages and explains its purpose.
	Name string
	// File is the base name of the generated file to patch, for example
	// "video_gen.go".
	File string
	// Apply returns the patched source. It must be deterministic and should
	// fail when the text it expects is missing (see [Replace]), so that a
	// header update that moves the target is noticed instead of silently
	// skipped.
	Apply func(src []byte) ([]byte, error)
}

// patches lists the patches applied, in order, at the end of [Run]. It is
// empty: every generated file currently comes straight from bindgen.
var patches []Patch

// Replace returns a Patch function that replaces the single occurrence of old
// with new and fails unless old occurs exactly once.
func Replace(old, new string) func([]byte) ([]byte, error) {
	return func(src []byte) ([]byte, error) {
		if n := bytes.Count(src, []byte(old)); n != 1 {
			return nil, fmt.Errorf("expected exactly one occurrence of %q, found %d", old, n)
		}
		return bytes.Replace(src, []byte(old), []byte(new), 1), nil
	}
}

// applyPatches applies ps to the files in dir and re-formats each patched
// file with gofmt. A patch whose file was not generated is an error.
func applyPatches(dir string, ps []Patch) error {
	for _, p := range ps {
		path := filepath.Join(dir, p.File)
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("gensdl: patch %q: %w", p.Name, err)
		}
		out, err := p.Apply(src)
		if err != nil {
			return fmt.Errorf("gensdl: patch %q on %s: %w", p.Name, p.File, err)
		}
		if out, err = format.Source(out); err != nil {
			return fmt.Errorf("gensdl: patch %q on %s: result does not parse: %w", p.Name, p.File, err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return err
		}
	}
	return nil
}
