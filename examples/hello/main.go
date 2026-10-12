// Command hello opens an SDL3 window, clears it with a renderer whose color
// cycles over time, processes events and quits when the window is closed.
//
// Run it with:
//
//	go run ./examples/hello
//
// The -frames flag makes it exit after N frames, which allows running it
// headless:
//
//	SDL_VIDEODRIVER=dummy go run ./examples/hello -frames 30
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

func init() {
	// SDL video and event functions must run on the process main thread
	// (mandatory on macOS). Locking in init makes main.main run there.
	runtime.LockOSThread()
}

func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 runs until the window is closed)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}

func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("puresdl hello", 640, 480, sdl.WINDOW_RESIZABLE, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderVSync(renderer, 1)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			switch sdl.EventType(event.Type()) {
			case sdl.EVENT_QUIT:
				return nil
			case sdl.EVENT_KEY_DOWN:
				if event.Key().Key == sdl.SDLK_ESCAPE {
					return nil
				}
			}
		}
		// Cycle the background color with the SDL tick counter.
		t := float64(sdl.GetTicks()) / 1000
		r := uint8(127 + 127*math.Sin(t))
		g := uint8(127 + 127*math.Sin(t+2))
		b := uint8(127 + 127*math.Sin(t+4))
		sdl.SetRenderDrawColor(renderer, r, g, b, 255)
		sdl.RenderClear(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1) // keep headless runs from spinning when vsync is a no-op
		}
	}
	return nil
}
