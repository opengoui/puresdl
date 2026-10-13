// Command 03-locale reports the user's preferred locales, in order of
// preference, as debug text in a window. It ports
// SDL/examples/misc/03-locale/locale.c.
//
// Run it headless with:
//
//	SDL_VIDEODRIVER=dummy go run ./examples/misc/03-locale -frames 30
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to the OS main thread, as SDL requires.
func init() {
	// SDL video and event functions must run on the process main thread.
	runtime.LockOSThread()
}

// Window dimensions in logical pixels, and the width of one debug-text
// character.
const (
	width    = 640
	height   = 480
	charSize = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
)

// main parses the -frames flag and runs the example, exiting with status 1 on
// error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 runs until the window is closed)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "03-locale:", err)
		os.Exit(1)
	}
}

// drawCentered draws msg horizontally centered at row y.
func drawCentered(r *sdl.Renderer, y float32, msg string) {
	x := (width - float32(charSize*len(msg))) / 2
	sdl.SetRenderDrawColor(r, 255, 255, 255, 255)
	sdl.RenderDebugText(r, x, y, msg)
}

// app is the example state: where preferred locales come from, mirroring the
// SDL_AppEvent/SDL_AppIterate split of the C program.
type app struct {
	// locales returns the preferred locales in order of preference, or nil
	// if they cannot be determined.
	locales func() []sdl.LocaleInfo
}

// newApp returns an app that reads preferred locales from locales.
func newApp(locales func() []sdl.LocaleInfo) *app { return &app{locales: locales} }

// handleEvent reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// localeLine formats one locale entry as " - lang" or " - lang_COUNTRY".
func localeLine(l sdl.LocaleInfo) string {
	msg := " - " + l.Language
	if l.Country != "" {
		msg += "_" + l.Country
	}
	return msg
}

// draw clears the renderer to black and draws the locale list. It does not
// present.
func (a *app) draw(renderer *sdl.Renderer) {
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, 255)
	sdl.RenderClear(renderer)

	locales := a.locales()
	if locales == nil {
		// The C code draws an uninitialized buffer here; draw the error instead.
		drawCentered(renderer, 0, "Couldn't get locales: "+sdl.GetError())
		return
	}
	drawCentered(renderer, 0, fmt.Sprintf("Locales, in order of preference (%d total):", len(locales)))
	for i, l := range locales {
		drawCentered(renderer, float32(charSize*2*(i+1)), localeLine(l))
	}
}

// run initializes SDL, creates a window and renderer, and draws the locale
// list each frame until the window is closed or maxFrames frames have been
// rendered (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Misc Locale", "1.0", "com.example.misc-locale")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/misc/locale", width, height, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, width, height, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	a := newApp(sdl.GetPreferredLocales)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.draw(renderer)
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}
