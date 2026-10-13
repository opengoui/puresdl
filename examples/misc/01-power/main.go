// Command power reports power status (plugged in, battery level, etc).
// It is a port of SDL/examples/misc/01-power/power.c.
//
// Run it with:
//
//	go run ./examples/misc/01-power [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to its OS thread, as SDL requires for
// video and event handling.
func init() {
	runtime.LockOSThread()
}

// main parses the -frames flag, runs the example, and exits with status 1
// after printing the error if it fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "power:", err)
		os.Exit(1)
	}
}

// run initializes SDL, creates a resizable window and renderer, and loops
// polling events, drawing and presenting frames until a quit event arrives
// or maxFrames frames have been shown (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Misc Power", "1.0", "com.example.misc-power")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("Couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/misc/power", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("Couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	configure(renderer)

	a := newApp(queryPower)
	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.iterate(renderer)
		sdl.RenderPresent(renderer)
		sdl.Delay(16) // the C relies on vsync; keep headless runs from spinning
	}
	return nil
}

// configure sets the 640x480 letterboxed logical presentation.
func configure(renderer *sdl.Renderer) {
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
}

// powerInfo is one reading of sdl.GetPowerInfo.
type powerInfo struct {
	state   sdl.PowerState
	seconds int32 // seconds of battery left, negative if unknown
	percent int32 // battery percentage, negative if unknown
	err     string
}

// queryPower reads the real power status; err is filled for POWERSTATE_ERROR.
func queryPower() powerInfo {
	var p powerInfo
	p.state = sdl.GetPowerInfo(&p.seconds, &p.percent)
	if p.state == sdl.POWERSTATE_ERROR {
		p.err = sdl.GetError()
	}
	return p
}

// app mirrors the C SDL_AppEvent/SDL_AppIterate split. The power source is
// injectable so tests can exercise every power state.
type app struct {
	query func() powerInfo
}

// newApp returns an app that reads power status through query.
func newApp(query func() powerInfo) *app { return &app{query: query} }

// handleEvent reports whether the program should end. Like the C source,
// only SDL_EVENT_QUIT ends it.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	return sdl.EventType(e.Type()) == sdl.EVENT_QUIT
}

// iterate clears and draws one frame; it does not present.
func (a *app) iterate(renderer *sdl.Renderer) { draw(renderer, a.query()) }

// textX centers a string of n debug-font characters over the bar frame.
func textX(frame sdl.FRect, n int) float32 {
	return frame.X + (frame.W-float32(sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE*n))/2
}

// draw clears the renderer with a color chosen from info.state and, when the
// battery percentage is known, draws a percentage bar with a battery/time
// remaining label, plus any status messages above the bar.
func draw(renderer *sdl.Renderer, info powerInfo) {
	frame := sdl.FRect{X: 100, Y: 200, W: 440, H: 80} // the percentage bar dimensions

	state, seconds, percent := info.state, info.seconds, info.percent

	var clearR, clearG, clearB uint8
	var barR, barG, barB uint8
	msg, msg2 := "", ""

	switch state {
	case sdl.POWERSTATE_ERROR:
		msg2 = "ERROR GETTING POWER STATE"
		msg = info.err
		clearR = 255
	case sdl.POWERSTATE_ON_BATTERY:
		msg = "Running on battery."
		barR = 255
	case sdl.POWERSTATE_NO_BATTERY:
		msg = "Plugged in, no battery available."
		clearG = 50
	case sdl.POWERSTATE_CHARGING:
		msg = "Charging."
		barB, barG = 255, 255
	case sdl.POWERSTATE_CHARGED:
		msg = "Charged."
		barG = 255
	default: // unknown, or anything unexpected
		msg = "Power state is unknown."
		clearR, clearG, clearB = 50, 50, 50
	}

	sdl.SetRenderDrawColor(renderer, clearR, clearG, clearB, 255)
	sdl.RenderClear(renderer)

	if percent >= 0 {
		pct := frame
		pct.W *= float32(percent) / 100

		remain := "unknown time"
		if seconds >= 0 {
			h := seconds / 3600
			seconds -= h * 3600
			m := seconds / 60
			seconds -= m * 60
			remain = fmt.Sprintf("%02d:%02d:%02d", h, m, seconds)
		}
		buf := fmt.Sprintf("Battery: %3d percent, %s remaining", percent, remain)
		x := textX(frame, len(buf))
		y := frame.Y + frame.H + sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE

		sdl.SetRenderDrawColor(renderer, barR, barG, barB, 255)
		sdl.RenderFillRect(renderer, &pct)
		sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
		sdl.RenderRect(renderer, &frame)
		sdl.RenderDebugText(renderer, x, y, buf)
	}

	if msg != "" {
		sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
		sdl.RenderDebugText(renderer, textX(frame, len(msg)), frame.Y-sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE*2, msg)
	}
	if msg2 != "" {
		sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
		sdl.RenderDebugText(renderer, textX(frame, len(msg2)), frame.Y-sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE*4, msg2)
	}
}
