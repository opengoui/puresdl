// Package gensdl runs the binding generation pipeline: parse the SDL3
// headers with ccfront and write the Go package with bindgen. It is shared by
// cmd/gensdl and cmd/sdlsync.
package gensdl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opengoui/puresdl/internal/bindgen"
	"github.com/opengoui/puresdl/internal/ccfront"
)

// Config describes one generation run.
type Config struct {
	// Include is the directory that contains SDL3/SDL.h.
	Include string
	// Out is the output directory of the Go package.
	Out string
	// HandDir is the directory whose hand-written Go files name extra
	// identifiers that docs may refer to; default Out.
	HandDir string
	// Skipped, when not empty, is the file that receives the list of unbound
	// C declarations.
	Skipped string
	// Tag and Revision identify the release the headers come from; they are
	// recorded in the generated file header. Both are optional.
	Tag, Revision string
	// Stdout and Stderr receive the summary line and the warnings; nil
	// discards them.
	Stdout, Stderr io.Writer
}

// Run executes the pipeline. Stale *_gen.go and *_gen_test.go files in
// Config.Out are removed first. The registered [Patch] list is applied to the
// freshly generated files before Run returns, so `sdlsync -check` compares
// patched output.
func Run(c Config) error {
	if c.Include == "" {
		return fmt.Errorf("gensdl: include directory is required")
	}
	if c.Out == "" {
		c.Out = "sdl"
	}
	stdout, stderr := c.Stdout, c.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	api, warnings, err := ccfront.Load(ccfront.Options{IncludeDir: c.Include})
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	old, _ := filepath.Glob(filepath.Join(c.Out, "*_gen.go"))
	olt, _ := filepath.Glob(filepath.Join(c.Out, "*_gen_test.go"))
	for _, f := range append(old, olt...) {
		os.Remove(f)
	}
	res, err := bindgen.Generate(api, bindgen.Options{OutDir: c.Out, HandDir: c.HandDir, Tag: c.Tag, Revision: c.Revision})
	if err != nil {
		return err
	}
	if err := applyPatches(c.Out, patches); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "SDL %s: wrote %d files, %d declarations not bound, %d typed constants\n", api.Version, len(res.Files), len(res.Skipped), res.TypedConsts)
	if c.Skipped != "" {
		var b []byte
		for _, s := range res.Skipped {
			b = append(b, s...)
			b = append(b, '\n')
		}
		if err := os.WriteFile(c.Skipped, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
