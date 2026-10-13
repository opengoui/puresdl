// Command gensdl generates the sdl package from the SDL3 C headers in a
// local directory. Use cmd/sdlsync to generate from a released version.
//
//	go run ./cmd/gensdl -include /path/to/SDL/include -out sdl
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/opengoui/puresdl/internal/gensdl"
)

// main parses the flags, validates the include directory and runs the generator.
func main() {
	include := flag.String("include", os.Getenv("SDL_INCLUDE"), "directory that contains SDL3/SDL.h (default $SDL_INCLUDE)")
	out := flag.String("out", "sdl", "output directory of the Go package")
	skipped := flag.String("skipped", "", "write the list of unbound C declarations to this file")
	flag.Parse()
	if *include == "" {
		log.Fatal("gensdl: -include or $SDL_INCLUDE is required")
	}
	if _, err := os.Stat(filepath.Join(*include, "SDL3", "SDL.h")); err != nil {
		log.Fatalf("gensdl: -include: no SDL3/SDL.h in %s", *include)
	}
	err := gensdl.Run(gensdl.Config{Include: *include, Out: *out, Skipped: *skipped, Stdout: os.Stdout, Stderr: os.Stderr})
	if err != nil {
		log.Fatal(err)
	}
}
