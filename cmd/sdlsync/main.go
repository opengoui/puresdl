// Command sdlsync pins the SDL release whose headers puresdl is generated
// from (the SDL_VERSION file), downloads and caches those headers, and can
// regenerate or check the generated bindings.
//
//	go run ./cmd/sdlsync -latest -write -generate   # bump to the newest release
//	go run ./cmd/sdlsync -generate                  # regenerate for SDL_VERSION
//	go run ./cmd/sdlsync -check                     # CI: fail on drift
//	export SDL_INCLUDE=$(go run ./cmd/sdlsync -print-include)
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/opengoui/puresdl/internal/gensdl"
	"github.com/opengoui/puresdl/internal/sdlsync"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sdlsync.Generate = func(c sdlsync.GenConfig) error { return gensdl.Run(gensdl.Config(c)) }
	code := sdlsync.Main(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
