package main

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	red       = color.RGBA{255, 0, 0, 255}
	green     = color.RGBA{0, 255, 0, 255}
	blue      = color.RGBA{0, 0, 255, 255}
	white     = color.RGBA{255, 255, 255, 255}
	black     = color.RGBA{0, 0, 0, 255}
	pasteBg   = color.RGBA{0, 53, 25, 255}
	pasteText = color.RGBA{0, 219, 107, 255}
	timeText  = color.RGBA{255, 255, 0, 255}
)

const fixedTime = "Monday, January 2, 2006   15:04:05"

// fixture is an app drawing on an offscreen canvas of the logical size.
type fixture struct {
	a  *app
	c  *sdltest.Canvas
	in *sdltest.Input
}

func setup(t *testing.T, w, h int32) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, w, h)
	ok := sdltest.Call(t, func() bool {
		return sdl.SetRenderLogicalPresentation(c.Renderer, 640, 480, sdl.LOGICAL_PRESENTATION_LETTERBOX)
	})
	if !ok {
		t.Fatalf("SetRenderLogicalPresentation: %s", sdl.GetError())
	}
	a := &app{renderer: c.Renderer, timeString: func() string { return fixedTime }}
	a.layout()
	a.currentTime = a.timeString()
	return &fixture{a: a, c: c, in: sdltest.NewInput(t, 0)}
}

// feed applies every queued event like the run loop; true means quit.
func (f *fixture) feed(t *testing.T) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if f.a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func (f *fixture) draw(t *testing.T) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, f.a.draw)
	return f.c.Capture(t)
}

// click performs a left press and release at the given logical positions.
func (f *fixture) click(t *testing.T, downX, downY, upX, upY float32) {
	t.Helper()
	f.in.MouseButton(sdl.BUTTON_LEFT, true, downX, downY)
	f.feed(t)
	f.in.MouseButton(sdl.BUTTON_LEFT, false, upX, upY)
	f.feed(t)
}

func center(r sdl.FRect) (float32, float32) { return r.X + r.W/2, r.Y + r.H/2 }

// inner is a pixel inside the filled area, clear of the frame and the label.
func inner(r sdl.FRect) (int, int) { return int(r.X) + 2, int(r.Y) + 2 }

func clipboard(t *testing.T) string {
	return sdltest.Call(t, sdl.GetClipboardText)
}

func countColor(img *image.RGBA, r image.Rectangle, c color.RGBA) (n int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) == c {
				n++
			}
		}
	}
	return n
}

func TestLayoutMatchesC(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	a := &app{}
	a.layout()
	want := map[string][2]sdl.FRect{
		"time":  {a.currentTimeRect, {X: 30, Y: 10, W: 390, H: 18}},
		"copy":  {a.copyButtonRect, {X: 450, Y: 10, W: 162, H: 18}},
		"text":  {a.pasteTextRect, {X: 10, Y: 38, W: 620, H: 404}},
		"paste": {a.pasteButtonRect, {X: 235, Y: 452, W: 170, H: 18}},
	}
	for name, p := range want {
		if p[0] != p[1] {
			t.Errorf("%s rect = %+v, want %+v", name, p[0], p[1])
		}
	}
}

func TestFormatTime(t *testing.T) {
	dt := sdl.DateTime{Year: 2006, Month: 1, Day: 2, Hour: 3, Minute: 4, Second: 5, DayOfWeek: 1}
	if got, want := formatTime(&dt), "Monday, January 2, 2006   03:04:05"; got != want {
		t.Errorf("formatTime = %q, want %q", got, want)
	}
	dt = sdl.DateTime{Year: 1999, Month: 12, Day: 31, Hour: 23, Minute: 59, Second: 59, DayOfWeek: 5}
	if got, want := formatTime(&dt), "Friday, December 31, 1999   23:59:59"; got != want {
		t.Errorf("formatTime = %q, want %q", got, want)
	}
}

func TestCurrentTimeStringIsReal(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	s := currentTimeString()
	if s == "" || len(s) > 63 {
		t.Errorf("currentTimeString = %q", s)
	}
}

func TestInitialFrame(t *testing.T) {
	f := setup(t, 640, 480)
	img := f.draw(t)
	a := f.a
	sdltest.AssertPixel(t, img, 1, 1, black)
	x, y := inner(a.currentTimeRect)
	sdltest.AssertPixel(t, img, x, y, blue)
	sdltest.AssertPixel(t, img, int(a.currentTimeRect.X), int(a.currentTimeRect.Y), white)
	x, y = inner(a.copyButtonRect)
	sdltest.AssertPixel(t, img, x, y, red)
	x, y = inner(a.pasteButtonRect)
	sdltest.AssertPixel(t, img, x, y, red)
	x, y = inner(a.pasteTextRect)
	sdltest.AssertPixel(t, img, x, y, pasteBg)
	tr := a.currentTimeRect
	rect := image.Rect(int(tr.X), int(tr.Y), int(tr.X+tr.W), int(tr.Y+tr.H))
	if countColor(img, rect, timeText) == 0 {
		t.Error("clock text not drawn in yellow")
	}
	// Nothing pasted yet: no pasted-text colour anywhere.
	if n := countColor(img, image.Rect(0, 0, 640, 480), pasteText); n != 0 {
		t.Errorf("%d pasted-text pixels before any paste", n)
	}
}

func TestClockTextIsCentredInFrame(t *testing.T) {
	f := setup(t, 640, 480)
	img := f.draw(t)
	tr := f.a.currentTimeRect
	startX := int(tr.X + (tr.W-float32(charSize*len(fixedTime)))/2)
	endX := startX + charSize*len(fixedTime)
	minX, maxX := 1<<30, -1
	for y := int(tr.Y); y < int(tr.Y+tr.H); y++ {
		for x := int(tr.X) + 1; x < int(tr.X+tr.W)-1; x++ {
			if sdltest.PixelAt(img, x, y) == timeText {
				minX, maxX = min(minX, x), max(maxX, x)
			}
		}
	}
	if minX < startX || maxX >= endX || maxX < endX-2*charSize {
		t.Errorf("text spans x %d..%d, want within %d..%d", minX, maxX, startX, endX)
	}
}

func TestCopyButtonCopiesCurrentTime(t *testing.T) {
	f := setup(t, 640, 480)
	sdltest.OnMain(t, func() { sdl.SetClipboardText("old") })
	cx, cy := center(f.a.copyButtonRect)

	f.in.MouseButton(sdl.BUTTON_LEFT, true, cx, cy)
	f.feed(t)
	x, y := inner(f.a.copyButtonRect)
	sdltest.AssertPixel(t, f.draw(t), x, y, green)
	if got := clipboard(t); got != "old" {
		t.Errorf("clipboard changed on press: %q", got)
	}
	f.in.MouseButton(sdl.BUTTON_LEFT, false, cx, cy)
	f.feed(t)
	sdltest.AssertPixel(t, f.draw(t), x, y, red)
	if got := clipboard(t); got != fixedTime {
		t.Errorf("clipboard = %q, want %q", got, fixedTime)
	}
}

func TestPasteButtonShowsClipboard(t *testing.T) {
	f := setup(t, 640, 480)
	sdltest.OnMain(t, func() { sdl.SetClipboardText("hello") })
	px, py := center(f.a.pasteButtonRect)
	f.in.MouseButton(sdl.BUTTON_LEFT, true, px, py)
	f.feed(t)
	x, y := inner(f.a.pasteButtonRect)
	img := f.draw(t)
	sdltest.AssertPixel(t, img, x, y, green)
	if n := countColor(img, image.Rect(0, 0, 640, 480), pasteText); n != 0 {
		t.Error("text pasted before release")
	}
	f.in.MouseButton(sdl.BUTTON_LEFT, false, px, py)
	f.feed(t)
	img = f.draw(t)
	sdltest.AssertPixel(t, img, x, y, red)
	tr := f.a.pasteTextRect
	rect := image.Rect(int(tr.X), int(tr.Y), int(tr.X+tr.W), int(tr.Y+tr.H))
	if countColor(img, rect, pasteText) == 0 {
		t.Error("pasted text not drawn")
	}
	if f.a.pastedStr != "hello" {
		t.Errorf("pastedStr = %q", f.a.pastedStr)
	}
}

func TestCopyThenPasteRoundTrip(t *testing.T) {
	f := setup(t, 640, 480)
	cx, cy := center(f.a.copyButtonRect)
	f.click(t, cx, cy, cx, cy)
	px, py := center(f.a.pasteButtonRect)
	f.click(t, px, py, px, py)
	if f.a.pastedStr != fixedTime {
		t.Errorf("pastedStr = %q, want %q", f.a.pastedStr, fixedTime)
	}
}

func TestPressMustReleaseOnSameButton(t *testing.T) {
	f := setup(t, 640, 480)
	sdltest.OnMain(t, func() { sdl.SetClipboardText("keep") })
	cx, cy := center(f.a.copyButtonRect)
	px, py := center(f.a.pasteButtonRect)

	// Press copy, release outside any button: nothing happens, press clears.
	f.click(t, cx, cy, 5, 300)
	if got := clipboard(t); got != "keep" {
		t.Errorf("clipboard = %q after cancelled copy", got)
	}
	if f.a.copyPressed {
		t.Error("copyPressed stuck after release elsewhere")
	}
	// Press copy, release on paste: neither action runs.
	f.click(t, cx, cy, px, py)
	if got := clipboard(t); got != "keep" || f.a.hasPasted {
		t.Errorf("cross-button release acted: clipboard=%q pasted=%v", got, f.a.hasPasted)
	}
	// Press paste, release on copy: neither action runs.
	f.click(t, px, py, cx, cy)
	if got := clipboard(t); got != "keep" || f.a.hasPasted {
		t.Errorf("cross-button release acted: clipboard=%q pasted=%v", got, f.a.hasPasted)
	}
	// Release without a prior press on a button does nothing.
	f.click(t, 5, 300, cx, cy)
	f.click(t, 5, 300, px, py)
	if got := clipboard(t); got != "keep" || f.a.hasPasted {
		t.Errorf("release without press acted: clipboard=%q pasted=%v", got, f.a.hasPasted)
	}
}

func TestNonLeftButtonsIgnored(t *testing.T) {
	f := setup(t, 640, 480)
	cx, cy := center(f.a.copyButtonRect)
	for _, b := range []uint8{sdl.BUTTON_RIGHT, sdl.BUTTON_MIDDLE} {
		f.in.MouseButton(b, true, cx, cy)
		f.feed(t)
		if f.a.copyPressed {
			t.Errorf("button %d pressed the copy button", b)
		}
	}
	// A right release must not clear a left press in progress.
	f.in.MouseButton(sdl.BUTTON_LEFT, true, cx, cy)
	f.in.MouseButton(sdl.BUTTON_RIGHT, false, 0, 0)
	f.feed(t)
	if !f.a.copyPressed {
		t.Error("right-button release cleared the left press")
	}
}

func TestPressElsewhereClearsPressedState(t *testing.T) {
	f := setup(t, 640, 480)
	cx, cy := center(f.a.copyButtonRect)
	f.in.MouseButton(sdl.BUTTON_LEFT, true, cx, cy)
	f.feed(t)
	f.in.MouseButton(sdl.BUTTON_LEFT, true, 5, 300)
	f.feed(t)
	if f.a.copyPressed || f.a.pastePressed {
		t.Errorf("pressed state after pressing elsewhere: %v %v", f.a.copyPressed, f.a.pastePressed)
	}
}

func TestButtonHitBoxEdges(t *testing.T) {
	f := setup(t, 640, 480)
	r := f.a.copyButtonRect
	for _, tc := range []struct {
		x, y float32
		hit  bool
	}{
		{r.X, r.Y, true},
		{r.X + r.W - 0.5, r.Y + r.H - 0.5, true},
		{r.X - 1, r.Y + 1, false},
		{r.X + r.W + 1, r.Y + 1, false},
		{r.X + 1, r.Y - 1, false},
		{r.X + 1, r.Y + r.H + 1, false},
	} {
		f.in.MouseButton(sdl.BUTTON_LEFT, true, tc.x, tc.y)
		f.feed(t)
		if f.a.copyPressed != tc.hit {
			t.Errorf("press at (%v,%v): copyPressed=%v want %v", tc.x, tc.y, f.a.copyPressed, tc.hit)
		}
		f.in.MouseButton(sdl.BUTTON_LEFT, false, tc.x, tc.y)
		f.feed(t)
	}
}

func TestLetterboxedCoordinatesAreConverted(t *testing.T) {
	// 1280x480 target with a 640x480 logical size: scale 1, offset 320 in x.
	f := setup(t, 1280, 480)
	cx, cy := center(f.a.copyButtonRect)
	f.in.MouseButton(sdl.BUTTON_LEFT, true, cx+320, cy)
	f.feed(t)
	if !f.a.copyPressed {
		t.Error("click on the letterboxed button did not hit it")
	}
	f.in.MouseButton(sdl.BUTTON_LEFT, false, cx+320, cy)
	f.feed(t)
	// Unconverted coordinates must not hit.
	f.in.MouseButton(sdl.BUTTON_LEFT, true, cx, cy)
	f.feed(t)
	if f.a.copyPressed {
		t.Error("window coordinates hit the button without conversion")
	}
}

func TestLetterboxBarsStayBlack(t *testing.T) {
	f := setup(t, 1280, 480)
	sdltest.OnMain(t, f.a.draw)
	// Capture is cropped to the logical size while presentation is active, so
	// switch it off to read the whole target.
	sdltest.OnMain(t, func() {
		sdl.SetRenderLogicalPresentation(f.c.Renderer, 0, 0, sdl.LOGICAL_PRESENTATION_DISABLED)
	})
	img := f.c.Capture(t)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, 320, 480), black)
	sdltest.AssertRegion(t, img, image.Rect(960, 0, 1280, 480), black)
	x, y := inner(f.a.copyButtonRect)
	sdltest.AssertPixel(t, img, x+320, y, red)
}

func TestQuitEvent(t *testing.T) {
	f := setup(t, 640, 480)
	f.in.Quit()
	if !f.feed(t) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestOtherEventsDoNotQuit(t *testing.T) {
	f := setup(t, 640, 480)
	f.in.Press(sdl.SCANCODE_ESCAPE) // the C example has no Escape handling
	f.in.MouseMotion(10, 10, 1, 1, 0)
	f.in.MouseWheel(0, 1)
	f.in.Resized(800, 600)
	f.in.CloseRequested()
	f.in.ClipboardUpdate()
	if f.feed(t) {
		t.Error("an event other than EVENT_QUIT quit the app")
	}
}

func TestPasteMultilineText(t *testing.T) {
	f := setup(t, 640, 480)
	f.a.pastedStr, f.a.hasPasted = "##\n\n##", true
	img := f.draw(t)
	tr := f.a.pasteTextRect
	rows := map[int]bool{}
	for y := int(tr.Y) + 1; y < int(tr.Y+tr.H)-1; y++ {
		for x := int(tr.X) + 1; x < int(tr.X+tr.W)-1; x++ {
			if sdltest.PixelAt(img, x, y) == pasteText {
				rows[y-int(tr.Y)] = true
			}
		}
	}
	// Lines start at y offsets 5, 17 (blank), 29 (each line is 8+2 high plus
	// the blank line's step), so ink appears in [5,13) and [25,33) only.
	for y := range rows {
		if !(y >= 5 && y < 13) && !(y >= 25 && y < 33) {
			t.Errorf("unexpected ink at row offset %d", y)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no text drawn")
	}
}

func TestCRLFMatchesLF(t *testing.T) {
	f := setup(t, 640, 480)
	f.a.pastedStr, f.a.hasPasted = "ab\ncd", true
	lf := f.draw(t)
	f.a.pastedStr = "ab\r\ncd"
	crlf := f.draw(t)
	for i := range lf.Pix {
		if lf.Pix[i] != crlf.Pix[i] {
			t.Fatal("CRLF rendering differs from LF rendering")
		}
	}
}

func TestLongLineTruncated(t *testing.T) {
	f := setup(t, 640, 480)
	long := make([]byte, 200)
	for i := range long {
		long[i] = '#'
	}
	f.a.pastedStr, f.a.hasPasted = string(long), true
	img := f.draw(t)
	tr := f.a.pasteTextRect
	maxChars := int((tr.W - 10) / charSize)
	if maxChars != 76 {
		t.Fatalf("maxChars = %d", maxChars)
	}
	limit := int(tr.X) + 5 + maxChars*charSize
	inside := image.Rect(int(tr.X)+5, int(tr.Y), limit, int(tr.Y+tr.H))
	beyond := image.Rect(limit, int(tr.Y)+1, int(tr.X+tr.W)-1, int(tr.Y+tr.H)-1)
	if countColor(img, inside, pasteText) == 0 {
		t.Error("no text drawn")
	}
	if n := countColor(img, beyond, pasteText); n != 0 {
		t.Errorf("%d text pixels past the %d-character limit", n, maxChars)
	}
}

func TestTooManyLinesAreCut(t *testing.T) {
	f := setup(t, 640, 480)
	var s []byte
	for i := 0; i < 60; i++ {
		s = append(s, "#\n"...)
	}
	f.a.pastedStr, f.a.hasPasted = string(s), true
	img := f.draw(t)
	tr := f.a.pasteTextRect
	lastRow := -1
	for y := int(tr.Y) + 1; y < int(tr.Y+tr.H)-1; y++ {
		if countColor(img, image.Rect(int(tr.X)+1, y, int(tr.X+tr.W)-1, y+1), pasteText) > 0 {
			lastRow = y - int(tr.Y)
		}
	}
	// As in the C code, the remaining height h-y compares the area height with
	// an absolute y, so drawing stops after line 36: it starts at offset
	// 5+35*10 = 355 and no further line is drawn.
	if lastRow < 355 || lastRow >= 355+charSize {
		t.Errorf("last ink row offset %d, want in [355,%d)", lastRow, 355+charSize)
	}
}

func TestEmptyPasteClearsText(t *testing.T) {
	f := setup(t, 640, 480)
	f.a.pastedStr, f.a.hasPasted = "text", true
	sdltest.OnMain(t, func() { sdl.SetClipboardText("") })
	px, py := center(f.a.pasteButtonRect)
	f.click(t, px, py, px, py)
	img := f.draw(t)
	if n := countColor(img, image.Rect(0, 0, 640, 480), pasteText); n != 0 {
		t.Errorf("%d text pixels after pasting an empty clipboard", n)
	}
}

func TestIterateUsesTimeSource(t *testing.T) {
	f := setup(t, 640, 480)
	f.a.timeString = func() string { return "tick" }
	sdltest.OnMain(t, f.a.iterate)
	if f.a.currentTime != "tick" {
		t.Errorf("currentTime = %q", f.a.currentTime)
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
