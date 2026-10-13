// Command infinite-monkeys lets random "monkeys" bash a keyboard until they
// reproduce a text, one character at a time.
//
// It is a port of SDL/examples/demo/03-infinite-monkeys/infinite-monkeys.c.
//
// Run it with:
//
//	go run ./examples/demo/03-infinite-monkeys [-monkeys N] [-frames N] [file.txt]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to its OS thread, since SDL video and event
// calls must run on the thread that initialized them.
func init() {
	runtime.LockOSThread()
}

// The lowest and highest scancodes a monkey can hit, and the debug font's
// character cell size in pixels.
const (
	minMonkeyScancode = sdl.SCANCODE_A
	maxMonkeyScancode = sdl.SCANCODE_SLASH
	charSize          = sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
)

// defaultText is the text the monkeys type when no file is given.
const defaultText = `Jabberwocky, by Lewis Carroll

'Twas brillig, and the slithy toves
      Did gyre and gimble in the wabe:
All mimsy were the borogoves,
      And the mome raths outgrabe.

"Beware the Jabberwock, my son!
      The jaws that bite, the claws that catch!
Beware the Jubjub bird, and shun
      The frumious Bandersnatch!"

He took his vorpal sword in hand;
      Long time the manxome foe he sought-
So rested he by the Tumtum tree
      And stood awhile in thought.

And, as in uffish thought he stood,
      The Jabberwock, with eyes of flame,
Came whiffling through the tulgey wood,
      And burbled as it came!

One, two! One, two! And through and through
      The vorpal blade went snicker-snack!
He left it dead, and with its head
      He went galumphing back.

"And hast thou slain the Jabberwock?
      Come to my arms, my beamish boy!
O frabjous day! Callooh! Callay!"
      He chortled in his joy.

'Twas brillig, and the slithy toves
      Did gyre and gimble in the wabe:
All mimsy were the borogoves,
      And the mome raths outgrabe.
`

// line is one row of decoded text.
type line []rune

// app is the demo state: the target text, typing progress, and the on-screen
// text grid. It must only be used from the main thread.
type app struct {
	renderer *sdl.Renderer
	monkeys  int

	// now reads the wall clock and rand draws a number in [0, n); both are
	// injectable so tests can control time and the monkeys.
	now  func() sdl.Time
	rand func(n int32) int32

	text     string
	progress int // byte offset of the next character to be typed

	startTime, endTime sdl.Time

	row, rows, cols int
	lines           []line // ring buffer of rows lines, nil when the window is too small
	monkeyChars     []rune // the characters currently typed, one per column
}

// main parses flags and runs the demo, exiting with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	monkeys := flag.Int("monkeys", 100, "number of monkeys")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-monkeys N] [-frames N] [file.txt]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if err := run(*frames, *monkeys, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "infinite-monkeys:", err)
		os.Exit(1)
	}
}

// run initializes SDL and runs the demo loop until a quit event or, when
// maxFrames is positive, that many frames. A non-empty file replaces the
// built-in text and its read error is returned. It must be called on the main
// thread.
func run(maxFrames, monkeys int, file string) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Infinite Monkeys", "1.0", "com.example.infinite-monkeys")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/demo/infinite-monkeys", 640, 480, sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderVSync(renderer, 1)

	text := defaultText
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("couldn't open %s: %w", file, err)
		}
		text = string(data)
	}
	a := newApp(renderer, monkeys, text, currentTime, sdl.Rand)

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if a.handleEvent(&event) {
				return nil
			}
		}
		a.advance()
		a.draw()
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// currentTime reads SDL's wall clock.
func currentTime() sdl.Time {
	var t sdl.Time
	sdl.GetCurrentTime(&t)
	return t
}

// newApp creates the state for typing text with the given number of monkeys
// and lays it out for the current size of renderer.
func newApp(r *sdl.Renderer, monkeys int, text string, now func() sdl.Time, rand func(int32) int32) *app {
	a := &app{renderer: r, monkeys: monkeys, text: text, now: now, rand: rand}
	a.startTime = now()
	a.onWindowSizeChanged()
	return a
}

// handleEvent mirrors SDL_AppEvent and reports whether the program should quit.
func (a *app) handleEvent(e *sdl.Event) (quit bool) {
	switch sdl.EventType(e.Type()) {
	case sdl.EVENT_WINDOW_PIXEL_SIZE_CHANGED:
		a.onWindowSizeChanged()
	case sdl.EVENT_QUIT:
		return true
	}
	return false
}

// onWindowSizeChanged recomputes the text grid for the renderer's output size,
// discarding the text typed so far on screen. The grid is left nil when the
// window is too small to hold any rows or columns.
func (a *app) onWindowSizeChanged() {
	var w, h int32
	if !sdl.GetCurrentRenderOutputSize(a.renderer, &w, &h) {
		return
	}
	a.lines, a.monkeyChars = nil, nil
	a.row = 0
	a.rows = int(h)/charSize - 4
	a.cols = int(w) / charSize
	if a.rows > 0 && a.cols > 0 {
		a.lines = make([]line, a.rows)
		for i := range a.lines {
			a.lines[i] = make(line, 0, a.cols)
		}
		a.monkeyChars = make([]rune, a.cols)
		for i := range a.monkeyChars {
			a.monkeyChars[i] = ' '
		}
	}
}

// displayLine draws l as debug text with its top-left corner at (x, y).
func (a *app) displayLine(x, y float32, l []rune) {
	var sb strings.Builder
	for _, r := range l {
		sb.WriteString(sdl.UCS4ToUTF8(r))
	}
	sdl.RenderDebugText(a.renderer, x, y, sb.String())
}

// canMonkeyType reports whether a monkey can produce ch: it must be on the
// keyboard between the A and slash scancodes, with at most Shift held.
func canMonkeyType(ch rune) bool {
	var mod sdl.Keymod
	sc := sdl.GetScancodeFromKey(sdl.Keycode(ch), &mod)
	if sc < minMonkeyScancode || sc > maxMonkeyScancode {
		return false
	}
	return mod&^sdl.KMOD_SHIFT == 0
}

// advanceRow moves to the next row of the ring buffer and clears it.
func (a *app) advanceRow() {
	a.row++
	a.lines[a.row%a.rows] = a.lines[a.row%a.rows][:0]
}

// stepUTF8 decodes the character at the progress offset and advances past it.
func (a *app) stepUTF8() rune {
	r, n := sdl.StepUTF8(a.text[a.progress:])
	a.progress += n
	return r
}

// addMonkeyChar records ch as typed by monkey (or by nobody when monkey is
// negative), appends it to the visible text, and consumes it from the target
// text.
func (a *app) addMonkeyChar(monkey int, ch rune) {
	if monkey >= 0 && a.monkeyChars != nil {
		a.monkeyChars[monkey%a.cols] = ch
	}
	if a.lines != nil {
		if ch == '\n' {
			a.advanceRow()
		} else {
			i := a.row % a.rows
			a.lines[i] = append(a.lines[i], ch)
			if len(a.lines[i]) == a.cols {
				a.advanceRow()
			}
		}
	}
	if a.progress < len(a.text) {
		a.stepUTF8()
	}
}

// nextChar returns the next character the monkeys must type, or 0 when the
// text is finished. Characters monkeys cannot type are emitted for free.
func (a *app) nextChar() rune {
	var ch rune
	for a.progress < len(a.text) {
		r, _ := sdl.StepUTF8(a.text[a.progress:])
		ch = r
		if canMonkeyType(ch) {
			break
		}
		// A freebie: monkeys can't type this.
		a.addMonkeyChar(-1, ch)
	}
	return ch
}

// monkeyPlay returns the character produced by one random key press, with
// Shift held at random.
func (a *app) monkeyPlay() rune {
	count := int32(maxMonkeyScancode - minMonkeyScancode + 1)
	sc := minMonkeyScancode + sdl.Scancode(a.rand(count))
	var mod sdl.Keymod
	if a.rand(2) != 0 {
		mod = sdl.KMOD_SHIFT
	}
	return rune(sdl.GetKeyFromScancode(sc, mod, false))
}

// advance lets every monkey hit one key, like the first half of SDL_AppIterate.
func (a *app) advance() {
	var next rune
	for monkey := 0; monkey < a.monkeys; monkey++ {
		if next == 0 {
			next = a.nextChar()
			if next == 0 {
				break // all done
			}
		}
		if ch := a.monkeyPlay(); ch == next {
			a.addMonkeyChar(monkey, ch)
			next = 0
		}
	}
}

// draw renders the current state without presenting it.
func (a *app) draw() {
	r := a.renderer
	sdl.SetRenderDrawColor(r, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(r)

	// Show the text already decoded.
	sdl.SetRenderDrawColor(r, 255, 255, 255, sdl.ALPHA_OPAQUE)
	var x, y float32
	if a.lines != nil {
		rowOffset := max(a.row-a.rows+1, 0)
		for i := 0; i < a.rows; i++ {
			a.displayLine(x, y, a.lines[(rowOffset+i)%a.rows])
			y += charSize
		}

		// Show the caption.
		y = float32((a.rows + 1) * charSize)
		var now sdl.Time
		if a.progress >= len(a.text) {
			if a.endTime == 0 {
				a.endTime = a.now()
			}
			now = a.endTime
		} else {
			now = a.now()
		}
		elapsed := int64(now-a.startTime) / sdl.NS_PER_SECOND
		seconds := elapsed % 60
		elapsed /= 60
		minutes := elapsed % 60
		hours := elapsed / 60
		sdl.RenderDebugText(r, x, y, fmt.Sprintf("Monkeys: %d - %dH:%dM:%dS", a.monkeys, hours, minutes, seconds))
		y += charSize

		// Show the characters currently typed.
		a.displayLine(x, y, a.monkeyChars)
		y += charSize
	}

	// Show the current progress.
	sdl.SetRenderDrawColor(r, 0, 255, 0, sdl.ALPHA_OPAQUE)
	rect := sdl.FRect{X: x, Y: y, H: charSize}
	if len(a.text) > 0 {
		rect.W = float32(a.progress) / float32(len(a.text)) * float32(a.cols*charSize)
	}
	sdl.RenderFillRect(r, &rect)
}
