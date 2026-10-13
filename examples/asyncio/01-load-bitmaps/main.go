// Command 01-load-bitmaps loads PNG files with SDL's asynchronous I/O
// queue and renders them as they finish loading.
// It is a port of SDL/examples/asyncio/01-load-bitmaps/load-bitmaps.c.
//
// The C program reads the PNGs from the application's base path. Here the
// images are embedded in the binary and written to a temporary directory at
// startup, so that SDL_LoadFileAsync still performs real asynchronous file
// reads; the finished buffers are decoded through an in-memory IOStream.
//
// Run it with:
//
//	go run ./examples/asyncio/01-load-bitmaps
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// assets holds the PNG images, embedded so the example is self-contained.
//
//go:embed *.png
var assets embed.FS

// init locks the main goroutine to the process main thread. SDL video and
// event functions must run there (mandatory on macOS), and locking in init
// makes main.main start on it.
func init() {
	runtime.LockOSThread()
}

// pngs lists the embedded image files; their index doubles as the load
// userdata and the slot in app.textures.
var pngs = [...]string{"sample.png", "gamepad_front.png", "speaker.png", "icon2x.png"}

// textureRects gives the destination rectangle, in the 640x480 logical
// space, for each image in pngs.
var textureRects = [len(pngs)]sdl.FRect{
	{X: 116, Y: 156, W: 408, H: 167},
	{X: 20, Y: 200, W: 96, H: 60},
	{X: 525, Y: 180, W: 96, H: 96},
	{X: 288, Y: 375, W: 64, H: 64},
}

// main parses the command-line flags and calls run, exiting with status 1
// and a message on stderr if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "01-load-bitmaps:", err)
		os.Exit(1)
	}
}

// run initializes SDL, writes the embedded PNGs to a temporary
// directory, starts the asynchronous loads and renders the results until the
// window is closed or maxFrames frames have been drawn (0 means no limit).
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
	if !sdl.CreateWindowAndRenderer("examples/asyncio/load-bitmaps", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	dir, err := os.MkdirTemp("", "puresdl-asyncio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, name := range pngs {
		data, err := assets.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}

	a, err := newApp(renderer, dir)
	if err != nil {
		return err
	}
	defer a.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		if err := a.iterate(); err != nil {
			return err
		}
		a.draw()
		sdl.RenderPresent(renderer)
	}
	return nil
}

// app is the program state: the renderer it draws with, the async I/O queue
// the PNG loads complete on, and the textures created so far.
type app struct {
	renderer *sdl.Renderer
	queue    *sdl.AsyncIOQueue
	textures [len(pngs)]*sdl.Texture
}

// newApp creates the async I/O queue and starts loading every PNG from dir.
// A file that cannot be read is skipped, as in the C program.
func newApp(renderer *sdl.Renderer, dir string) (*app, error) {
	a := &app{renderer: renderer}
	if a.queue = sdl.CreateAsyncIOQueue(); a.queue == nil {
		return nil, fmt.Errorf("SDL_CreateAsyncIOQueue: %s", sdl.GetError())
	}
	for i, name := range pngs {
		// The userdata is a handle token for the index, safe to hold in SDL.
		// A failed start leaves the token unused, so release it.
		ud := sdl.NewUserdataHandle(i)
		if !sdl.LoadFileAsync(filepath.Join(dir, name), a.queue, ud) {
			sdl.FreeUserdataHandle(ud)
		}
	}
	return a, nil
}

// handleEvent reports whether e asks the program to quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// iterate consumes at most one finished load, without blocking.
func (a *app) iterate() error {
	var outcome sdl.AsyncIOOutcome
	if sdl.GetAsyncIOResult(a.queue, &outcome) { // a PNG load has finished?
		return a.handleOutcome(&outcome)
	}
	return nil
}

// handleOutcome turns one finished load into a texture and releases its
// buffer and userdata.
func (a *app) handleOutcome(outcome *sdl.AsyncIOOutcome) error {
	defer sdl.Free(outcome.Buffer)
	defer sdl.FreeUserdataHandle(outcome.Userdata)
	if outcome.Result != sdl.ASYNCIO_COMPLETE {
		return nil
	}
	// This might be any of the PNGs; they finish in any order.
	i, _ := sdl.UserdataHandleValue(outcome.Userdata).(int)
	if i < 0 || i >= len(pngs) {
		return nil
	}
	src := sdl.IOFromConstMem(outcome.Buffer, uint(outcome.BytesTransferred))
	surface := sdl.LoadPNG_IO(src, true)
	if surface == nil {
		return nil
	}
	// The renderer is not multithreaded, so create the texture here.
	a.textures[i] = sdl.CreateTextureFromSurface(a.renderer, surface)
	if a.textures[i] == nil {
		return fmt.Errorf("SDL_CreateTextureFromSurface: %s", sdl.GetError())
	}
	sdl.DestroySurface(surface)
	return nil
}

// draw clears to black and draws every texture loaded so far. It does not present.
func (a *app) draw() {
	sdl.SetRenderDrawColor(a.renderer, 0, 0, 0, 255)
	sdl.RenderClear(a.renderer)
	for i, t := range a.textures {
		if t != nil {
			sdl.RenderTexture(a.renderer, t, nil, &textureRects[i])
		}
	}
}

// close destroys the queue and the textures.
func (a *app) close() {
	sdl.DestroyAsyncIOQueue(a.queue)
	for i, t := range a.textures {
		if t != nil {
			sdl.DestroyTexture(t)
			a.textures[i] = nil
		}
	}
}
