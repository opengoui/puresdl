// Command gensdl generates the sdl package from the SDL3 C headers.
//
//	go run ./cmd/gensdl -include /path/to/SDL/include -out sdl
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/opengoui/puresdl/internal/bindgen"
	"github.com/opengoui/puresdl/internal/ccfront"
)

func main() {
	include := flag.String("include", os.Getenv("SDL_INCLUDE"), "directory that contains SDL3/SDL.h (default $SDL_INCLUDE)")
	out := flag.String("out", "sdl", "output directory of the Go package")
	skipped := flag.String("skipped", "", "write the list of unbound C declarations to this file")
	flag.Parse()
	if *include == "" {
		log.Fatal("gensdl: -include or $SDL_INCLUDE is required")
	}
	api, warnings, err := ccfront.Load(ccfront.Options{IncludeDir: *include})
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	// remove stale generated files
	old, _ := filepath.Glob(filepath.Join(*out, "*_gen.go"))
	olt, _ := filepath.Glob(filepath.Join(*out, "*_gen_test.go"))
	for _, f := range append(old, olt...) {
		os.Remove(f)
	}
	res, err := bindgen.Generate(api, bindgen.Options{OutDir: *out})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("SDL %s: wrote %d files, %d declarations not bound, %d typed constants\n", api.Version, len(res.Files), len(res.Skipped), res.TypedConsts)
	if *skipped != "" {
		var b []byte
		for _, s := range res.Skipped {
			b = append(b, s...)
			b = append(b, '\n')
		}
		if err := os.WriteFile(*skipped, b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
