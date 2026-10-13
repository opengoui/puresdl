// Command 01-user creates a window and waits for the user to click on
// it. The window is blue by default; when a storage operation succeeds it
// turns green, and if it fails it turns red and the error is logged. Left
// clicking saves a game, any other click loads it.
//
// The goal is to show how to handle save data without blocking the main
// thread while keeping the storage handle open for as short a time as
// possible.
//
// It is a port of SDL's SDL/examples/storage/01-user/user.c.
//
// Run it with:
//
//	go run ./examples/storage/01-user [-frames N] [-autoclick]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to the OS main thread, as SDL requires for
// video and event handling.
func init() {
	runtime.LockOSThread()
}

// Steps that occur as part of saving or loading a game.
const (
	saveStateUnstarted             = iota // blue
	saveStateProcessingGameWorld          // yellow
	saveStatePreparingStorage             // cyan
	saveStateProcessingStorageFile        // magenta
	saveStateFinalCheck                   // green if success, red if failed
)

// saveFileName is the name of the save file within the user storage.
const saveFileName = "save.sav"

// saver holds the save/load state machine shared by the main thread and the
// worker thread.
type saver struct {
	state sdl.AtomicInt

	// result indicates success or failure during the final check.
	result int32

	// thread handles the majority of save operations.
	thread *sdl.Thread

	// ready is signaled by the main thread once storage is ready.
	ready *sdl.Semaphore

	// storage is the handle for the user's filesystem. It is written by the
	// worker thread and read by the main thread.
	storage atomic.Pointer[sdl.Storage]

	// openStorage opens the storage; tests inject a hermetic directory.
	openStorage func() *sdl.Storage

	// shown is the state sampled by the last advance and used by draw.
	shown int32

	writeFn sdl.ThreadFunction
	readFn  sdl.ThreadFunction
}

// openUserStorage opens the per-user storage for this example's organization
// and application names. It returns nil on failure.
func openUserStorage() *sdl.Storage {
	return sdl.OpenUserStorage("libsdl", "User Storage Example", 0)
}

// newSaver creates the default (unstarted) save state. SDL must be loaded.
func newSaver(openStorage func() *sdl.Storage) *saver {
	g := &saver{result: -1, openStorage: openStorage}
	sdl.SetAtomicInt(&g.state, saveStateUnstarted)
	g.ready = sdl.CreateSemaphore(0)
	g.writeFn = sdl.NewThreadFunction(g.writeSaveData)
	g.readFn = sdl.NewThreadFunction(g.readSaveData)
	return g
}

// writeSaveData pretends to serialize a fictional game world, then opens the
// filesystem to write the serialized data.
func (g *saver) writeSaveData(unsafe.Pointer) int32 {
	sdl.SetAtomicInt(&g.state, saveStateProcessingGameWorld)

	// Pretend that an entire game fits in 64 bits.
	gameWorld := []uint64{sdl.GetPerformanceCounter()}

	// Save data is ready; start opening the filesystem.
	st := g.openStorage()
	if st == nil {
		sdl.SetAtomicInt(&g.state, saveStateFinalCheck)
		return -1
	}
	g.storage.Store(st)
	sdl.SetAtomicInt(&g.state, saveStatePreparingStorage)

	// The main thread eventually signals that storage is ready.
	sdl.WaitSemaphore(g.ready)

	writeOK := sdl.WriteStorageFileSlice(st, saveFileName, gameWorld)

	// Regardless of what happened above, we've reached the end of the routine.
	sdl.CloseStorage(st)
	sdl.SetAtomicInt(&g.state, saveStateFinalCheck)

	if !writeOK {
		return -1
	}
	return 0
}

// readSaveData opens the filesystem to read a save file, then deserializes
// the data into fictional game world data.
func (g *saver) readSaveData(unsafe.Pointer) int32 {
	gameWorld := make([]uint64, 1)

	st := g.openStorage()
	if st == nil {
		sdl.SetAtomicInt(&g.state, saveStateFinalCheck)
		return -1
	}
	g.storage.Store(st)
	sdl.SetAtomicInt(&g.state, saveStatePreparingStorage)

	// The main thread eventually signals that storage is ready.
	sdl.WaitSemaphore(g.ready)

	var saveLen uint64
	if !sdl.GetStorageFileSize(st, saveFileName, &saveLen) {
		sdl.CloseStorage(st)
		sdl.SetAtomicInt(&g.state, saveStateFinalCheck)
		sdl.Log("Save data was not found")
		return -1
	} else if saveLen != uint64(unsafe.Sizeof(gameWorld[0])) {
		sdl.CloseStorage(st)
		sdl.SetAtomicInt(&g.state, saveStateFinalCheck)
		sdl.Log("Save data size is incorrect, was the file corrupted?")
		return -1
	}

	// Once the file is read, the storage handle is no longer needed.
	readOK := sdl.ReadStorageFileSlice(st, saveFileName, gameWorld)
	sdl.CloseStorage(st)

	sdl.SetAtomicInt(&g.state, saveStateProcessingGameWorld)

	if readOK {
		sdl.Log("Game World loaded, value was %d", gameWorld[0])
	}

	sdl.SetAtomicInt(&g.state, saveStateFinalCheck)

	if !readOK {
		return -1
	}
	return 0
}

// handleEvent mirrors SDL_AppEvent. It reports whether the program should quit.
func (g *saver) handleEvent(event *sdl.Event) (quit bool) {
	switch event.Type() {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_MOUSE_BUTTON_DOWN:
		if g.thread != nil {
			sdl.Log("Ignoring interaction, save/load is in progress")
			break
		}
		// Once the thread starts, it updates this to the first real state.
		sdl.SetAtomicInt(&g.state, saveStateUnstarted)
		if event.Button().Button == sdl.BUTTON_LEFT {
			g.thread = sdl.CreateThread(g.writeFn, "Save Write Thread", nil)
		} else {
			g.thread = sdl.CreateThread(g.readFn, "Save Read Thread", nil)
		}
	}
	return false
}

// advance mirrors the logic half of SDL_AppIterate: the main thread only
// helps the worker wait for storage to be ready and reads the result when the
// worker is finished.
func (g *saver) advance() {
	state := sdl.GetAtomicInt(&g.state)
	g.shown = state
	switch state {
	case saveStatePreparingStorage:
		if sdl.StorageReady(g.storage.Load()) {
			sdl.SetAtomicInt(&g.state, saveStateProcessingStorageFile)
			sdl.SignalSemaphore(g.ready)
		}
	case saveStateFinalCheck:
		if g.thread != nil {
			sdl.WaitThread(g.thread, &g.result)
			g.thread = nil
			if g.result == 0 {
				sdl.Log("Save/Load complete!")
			} else {
				sdl.Log("Save/Load failed: %s", sdl.GetError())
			}
		}
	}
}

// draw clears the renderer to the colour of the state sampled by advance. It
// does not present.
func (g *saver) draw(renderer *sdl.Renderer) {
	var red, green, blue float32
	switch g.shown {
	case saveStateUnstarted:
		blue = 1
	case saveStateProcessingGameWorld:
		red, green = 1, 1
	case saveStatePreparingStorage:
		green, blue = 1, 1
	case saveStateProcessingStorageFile:
		red, blue = 1, 1
	case saveStateFinalCheck:
		if g.result == 0 {
			green = 1
		} else {
			red = 1
		}
	}
	sdl.SetRenderDrawColorFloat(renderer, red, green, blue, 1)
	sdl.RenderClear(renderer)
}

// close forces a still-running worker not to wait, joins it and releases the
// semaphore.
func (g *saver) close() {
	if g.ready == nil {
		return
	}
	sdl.SignalSemaphore(g.ready)
	if g.thread != nil {
		sdl.WaitThread(g.thread, nil)
		g.thread = nil
	}
	sdl.DestroySemaphore(g.ready)
	g.ready = nil
}

// main parses the command-line flags, runs the example and exits with status 1
// if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	autoclick := flag.Bool("autoclick", false, "synthesize a left click (save) then a right click (load) for headless testing")
	flag.Parse()
	if err := run(*frames, *autoclick); err != nil {
		fmt.Fprintln(os.Stderr, "storage/01-user:", err)
		os.Exit(1)
	}
}

// pushClick pushes a synthetic single-click mouse button press for the given
// button onto the SDL event queue.
func pushClick(button uint8) {
	var ev sdl.Event
	b := ev.Button()
	b.Type = sdl.EVENT_MOUSE_BUTTON_DOWN
	b.Button = button
	b.Down = true
	b.Clicks = 1
	sdl.PushEvent(&ev)
}

// run initializes SDL, creates the window and renderer, and runs the event and
// draw loop for maxFrames frames (0 means until quit). When autoclick is set it
// synthesizes a left click (save) followed by a right click (load).
func run(maxFrames int, autoclick bool) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("User Storage Example", "1.0", "com.example.storage-user")

	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/storage/user", 640, 480, sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	g := newSaver(openUserStorage)
	defer g.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		if autoclick {
			switch frame {
			case 1:
				pushClick(sdl.BUTTON_LEFT)
			case maxFrames / 2:
				// Load is only possible once the save thread was reaped.
				if maxFrames > 0 {
					pushClick(sdl.BUTTON_RIGHT)
				}
			}
		}
		for sdl.PollEvent(&event) {
			if g.handleEvent(&event) {
				return nil
			}
		}
		g.advance()
		g.draw(renderer)
		sdl.RenderPresent(renderer)
		if autoclick {
			sdl.Delay(5)
		}
	}
	return nil
}
