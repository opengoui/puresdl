package main

import (
	"fmt"
	"image"
	imgcolor "image/color"
	"os"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// feed pumps SDL and applies every queued event to a like the run loop does.
// It returns true if the program asked to quit.
func feed(t *testing.T, a *app) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if !a.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func setup(t *testing.T) (*app, *sdltest.Clock, *sdltest.Input) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	var clock sdltest.Clock
	var a *app
	sdltest.OnMain(t, func() { a = newApp(clock.Ticks) })
	return a, &clock, sdltest.NewInput(t, 0)
}

func texts(a *app) []string {
	var out []string
	for _, m := range a.messages {
		out = append(out, m.str)
	}
	return out
}

func last(t *testing.T, a *app) *eventMessage {
	t.Helper()
	if len(a.messages) == 0 {
		t.Fatal("no messages")
	}
	return a.messages[len(a.messages)-1]
}

func handle(t *testing.T, a *app, e *sdl.Event) bool {
	t.Helper()
	var ok bool
	sdltest.OnMain(t, func() { ok = a.handleEvent(e) })
	return ok
}

func TestInitialMessageIsHintInWhite(t *testing.T) {
	a, _, _ := setup(t)
	if got := texts(a); len(got) != 1 || got[0] != "Please plug in a joystick." {
		t.Fatalf("messages = %q", got)
	}
	if a.messages[0].color != (color{255, 255, 255, 255}) {
		t.Errorf("hint colour = %v, want opaque white", a.messages[0].color)
	}
	for i := 1; i < len(a.colors); i++ {
		if a.colors[i].a != 255 {
			t.Fatalf("colors[%d].a = %d, want 255", i, a.colors[i].a)
		}
	}
}

func TestQuitEventQuitsButEscapeDoesNot(t *testing.T) {
	a, _, in := setup(t)
	in.Press(sdl.SCANCODE_ESCAPE)
	if feed(t, a) {
		t.Error("Escape quit; the C program only quits on EVENT_QUIT")
	}
	in.Quit()
	if !feed(t, a) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestUnrelatedEventsAreIgnored(t *testing.T) {
	a, _, in := setup(t)
	in.MouseMotion(1, 2, 3, 4, 0)
	in.Text("x")
	if feed(t, a) {
		t.Fatal("unexpected quit")
	}
	if len(a.messages) != 1 {
		t.Errorf("messages = %q, want only the hint", texts(a))
	}
}

func TestHatStateStrings(t *testing.T) {
	for _, tc := range []struct {
		v    uint8
		want string
	}{
		{sdl.HAT_CENTERED, "CENTERED"}, {sdl.HAT_UP, "UP"}, {sdl.HAT_RIGHT, "RIGHT"},
		{sdl.HAT_DOWN, "DOWN"}, {sdl.HAT_LEFT, "LEFT"}, {sdl.HAT_RIGHTUP, "RIGHT+UP"},
		{sdl.HAT_RIGHTDOWN, "RIGHT+DOWN"}, {sdl.HAT_LEFTUP, "LEFT+UP"},
		{sdl.HAT_LEFTDOWN, "LEFT+DOWN"}, {0xFF, "UNKNOWN"}, {sdl.HAT_UP | sdl.HAT_DOWN, "UNKNOWN"},
	} {
		if got := hatStateString(tc.v); got != tc.want {
			t.Errorf("hatStateString(%#x) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestBatteryStateStrings(t *testing.T) {
	for _, tc := range []struct {
		s    sdl.PowerState
		want string
	}{
		{sdl.POWERSTATE_ERROR, "ERROR"}, {sdl.POWERSTATE_UNKNOWN, "UNKNOWN"},
		{sdl.POWERSTATE_ON_BATTERY, "ON BATTERY"}, {sdl.POWERSTATE_NO_BATTERY, "NO BATTERY"},
		{sdl.POWERSTATE_CHARGING, "CHARGING"}, {sdl.POWERSTATE_CHARGED, "CHARGED"},
		{sdl.PowerState(99), "UNKNOWN"},
	} {
		if got := batteryStateString(tc.s); got != tc.want {
			t.Errorf("batteryStateString(%d) = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestVirtualJoystickPlugButtonsHatAxisAndUnplug(t *testing.T) {
	a, clock, _ := setup(t)
	vj := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Name: "Test Stick", Axes: 3, Buttons: 4, Hats: 1})
	feed(t, a)
	id := vj.ID
	want := fmt.Sprintf("Joystick #%d ('Test Stick') added", id)
	if got := last(t, a).str; got != want {
		// Initial-state axis events may follow the add; the add must exist.
		found := false
		for _, s := range texts(a) {
			found = found || s == want
		}
		if !found {
			t.Fatalf("messages = %q, missing %q", texts(a), want)
		}
	}
	if m := a.messages[1]; m.color != a.colors[int(id)%64] {
		t.Errorf("message colour = %v, want colors[%d] = %v", m.color, id, a.colors[int(id)%64])
	}

	check := func(want string) {
		t.Helper()
		for _, s := range texts(a) {
			if s == want {
				return
			}
		}
		t.Errorf("messages = %q, missing %q", texts(a), want)
	}

	vj.SetButton(2, true)
	feed(t, a)
	check(fmt.Sprintf("Joystick #%d button 2 -> PRESSED", id))
	vj.SetButton(2, false)
	feed(t, a)
	check(fmt.Sprintf("Joystick #%d button 2 -> RELEASED", id))

	for _, h := range []struct {
		v    uint8
		name string
	}{{sdl.HAT_RIGHTUP, "RIGHT+UP"}, {sdl.HAT_LEFT, "LEFT"}, {sdl.HAT_CENTERED, "CENTERED"}} {
		vj.SetHat(0, h.v)
		feed(t, a)
		check(fmt.Sprintf("Joystick #%d hat 0 -> %s", id, h.name))
	}

	// SDL emits initial-state axis events lazily on the first axis update;
	// let them take (and then expire) the motion cooldown before the real test.
	vj.SetAxis(1, 3000)
	feed(t, a)
	clock.Advance(40)
	vj.SetAxis(1, 12000)
	feed(t, a)
	check(fmt.Sprintf("Joystick #%d axis 1 -> 12000", id))

	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(id) })
	feed(t, a)
	check(fmt.Sprintf("Joystick #%d removed", id))
	sdltest.OnMain(t, func() {
		// The handler dropped its reference; ours (the harness's) remains
		// until cleanup, so only the device list is observable here.
		if n := len(sdl.GetJoysticks()); n != 0 {
			t.Errorf("joystick list has %d entries after unplug", n)
		}
	})
}

func TestGamepadIsAlsoReportedAsJoystick(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK|sdl.INIT_GAMEPAD)
	var clock sdltest.Clock
	var a *app
	sdltest.OnMain(t, func() { a = newApp(clock.Ticks) })
	gp := sdltest.AttachGamepad(t, "Pad")
	feed(t, a)
	gp.SetGamepadButton(sdl.GAMEPAD_BUTTON_SOUTH, true)
	feed(t, a)
	want := fmt.Sprintf("Joystick #%d button %d -> PRESSED", gp.ID, sdl.GAMEPAD_BUTTON_SOUTH)
	found := false
	for _, s := range texts(a) {
		found = found || s == want
	}
	if !found {
		t.Errorf("messages = %q, missing %q", texts(a), want)
	}
}

func TestInjectedEventFieldsRoundTrip(t *testing.T) {
	a, _, in := setup(t)
	in.JoyButton(7, 200, true)
	in.JoyHat(7, 3, sdl.HAT_LEFTDOWN)
	in.JoyAxis(7, 5, -32768)
	feed(t, a)
	got := texts(a)[1:]
	want := []string{
		"Joystick #7 button 200 -> PRESSED",
		"Joystick #7 hat 3 -> LEFT+DOWN",
		"Joystick #7 axis 5 -> -32768",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("messages = %q, want %q", got, want)
	}
	if a.messages[1].color != a.colors[7] {
		t.Errorf("colour = %v, want colors[7]", a.messages[1].color)
	}
}

func TestColorIndexWrapsAtTableSize(t *testing.T) {
	a, _, in := setup(t)
	in.JoyHat(64+9, 0, sdl.HAT_UP)
	feed(t, a)
	if last(t, a).color != a.colors[9] {
		t.Errorf("id 73 colour = %v, want colors[9] = %v", last(t, a).color, a.colors[9])
	}
}

func TestAxisMotionCooldown(t *testing.T) {
	a, clock, in := setup(t)
	push := func(v int16) { in.JoyAxis(1, 0, v); feed(t, a) }

	push(1) // first event passes (cooldown starts at 0)
	push(2) // same tick: suppressed
	clock.Advance(39)
	push(3) // 39 ms: still suppressed
	clock.Advance(1)
	push(4) // 40 ms: passes
	var got []string
	for _, s := range texts(a)[1:] {
		got = append(got, s)
	}
	want := []string{"Joystick #1 axis 0 -> 1", "Joystick #1 axis 0 -> 4"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestBallMotionCooldownIsIndependentOfAxis(t *testing.T) {
	a, clock, in := setup(t)
	in.JoyAxis(1, 0, 10)
	feed(t, a)
	ball := func(x, y int16) {
		e := new(sdl.Event)
		e.SetType(sdl.EVENT_JOYSTICK_BALL_MOTION)
		b := e.Jball()
		b.Which, b.Ball, b.Xrel, b.Yrel = 2, 1, x, y
		handle(t, a, e)
	}
	ball(1, 2) // axis cooldown is active but ball has its own
	ball(3, 4) // suppressed
	clock.Advance(39)
	ball(5, 6) // 39 ms: still suppressed
	clock.Advance(1)
	ball(7, 8) // 40 ms: passes
	want := []string{"Joystick #1 axis 0 -> 10", "Joystick #2 ball 1 -> 1, 2", "Joystick #2 ball 1 -> 7, 8"}
	if got := texts(a)[1:]; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestBatteryUpdateMessage(t *testing.T) {
	a, _, _ := setup(t)
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_JOYSTICK_BATTERY_UPDATED)
	b := e.Jbattery()
	b.Which, b.State, b.Percent = 4, sdl.POWERSTATE_CHARGING, 85
	if !handle(t, a, e) {
		t.Fatal("battery event quit")
	}
	if got, want := last(t, a).str, "Joystick #4 battery -> CHARGING - 85%"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestAddOfUnopenableJoystickReportsError(t *testing.T) {
	a, _, _ := setup(t)
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_JOYSTICK_ADDED)
	e.Jdevice().Which = 4242
	handle(t, a, e)
	if got := last(t, a).str; !strings.HasPrefix(got, "Joystick #4242 add, but not opened: ") {
		t.Errorf("message = %q", got)
	}
	e = new(sdl.Event)
	e.SetType(sdl.EVENT_JOYSTICK_REMOVED)
	e.Jdevice().Which = 4242
	handle(t, a, e)
	if got := last(t, a).str; got != "Joystick #4242 removed" {
		t.Errorf("message = %q", got)
	}
}

func TestDrawFadesAndExpiresMessages(t *testing.T) {
	a, clock, _ := setup(t)
	c := sdltest.NewCanvas(t, 640, 480)
	draw := func() *image.RGBA {
		sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
		return c.Capture(t)
	}
	inkBounds := func(img *image.RGBA) (r image.Rectangle) {
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				if sdltest.PixelAt(img, x, y) != (imgcolor.RGBA{0, 0, 0, 255}) {
					r = r.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return r
	}

	// Life 0: the 26-character hint is centred horizontally at the top.
	img := draw()
	b := inkBounds(img)
	textW := len("Please plug in a joystick.") * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	box := image.Rect((640-textW)/2, 0, (640+textW)/2, sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)
	if b.Empty() || !b.In(box) {
		t.Fatalf("ink bounds %v, want non-empty and inside %v", b, box)
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < 640; x++ {
			if p := sdltest.PixelAt(img, x, y); p != (imgcolor.RGBA{0, 0, 0, 255}) && p != (imgcolor.RGBA{255, 255, 255, 255}) {
				t.Fatalf("pixel (%d,%d) = %v at full life, want pure white ink", x, y, p)
			}
		}
	}

	// Half way through its life the message sits half way down the window.
	clock.Advance(1750)
	b = inkBounds(draw())
	box = image.Rect((640-textW)/2, 240, (640+textW)/2, 240+sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)
	if b.Empty() || !b.In(box) {
		t.Fatalf("mid-life ink bounds %v, want inside %v", b, box)
	}

	// One millisecond before expiry it is still alive; at the lifetime it is gone.
	clock.Advance(1749)
	draw()
	if len(a.messages) != 1 {
		t.Fatalf("message dropped before its lifetime: %d left", len(a.messages))
	}
	clock.Advance(1)
	img = draw()
	if len(a.messages) != 0 {
		t.Errorf("expired message not removed: %q", texts(a))
	}
	if b := inkBounds(img); !b.Empty() {
		t.Errorf("expired frame has ink at %v", b)
	}
}

func TestDrawCentresByWindowWidthAndUsesJoystickColour(t *testing.T) {
	a, _, in := setup(t)
	a.messages = nil
	in.JoyHat(5, 0, sdl.HAT_UP)
	feed(t, a)
	c := sdltest.NewCanvas(t, 800, 100)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 800, 100) })
	img := c.Capture(t)
	m := a.messages[0]
	want := imgcolor.RGBA{m.color.r, m.color.g, m.color.b, 255}
	textW := len(m.str) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	box := image.Rect((800-textW)/2, 0, (800+textW)/2, sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)
	ink := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 800; x++ {
			p := sdltest.PixelAt(img, x, y)
			if p == (imgcolor.RGBA{0, 0, 0, 255}) {
				continue
			}
			ink++
			if !image.Pt(x, y).In(box) {
				t.Fatalf("ink at (%d,%d) outside %v", x, y, box)
			}
			if p != want {
				t.Fatalf("ink at (%d,%d) = %v, want %v", x, y, p, want)
			}
		}
	}
	if ink == 0 {
		t.Error("nothing drawn")
	}
}

func TestCrowdedMessagesWaitForThePreviousOne(t *testing.T) {
	a, clock, _ := setup(t)
	clock.Advance(5)
	a.addMessage(1, "B")
	a.addMessage(1, "C")
	clock.Advance(5) // now=10: hint is ~1.4px down, B would overlap it
	c := sdltest.NewCanvas(t, 640, 480)
	sdltest.OnMain(t, func() { a.draw(c.Renderer, 640, 480) })
	if got := a.messages[1].startTicks; got != 10 {
		t.Errorf("B.startTicks = %d, want restarted at 10", got)
	}
	if got := a.messages[2].startTicks; got != 5 {
		t.Errorf("C.startTicks = %d, want untouched 5 (loop stops at B)", got)
	}
	if got := a.messages[0].startTicks; got != 0 {
		t.Errorf("hint.startTicks = %d, want 0", got)
	}
}

func TestIterateFollowsResizedWindow(t *testing.T) {
	a, _, _ := setup(t)
	var err string
	sdltest.OnMain(t, func() {
		if !sdl.CreateWindowAndRenderer("t", 640, 480, sdl.WINDOW_HIDDEN|sdl.WINDOW_RESIZABLE, &a.window, &a.renderer) {
			err = sdl.GetError()
		}
	})
	if err != "" {
		t.Skip("cannot create window/renderer:", err)
	}
	t.Cleanup(func() {
		sdltest.OnMain(t, func() { sdl.DestroyRenderer(a.renderer); sdl.DestroyWindow(a.window) })
	})
	sdltest.ResizeWindow(t, a.window, 800, 300)
	sdltest.OnMain(t, a.iterate)
	img := sdltest.Capture(t, a.renderer)
	textW := len(a.messages[0].str) * sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE
	box := image.Rect((800-textW)/2, 0, (800+textW)/2, sdl.DEBUG_TEXT_FONT_CHARACTER_SIZE)
	ink := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if sdltest.PixelAt(img, x, y) != (imgcolor.RGBA{0, 0, 0, 255}) {
				ink++
				if !image.Pt(x, y).In(box) {
					t.Fatalf("ink at (%d,%d) outside %v (image %v)", x, y, box, img.Bounds())
				}
			}
		}
	}
	if ink == 0 {
		t.Error("nothing drawn")
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatal(err)
	}
}
