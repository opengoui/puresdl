# puresdl

`puresdl` is a pure Go binding for SDL 3 (`github.com/opengoui/puresdl`, package `sdl`). It uses [purego](https://github.com/ebitengine/purego) to load SDL 3 at runtime, so it needs no cgo or C toolchain and can be cross-compiled with `CGO_ENABLED=0`.

The bindings are generated from the official SDL headers pinned by [`SDL_VERSION`](SDL_VERSION), currently `release-3.4.18`. The generated package covers SDL functions, constants, enums, structs, unions, callback types, and their documentation. Importing the package does not panic when SDL 3 is unavailable; check `sdl.LoadError()` before using it.

## Requirements

- Go 1.27.2 or later.
- An SDL 3 shared library at runtime. The matching `release-3.4.18` library has the symbols used by these bindings; newer SDL 3 releases usually work as well.

Install SDL 3 using your platform's package manager or download it from the [SDL releases page](https://github.com/libsdl-org/SDL/releases):

| Platform | Example installation | Default library lookup |
| --- | --- | --- |
| macOS | `brew install sdl3` | `libSDL3.dylib`, common Homebrew/MacPorts paths, or `SDL3.framework` |
| Linux | `apt install libsdl3-0` (if available for your distribution) | `libSDL3.so.0` or `libSDL3.so` via the dynamic linker and common library paths |
| Windows | Download `SDL3.dll` from SDL releases | Beside the executable or on `PATH` |

Set `SDL3_LIBRARY` to an explicit library path when needed. In Go, you can also call `sdl.Load(path)`. `sdl.LoadError()` reports a loading failure, `sdl.LoadedPath()` returns the loaded path, and `sdl.Available("SDL_Init")` checks whether a symbol exists in the loaded library.

## Quick start

```go
package main

import (
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// SDL video and event functions must run on the process main thread on macOS.
func init() {
	runtime.LockOSThread()
}

func main() {
	if err := sdl.LoadError(); err != nil {
		panic(err)
	}
	if !sdl.Init(sdl.INIT_VIDEO) {
		panic(sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer(
		"hello", 640, 480,
		sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY,
		&window, &renderer,
	) {
		panic(sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	var event sdl.Event
	for running := true; running; {
		for sdl.PollEvent(&event) {
			if sdl.EventType(event.Type()) == sdl.EVENT_QUIT {
				running = false
			}
		}
		sdl.SetRenderDrawColor(renderer, 30, 120, 220, 255)
		sdl.RenderClear(renderer)
		sdl.RenderPresent(renderer)
		sdl.Delay(16)
	}
}
```

SDL video and event APIs generally require the main thread. On macOS, call `runtime.LockOSThread()` from `init` as shown above.

## Examples

[`examples/`](examples/README.md) contains Go ports of the examples in the SDL repository. For example:

```bash
go run ./examples/renderer/01-clear
```

Examples accept `-frames N`, which is useful for running them with SDL's dummy drivers without opening a window:

```bash
SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy SDL_CAMERA_DRIVER=dummy \
  go run ./examples/renderer/01-clear -frames 30
```

See [examples/TESTING.md](examples/TESTING.md) for the example test and manual review checklist.

## API conventions

See [`sdl/doc.go`](sdl/doc.go) or run `go doc ./sdl` for package documentation. Common conventions include:

- The `SDL_` prefix is removed: `SDL_CreateWindow` becomes `sdl.CreateWindow`, and `SDL_INIT_VIDEO` becomes `sdl.INIT_VIDEO`. Keycode constants retain their `SDLK_` prefix.
- C `bool`, integer, floating-point, and enum types map to Go types of the same size. Opaque SDL handles are pointers, such as `*sdl.Window`.
- C `const char *` parameters use Go `string`. Parameters where NULL differs from an empty string use `*string`; `nil` represents NULL and `sdl.Ptr("")` represents an empty string.
- SDL-owned string results are copied into Go strings. Functions returning arrays that SDL expects callers to free return copied Go slices and release the SDL allocation.
- C unions such as `sdl.Event` expose members through accessor methods, for example `event.Type()` and `event.Key()`.
- printf-style APIs accept a Go format string and arguments; formatting happens in Go before calling SDL.
- Callback types use `uintptr` and their corresponding `sdl.New...` constructors. purego supports about 2,000 callbacks per process and does not release them, so create callbacks once and reuse them.

## SDL version and code generation

The root [`SDL_VERSION`](SDL_VERSION) file pins the SDL release used to generate the bindings. To regenerate them:

```bash
go generate ./sdl
```

`cmd/sdlsync` downloads and caches the official headers for the pinned release, then runs the same generation pipeline as `cmd/gensdl`. Useful commands include:

```bash
go run ./cmd/sdlsync -check       # Check generated files against SDL_VERSION
go run ./cmd/sdlsync -list        # List recent stable SDL releases
go run ./cmd/sdlsync -latest      # Synchronize headers for the latest stable release
```

Do not edit `*_gen.go` files by hand. Update the generator rules under `internal/bindgen` or the handwritten patches under `internal/gensdl`, then regenerate. See [docs/architecture.md](docs/architecture.md) for the generator design.

## Limitations

- purego does not support arbitrary C variadic functions or `va_list` APIs. printf-style functions with a `const char *fmt, ...` signature are handled by formatting their arguments in Go; other unsupported declarations are listed in [docs/skipped.txt](docs/skipped.txt).
- Bindings are generated for SDL 3.4.18. A function added in a newer SDL release is not available until the bindings are regenerated. If a function is missing from the loaded library, calling it panics with a message naming that symbol; use `sdl.Available` or `sdl.GetVersion` to check compatibility.
- Some APIs are only declared by SDL headers for specific platforms. They are present when generated, but calling one on a platform whose SDL library does not export it will panic.
- 32-bit targets can compile, but purego does not support the by-value struct calls required by this binding on 32-bit architectures. `sdl.Load` returns `sdl.ErrUnsupportedArch` there.
- The SDL `SDL_main` entry point is not exported by libSDL3 and is not provided by this package. Define the Go program's own `main` function.

## License

This project is licensed under the [MIT License](LICENSE).
