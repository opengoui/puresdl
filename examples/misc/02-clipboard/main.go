// Command 02-clipboard lets the user copy and paste text with the system
// clipboard. It is a port of SDL/examples/misc/02-clipboard/clipboard.c.
//
// Click the first button to copy the current time to the clipboard, and the
// second to paste the clipboard text into the green area.
//
// Run it with:
//
//	go run ./examples/misc/02-clipboard [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to the OS main thread, as SDL requires for
// video and event handling.
func init() {
	runtime.LockOSThread()
}

// Layout and label constants for the example UI.
const (
	charSize        = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	copyButtonStr   = "Click here to copy!"
	pasteButtonStr  = "Click here to paste!"
	unknownTimeText = "(Don't know the current time, sorry.)"
)

// months and days hold the English names used by formatTime, indexed by
// zero-based month and by day of week starting at Sunday.
var (
	months = [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	days   = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

// app holds the program state.
type app struct {
	renderer *sdl.Renderer

	currentTimeRect sdl.FRect
	copyButtonRect  sdl.FRect
	pasteTextRect   sdl.FRect
	pasteButtonRect sdl.FRect

	copyPressed  bool
	pastePressed bool
	currentTime  string
	pastedStr    string
	hasPasted    bool

	// timeString supplies the clock text each frame; tests replace it.
	timeString func() string
}

// main parses flags, runs the example, and exits with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "02-clipboard:", err)
		os.Exit(1)
	}
}

// currentTimeString returns the current local time formatted for display, or
// unknownTimeText if SDL cannot provide it.
func currentTimeString() string {
	var ticks sdl.Time
	var dt sdl.DateTime
	if !sdl.GetCurrentTime(&ticks) || !sdl.TimeToDateTime(ticks, &dt, true) {
		return unknownTimeText
	}
	return formatTime(&dt)
}

// formatTime renders dt like the C example's CalculateCurrentTimeString.
func formatTime(dt *sdl.DateTime) string {
	return fmt.Sprintf("%s, %s %d, %d   %02d:%02d:%02d",
		days[dt.DayOfWeek], months[dt.Month-1], dt.Day, dt.Year, dt.Hour, dt.Minute, dt.Second)
}

// run initializes SDL, creates the window and renderer, and runs the event
// loop until a quit event arrives or maxFrames frames have been rendered
// (0 means no limit).
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Misc Clipboard", "1.0", "com.example.misc-clipboard")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("Couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/misc/clipboard", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("Couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	a := &app{renderer: renderer, timeString: currentTimeString}
	a.layout()
	a.currentTime = a.timeString()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.iterate()
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// layout sets up the locations where we'll draw stuff.
func (a *app) layout() {
	a.currentTimeRect = sdl.FRect{X: 30, Y: 10, W: 390, H: charSize + 10}

	a.copyButtonRect = sdl.FRect{
		X: a.currentTimeRect.X + a.currentTimeRect.W + 30,
		Y: a.currentTimeRect.Y,
		W: float32(charSize*len(copyButtonStr) + 10),
		H: a.currentTimeRect.H,
	}

	a.pasteTextRect.X = 10
	a.pasteTextRect.Y = a.currentTimeRect.Y + a.currentTimeRect.H + 10
	a.pasteTextRect.W = 620
	a.pasteTextRect.H = ((480 - a.pasteTextRect.Y) - a.copyButtonRect.H) - 20

	a.pasteButtonRect.W = float32(charSize*len(pasteButtonStr) + 10)
	a.pasteButtonRect.X = (640 - a.pasteButtonRect.W) / 2
	a.pasteButtonRect.Y = a.pasteTextRect.Y + a.pasteTextRect.H + 10
	a.pasteButtonRect.H = a.copyButtonRect.H
}

// handleEvent processes one event and reports whether the program should quit.
func (a *app) handleEvent(event *sdl.Event) bool {
	sdl.ConvertEventToRenderCoordinates(a.renderer, event)
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_MOUSE_BUTTON_DOWN:
		b := event.Button()
		if b.Button == sdl.BUTTON_LEFT {
			p := sdl.FPoint{X: b.X, Y: b.Y}
			a.copyPressed = sdl.PointInRectFloat(&p, &a.copyButtonRect)
			a.pastePressed = sdl.PointInRectFloat(&p, &a.pasteButtonRect)
		}
	case sdl.EVENT_MOUSE_BUTTON_UP:
		b := event.Button()
		if b.Button == sdl.BUTTON_LEFT {
			p := sdl.FPoint{X: b.X, Y: b.Y}
			if a.copyPressed && sdl.PointInRectFloat(&p, &a.copyButtonRect) {
				sdl.SetClipboardText(a.currentTime)
			} else if a.pastePressed && sdl.PointInRectFloat(&p, &a.pasteButtonRect) {
				a.pastedStr = sdl.GetClipboardText()
				a.hasPasted = true
			}
			a.copyPressed, a.pastePressed = false, false
		}
	}
	return false
}

// renderPastedText draws the pasted text. It doesn't wordwrap or deal with
// Unicode; this is just a simple example app.
func (a *app) renderPastedText() {
	if !a.hasPasted {
		return
	}
	x := a.pasteTextRect.X + 5
	y := a.pasteTextRect.Y + 5
	w := a.pasteTextRect.W - 10
	h := a.pasteTextRect.H
	maxChars := int(w / charSize)

	drawLine := func(s string) {
		if len(s) > maxChars {
			s = s[:maxChars]
		}
		sdl.RenderDebugText(a.renderer, x, y, s)
	}

	str := a.pastedStr
	for {
		nl := strings.IndexByte(str, '\n')
		if nl < 0 {
			break
		}
		line := strings.TrimSuffix(str[:nl], "\r")
		drawLine(line)
		str = str[nl+1:]
		y += charSize + 2
		if h-y < charSize {
			return // no space for another line, stop here.
		}
	}
	// Last text after newline, if there's room.
	if h-y >= charSize {
		drawLine(str)
	}
}

// fillAndFrame fills rect with the given opaque color and draws a white
// outline around it.
func (a *app) fillAndFrame(rect *sdl.FRect, r, g, b uint8) {
	sdl.SetRenderDrawColor(a.renderer, r, g, b, 255)
	sdl.RenderFillRect(a.renderer, rect)
	sdl.SetRenderDrawColor(a.renderer, 255, 255, 255, 255)
	sdl.RenderRect(a.renderer, rect)
}

// button draws a framed button with label, green while pressed and red
// otherwise.
func (a *app) button(rect *sdl.FRect, pressed bool, label string) {
	if pressed {
		a.fillAndFrame(rect, 0, 255, 0)
	} else {
		a.fillAndFrame(rect, 255, 0, 0)
	}
	sdl.SetRenderDrawColor(a.renderer, 255, 255, 255, 255)
	sdl.RenderDebugText(a.renderer, rect.X+5, rect.Y+5, label)
}

// iterate renders one frame.
func (a *app) iterate() {
	a.currentTime = a.timeString()
	a.draw()
	sdl.RenderPresent(a.renderer)
}

// draw renders the whole UI without presenting it.
func (a *app) draw() {
	sdl.SetRenderDrawColor(a.renderer, 0, 0, 0, 255)
	sdl.RenderClear(a.renderer)

	// Frame around the current time, with the time drawn inside.
	a.fillAndFrame(&a.currentTimeRect, 0, 0, 255)
	x := a.currentTimeRect.X + (a.currentTimeRect.W-float32(charSize*len(a.currentTime)))/2
	y := a.currentTimeRect.Y + 5
	sdl.SetRenderDrawColor(a.renderer, 255, 255, 0, 255)
	sdl.RenderDebugText(a.renderer, x, y, a.currentTime)

	a.button(&a.copyButtonRect, a.copyPressed, copyButtonStr)

	// Pasted text area and its contents.
	a.fillAndFrame(&a.pasteTextRect, 0, 53, 25)
	sdl.SetRenderDrawColor(a.renderer, 0, 219, 107, 255)
	a.renderPastedText()

	a.button(&a.pasteButtonRect, a.pastePressed, pasteButtonStr)
}
