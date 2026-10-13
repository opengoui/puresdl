package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	black = color.RGBA{0, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
)

// Canvas geometry used by most tests: 8 columns and 6 text rows. The caption
// is at y=56, the typed characters at y=64 and the progress bar at y=72.
const (
	cw, ch    = 64, 80
	captionY  = 56
	monkeysY  = 64
	progressY = 72
)

// fakeClock is a manually advanced wall clock in nanoseconds.
type fakeClock struct{ ns sdl.Time }

func (c *fakeClock) now() sdl.Time { return c.ns }

func (c *fakeClock) advanceSeconds(s int64) { c.ns += sdl.Time(s * sdl.NS_PER_SECOND) }

// script is a deterministic stand-in for the monkeys' random number source:
// every monkeyPlay consumes the next rune, as a (scancode, shift) pair.
type script struct {
	count  int32
	offs   []int32
	shifts []int32
	play   int
}

func newScript(t *testing.T, keys string) *script {
	t.Helper()
	s := &script{count: int32(maxMonkeyScancode - minMonkeyScancode + 1)}
	sdltest.OnMain(t, func() {
		for _, r := range keys {
			var mod sdl.Keymod
			sc := sdl.GetScancodeFromKey(sdl.Keycode(r), &mod)
			s.offs = append(s.offs, int32(sc-minMonkeyScancode))
			var shift int32
			if mod&sdl.KMOD_SHIFT != 0 {
				shift = 1
			}
			s.shifts = append(s.shifts, shift)
		}
	})
	return s
}

func (s *script) rand(n int32) int32 {
	if s.play >= len(s.offs) {
		panic("script exhausted")
	}
	if n == s.count {
		return s.offs[s.play]
	}
	if n != 2 {
		panic(fmt.Sprintf("unexpected rand(%d)", n))
	}
	v := s.shifts[s.play]
	s.play++
	return v
}

// newTestApp creates an app on a w x h canvas with a fake clock and a script.
func newTestApp(t *testing.T, w, h int32, monkeys int, text, keys string) (*app, *sdltest.Canvas, *fakeClock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, w, h)
	clock := &fakeClock{}
	sc := newScript(t, keys)
	var a *app
	sdltest.OnMain(t, func() { a = newApp(c.Renderer, monkeys, text, clock.now, sc.rand) })
	return a, c, clock
}

func step(t *testing.T, a *app) {
	t.Helper()
	sdltest.OnMain(t, a.advance)
}

func frame(t *testing.T, a *app, c *sdltest.Canvas) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, a.draw)
	return c.Capture(t)
}

func lineString(l line) string { return string(l) }

// debugText renders s with SDL's debug font on a fresh canvas of the given
// size at (0, y), the reference for what the app must draw there.
func debugText(t *testing.T, w, h int32, y float32, s string) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, w, h)
	sdltest.OnMain(t, func() {
		sdl.SetRenderDrawColor(c.Renderer, 0, 0, 0, 255)
		sdl.RenderClear(c.Renderer)
		sdl.SetRenderDrawColor(c.Renderer, 255, 255, 255, 255)
		sdl.RenderDebugText(c.Renderer, 0, y, s)
	})
	return c.Capture(t)
}

// assertBand fails unless rows [y, y+8) of got equal those of want.
func assertBand(t *testing.T, name string, got, want *image.RGBA, y int) {
	t.Helper()
	for yy := y; yy < y+8; yy++ {
		for x := 0; x < got.Bounds().Dx(); x++ {
			if g, w := got.RGBAAt(x, yy), want.RGBAAt(x, yy); g != w {
				t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, x, yy, g, w)
			}
		}
	}
}

func TestCanMonkeyType(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	for _, tc := range []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'z', true}, {'A', true}, {'Z', true}, {'5', true}, {' ', true},
		{',', true}, {'.', true}, {'!', true}, {'"', true}, {'\'', true}, {'-', true},
		{'\n', false}, {'é', false}, {'日', false}, {0x7f, false},
	} {
		got := sdltest.Call(t, func() bool { return canMonkeyType(tc.r) })
		if got != tc.want {
			t.Errorf("canMonkeyType(%q) = %v, want %v", tc.r, got, tc.want)
		}
	}
}

func TestMonkeyPlayProducesKeysWithShift(t *testing.T) {
	a, _, _ := newTestApp(t, cw, ch, 1, "x", "aAz.")
	var got []rune
	sdltest.OnMain(t, func() {
		for i := 0; i < 4; i++ {
			got = append(got, a.monkeyPlay())
		}
	})
	if string(got) != "aAz." {
		t.Errorf("monkeyPlay sequence = %q, want %q", string(got), "aAz.")
	}
}

func TestInitialFrame(t *testing.T) {
	a, c, _ := newTestApp(t, cw, ch, 3, "hello", "")
	if a.cols != 8 || a.rows != 6 || len(a.lines) != 6 {
		t.Fatalf("layout = %d cols x %d rows (%d lines), want 8x6 (6)", a.cols, a.rows, len(a.lines))
	}
	if got := string(a.monkeyChars); got != strings.Repeat(" ", 8) {
		t.Errorf("monkeyChars = %q, want 8 spaces", got)
	}
	img := frame(t, a, c)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, cw, captionY-8+8), black)     // text area is empty
	sdltest.AssertRegion(t, img, image.Rect(0, monkeysY, cw, progressY), black) // blank monkey row
	sdltest.AssertRegion(t, img, image.Rect(0, progressY, cw, ch), black)       // empty progress bar
	assertBand(t, "caption", img, debugText(t, cw, ch, captionY, "Monkeys: 3 - 0H:0M:0S"), captionY)
}

func TestMonkeyTypesNextCharacterOnly(t *testing.T) {
	// The first monkey hits the wrong key, the second the right one.
	a, _, _ := newTestApp(t, cw, ch, 2, "ab", "zq")
	step(t, a)
	// Monkeys: z (miss), then q (miss): nothing typed.
	if a.progress != 0 || len(a.lines[0]) != 0 {
		t.Fatalf("miss typed something: progress=%d line=%q", a.progress, lineString(a.lines[0]))
	}
	a2, _, _ := newTestApp(t, cw, ch, 2, "ab", "za")
	step(t, a2)
	if a2.progress != 1 || lineString(a2.lines[0]) != "a" {
		t.Fatalf("progress=%d line=%q, want 1 and \"a\"", a2.progress, lineString(a2.lines[0]))
	}
	if a2.monkeyChars[1] != 'a' || a2.monkeyChars[0] != ' ' {
		t.Errorf("monkeyChars = %q, want the hit shown under monkey 1", string(a2.monkeyChars))
	}
}

func TestEachMonkeyTypesAtMostOneCharacterPerFrame(t *testing.T) {
	// Two monkeys, two hits in order: both characters land in one frame.
	a, _, _ := newTestApp(t, cw, ch, 2, "ab", "ab")
	step(t, a)
	if a.progress != 2 || lineString(a.lines[0]) != "ab" {
		t.Fatalf("progress=%d line=%q, want 2 and \"ab\"", a.progress, lineString(a.lines[0]))
	}
	if string(a.monkeyChars[:2]) != "ab" {
		t.Errorf("monkeyChars = %q, want monkeys 0 and 1 to show a and b", string(a.monkeyChars))
	}
}

func TestShiftedCharacterIsTyped(t *testing.T) {
	a, _, _ := newTestApp(t, cw, ch, 2, "A", "aA")
	step(t, a)
	if a.progress != 1 || lineString(a.lines[0]) != "A" {
		t.Fatalf("progress=%d line=%q, want \"A\" (the unshifted a must not match)", a.progress, lineString(a.lines[0]))
	}
}

func TestMonkeyColumnWrapsAroundColumns(t *testing.T) {
	// Monkey 9 hits the key; with 8 columns it is shown in column 1.
	keys := strings.Repeat("z", 9) + "a"
	a, _, _ := newTestApp(t, cw, ch, 10, "a", keys)
	step(t, a)
	if a.monkeyChars[1] != 'a' {
		t.Errorf("monkeyChars = %q, want 'a' in column 1", string(a.monkeyChars))
	}
}

func TestFreebiesAreTypedForFree(t *testing.T) {
	// "\n" and "é" cannot be typed by monkeys, so they appear without a hit.
	a, _, _ := newTestApp(t, cw, ch, 1, "a\né\nb", "a"+"b")
	step(t, a) // types a, which is the only thing that needs a monkey this frame
	if a.progress != 1 || lineString(a.lines[0]) != "a" || a.row != 0 {
		t.Fatalf("after frame 1: progress=%d row=%d line=%q", a.progress, a.row, lineString(a.lines[0]))
	}
	step(t, a) // freebies \n, é, \n, then the monkey types b
	if a.row != 2 {
		t.Errorf("row = %d, want 2 (two freebie newlines)", a.row)
	}
	if got := lineString(a.lines[1]); got != "é" {
		t.Errorf("line 1 = %q, want the freebie é", got)
	}
	if got := lineString(a.lines[2]); got != "b" {
		t.Errorf("line 2 = %q, want b", got)
	}
	if a.progress != len(a.text) {
		t.Errorf("progress = %d, want %d", a.progress, len(a.text))
	}
	// Freebies are not attributed to a monkey.
	if a.monkeyChars[0] != 'b' {
		t.Errorf("monkeyChars[0] = %q, want b", a.monkeyChars[0])
	}
}

func TestLineWrapsAtColumnCount(t *testing.T) {
	text := "abcdefghij"
	a, _, _ := newTestApp(t, cw, ch, 10, text, text)
	step(t, a)
	if a.row != 1 {
		t.Errorf("row = %d, want 1 after filling 8 columns", a.row)
	}
	if got := lineString(a.lines[0]); got != "abcdefgh" {
		t.Errorf("line 0 = %q, want abcdefgh", got)
	}
	if got := lineString(a.lines[1]); got != "ij" {
		t.Errorf("line 1 = %q, want ij", got)
	}
}

func TestScrollingRingBuffer(t *testing.T) {
	// Eight lines of text on a six-row screen: the first two scroll away.
	var lines []string
	for i := 0; i < 8; i++ {
		lines = append(lines, string(rune('a'+i)))
	}
	text := strings.Join(lines, "\n")
	keys := strings.Join(lines, "")
	a, c, _ := newTestApp(t, cw, ch, 20, text, keys)
	step(t, a)
	if a.progress != len(text) {
		t.Fatalf("progress = %d, want all %d typed", a.progress, len(text))
	}
	if a.row != 7 {
		t.Fatalf("row = %d, want 7", a.row)
	}
	img := frame(t, a, c)
	// Visible lines are c..h in order.
	for i := 0; i < 6; i++ {
		ref := debugText(t, cw, ch, float32(i*8), string(rune('c'+i)))
		assertBand(t, fmt.Sprintf("row %d", i), img, ref, i*8)
	}
}

func TestCaptionShowsElapsedTimeAndFreezesWhenDone(t *testing.T) {
	a, c, clock := newTestApp(t, cw, ch, 1, "ab", "a"+"b")
	clock.advanceSeconds(3600 + 2*60 + 5)
	img := frame(t, a, c)
	assertBand(t, "running caption", img, debugText(t, cw, ch, captionY, "Monkeys: 1 - 1H:2M:5S"), captionY)

	step(t, a) // types a
	step(t, a) // types b: finished
	clock.advanceSeconds(10)
	img = frame(t, a, c)
	// The frame that notices the end latches the time of that frame.
	assertBand(t, "finished caption", img, debugText(t, cw, ch, captionY, "Monkeys: 1 - 1H:2M:15S"), captionY)
	clock.advanceSeconds(1000)
	img = frame(t, a, c)
	assertBand(t, "frozen caption", img, debugText(t, cw, ch, captionY, "Monkeys: 1 - 1H:2M:15S"), captionY)
}

func TestProgressBarGrowsWithProgress(t *testing.T) {
	a, c, _ := newTestApp(t, cw, ch, 2, "abcd", "ab")
	step(t, a) // 2 of 4 characters: half of 64 pixels
	img := frame(t, a, c)
	sdltest.AssertRegion(t, img, image.Rect(0, progressY, 32, ch), green)
	sdltest.AssertRegion(t, img, image.Rect(32, progressY, cw, ch), black)
}

func TestProgressBarFullAtEnd(t *testing.T) {
	a, c, _ := newTestApp(t, cw, ch, 1, "a", "a")
	step(t, a)
	img := frame(t, a, c)
	sdltest.AssertRegion(t, img, image.Rect(0, progressY, cw, ch), green)
}

func TestNothingMoreHappensAfterTheTextIsDone(t *testing.T) {
	a, _, _ := newTestApp(t, cw, ch, 1, "a", "a")
	step(t, a)
	before := fmt.Sprint(a.progress, a.row, lineString(a.lines[0]), string(a.monkeyChars))
	step(t, a) // would panic the script if a monkey played
	if after := fmt.Sprint(a.progress, a.row, lineString(a.lines[0]), string(a.monkeyChars)); after != before {
		t.Errorf("state changed after completion: %q -> %q", before, after)
	}
}

func TestEmptyTextDrawsNoBar(t *testing.T) {
	a, c, _ := newTestApp(t, cw, ch, 3, "", "")
	step(t, a)
	img := frame(t, a, c)
	sdltest.AssertRegion(t, img, image.Rect(0, progressY, cw, ch), black)
}

func TestTinyWindowHasNoLinesButStillWorks(t *testing.T) {
	// 32 pixels high is only 4 character rows: rows = 0, so no text area.
	a, c, _ := newTestApp(t, cw, 32, 2, "ab", "ab")
	if a.lines != nil || a.monkeyChars != nil {
		t.Fatalf("lines=%v monkeyChars=%v, want none", a.lines, a.monkeyChars)
	}
	step(t, a)
	if a.progress != 2 {
		t.Fatalf("progress = %d, want 2 even without a text area", a.progress)
	}
	img := frame(t, a, c)
	// The bar is drawn at y=0 and, with cols=8, spans the full width.
	sdltest.AssertRegion(t, img, image.Rect(0, 0, cw, 8), green)
	sdltest.AssertRegion(t, img, image.Rect(0, 8, cw, 32), black)
}

func TestFreebiesWithoutTextAreaStillAdvance(t *testing.T) {
	a, _, _ := newTestApp(t, cw, 32, 1, "\n\na", "a")
	step(t, a)
	if a.progress != 3 {
		t.Errorf("progress = %d, want 3", a.progress)
	}
}

func TestEventsQuitOnlyOnQuit(t *testing.T) {
	a, _, _ := newTestApp(t, cw, ch, 1, "a", "")
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.CloseRequested()
	in.MouseMotion(1, 1, 0, 0, 0)
	if feedApp(t, a) {
		t.Fatal("non-quit events asked to quit")
	}
	in.Quit()
	if !feedApp(t, a) {
		t.Fatal("EVENT_QUIT did not quit")
	}
}

func feedApp(t *testing.T, a *app) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func TestWindowResizeRelaysOutAndRestartsText(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	win := sdltest.NewWindow(t, 64, 80)
	var r *sdl.Renderer
	sdltest.OnMain(t, func() { r = sdl.CreateRenderer(win, "software") })
	if r == nil {
		t.Fatalf("CreateRenderer: %s", sdl.GetError())
	}
	t.Cleanup(func() { sdltest.OnMain(t, func() { sdl.DestroyRenderer(r) }) })
	clock := &fakeClock{}
	sc := newScript(t, "abc")
	var a *app
	sdltest.OnMain(t, func() { a = newApp(r, 3, "abcdef", clock.now, sc.rand) })
	feedApp(t, a) // drop events from window creation
	step(t, a)
	if a.progress != 3 || a.row != 0 || lineString(a.lines[0]) != "abc" {
		t.Fatalf("setup: progress=%d row=%d line=%q", a.progress, a.row, lineString(a.lines[0]))
	}

	sdltest.ResizeWindow(t, win, 128, 120)
	if feedApp(t, a) {
		t.Fatal("resize asked to quit")
	}
	if a.cols != 16 || a.rows != 11 {
		t.Errorf("layout after resize = %dx%d, want 16x11", a.cols, a.rows)
	}
	if len(a.lines) != 11 || len(a.monkeyChars) != 16 {
		t.Errorf("buffers = %d lines, %d monkeyChars", len(a.lines), len(a.monkeyChars))
	}
	// The screen restarts but the typing progress is kept.
	if a.row != 0 || len(a.lines[0]) != 0 || a.progress != 3 {
		t.Errorf("after resize: row=%d line0=%q progress=%d", a.row, lineString(a.lines[0]), a.progress)
	}
	if string(a.monkeyChars) != strings.Repeat(" ", 16) {
		t.Errorf("monkeyChars = %q, want blanks", string(a.monkeyChars))
	}

	// Shrinking below five rows removes the text area entirely.
	sdltest.ResizeWindow(t, win, 128, 32)
	feedApp(t, a)
	if a.lines != nil || a.monkeyChars != nil {
		t.Errorf("tiny window kept lines=%d monkeyChars=%d", len(a.lines), len(a.monkeyChars))
	}
}

func TestSeededRandomnessIsDeterministicAndProgresses(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	progress := func() int {
		var a *app
		sdltest.OnMain(t, func() {
			sdl.Srand(42)
			a = newApp(c.Renderer, 5000, defaultText, (&fakeClock{}).now, sdl.Rand)
			for i := 0; i < 3; i++ {
				a.advance()
			}
		})
		return a.progress
	}
	p1, p2 := progress(), progress()
	if p1 != p2 {
		t.Errorf("same seed gave progress %d then %d", p1, p2)
	}
	if p1 <= 0 || p1 > len(defaultText) {
		t.Errorf("progress = %d, want in (0, %d]", p1, len(defaultText))
	}
}

func TestDefaultLayoutAt640x480(t *testing.T) {
	a, _, _ := newTestApp(t, 640, 480, 100, defaultText, "")
	if a.cols != 80 || a.rows != 56 {
		t.Fatalf("layout = %dx%d, want 80x56", a.cols, a.rows)
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	file := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(file, []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"", file} {
		var err error
		sdltest.OnMain(t, func() { err = run(3, 10, f) })
		if err != nil {
			t.Errorf("run(file=%q) = %v", f, err)
		}
	}
	var err error
	sdltest.OnMain(t, func() { err = run(1, 1, filepath.Join(t.TempDir(), "missing.txt")) })
	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("run with a missing file = %v, want an error naming the file", err)
	}
}
