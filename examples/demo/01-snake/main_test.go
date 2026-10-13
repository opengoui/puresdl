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

const startX, startY = snakeGameWidth / 2, snakeGameHeight / 2

// feed pumps SDL and applies every queued event to g like the run loop does.
// It returns true if the game asked to quit.
func feed(t *testing.T, g *game) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if g.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func setup(t *testing.T, flags sdl.InitFlags) (*game, *sdltest.Clock, *sdltest.Input) {
	t.Helper()
	sdltest.Setup(t, flags)
	var clock sdltest.Clock
	g := newGame(clock.Ticks)
	t.Cleanup(func() { sdltest.OnMain(t, g.close) })
	return g, &clock, sdltest.NewInput(t, 0)
}

func TestKeysQuit(t *testing.T) {
	for _, sc := range []sdl.Scancode{sdl.SCANCODE_ESCAPE, sdl.SCANCODE_Q} {
		g, _, in := setup(t, sdl.INIT_VIDEO)
		in.Press(sc)
		if !feed(t, g) {
			t.Errorf("scancode %d did not quit", sc)
		}
	}
	g, _, in := setup(t, sdl.INIT_VIDEO)
	in.Quit()
	if !feed(t, g) {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestArrowKeysSteerTheHead(t *testing.T) {
	for _, tc := range []struct {
		sc     sdl.Scancode
		dx, dy int8
		name   string
	}{
		{sc: sdl.SCANCODE_UP, dx: 0, dy: -1, name: "up"},
		{sc: sdl.SCANCODE_DOWN, dx: 0, dy: 1, name: "down"},
		{sc: sdl.SCANCODE_RIGHT, dx: 1, dy: 0, name: "right"},
		// Turning back onto the neck is forbidden, so the snake keeps going right.
		{sc: sdl.SCANCODE_LEFT, dx: 1, dy: 0, name: "left is rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, clock, in := setup(t, sdl.INIT_VIDEO)
			in.Press(tc.sc)
			feed(t, g)
			clock.Advance(stepRateInMilliseconds)
			g.advance()
			if g.ctx.headX != startX+tc.dx || g.ctx.headY != startY+tc.dy {
				t.Errorf("head = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, startX+tc.dx, startY+tc.dy)
			}
		})
	}
}

func TestRestartKeyResetsTheSnake(t *testing.T) {
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	clock.Advance(3 * stepRateInMilliseconds)
	g.advance()
	if g.ctx.headX == startX {
		t.Fatal("snake did not move")
	}
	in.Press(sdl.SCANCODE_R)
	feed(t, g)
	if g.ctx.headX != startX || g.ctx.headY != startY {
		t.Errorf("head after restart = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, startX, startY)
	}
}

func TestAdvanceStepsOncePerInterval(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	clock.Advance(stepRateInMilliseconds - 1)
	g.advance()
	if g.ctx.headX != startX {
		t.Fatalf("moved after %d ms, before the step interval", stepRateInMilliseconds-1)
	}
	clock.Advance(1)
	g.advance()
	if g.ctx.headX != startX+1 {
		t.Fatalf("headX = %d after one interval, want %d", g.ctx.headX, startX+1)
	}
	// Falling behind by several intervals catches up in one call.
	clock.Advance(3 * stepRateInMilliseconds)
	g.advance()
	if g.ctx.headX != startX+4 {
		t.Fatalf("headX = %d after catching up, want %d", g.ctx.headX, startX+4)
	}
}

func TestVirtualJoystickHatSteers(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	pad := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g) // EVENT_JOYSTICK_ADDED opens the device
	if g.joystick == nil {
		t.Fatal("game did not open the added joystick")
	}
	pad.SetHat(0, sdl.HAT_DOWN)
	feed(t, g)
	clock.Advance(stepRateInMilliseconds)
	g.advance()
	if g.ctx.headY != startY+1 {
		t.Errorf("headY = %d, want %d after hat down", g.ctx.headY, startY+1)
	}
}

func TestDrawPaintsHeadAndBody(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	clock.Advance(2 * stepRateInMilliseconds)
	g.advance()
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	img := c.Capture(t)

	cell := func(x, y int) image.Rectangle {
		return image.Rect(x*snakeBlockSizeInPixels, y*snakeBlockSizeInPixels, (x+1)*snakeBlockSizeInPixels, (y+1)*snakeBlockSizeInPixels)
	}
	sdltest.AssertRegion(t, img, cell(startX+2, startY), color.RGBA{255, 255, 0, 255}) // head
	sdltest.AssertRegion(t, img, cell(startX+1, startY), color.RGBA{0, 128, 0, 255})   // body
	sdltest.AssertRegion(t, img, cell(startX-3, startY), color.RGBA{0, 0, 0, 255})     // empty
}

// --- helpers for the board-level tests -------------------------------------

// cellsOf lists every board cell whose content satisfies pred.
func cellsOf(c *snakeContext, pred func(snakeCell) bool) (out [][2]int8) {
	for x := range int8(snakeGameWidth) {
		for y := range int8(snakeGameHeight) {
			if pred(c.cellAt(x, y)) {
				out = append(out, [2]int8{x, y})
			}
		}
	}
	return out
}

func isFood(c snakeCell) bool { return c == snakeCellFood }
func isBody(c snakeCell) bool { return c != snakeCellNothing && c != snakeCellFood }

// clearFood removes every food cell so a step cannot eat or collide by chance.
func clearFood(c *snakeContext) {
	for _, p := range cellsOf(c, isFood) {
		c.putCellAt(p[0], p[1], snakeCellNothing)
	}
}

// tick advances the clock by exactly one step interval and runs the game logic.
func tick(g *game, clock *sdltest.Clock) {
	clock.Advance(stepRateInMilliseconds)
	g.advance()
}

// press injects a key press, delivers it to the game, then runs one step.
func press(t *testing.T, g *game, clock *sdltest.Clock, in *sdltest.Input, sc sdl.Scancode) {
	t.Helper()
	in.Press(sc)
	feed(t, g)
	tick(g, clock)
}

func TestInitialBoard(t *testing.T) {
	g, _, _ := setup(t, sdl.INIT_VIDEO)
	c := &g.ctx
	if c.headX != startX || c.headY != startY || c.tailX != startX || c.tailY != startY {
		t.Fatalf("head/tail = (%d,%d)/(%d,%d), want both at (%d,%d)", c.headX, c.headY, c.tailX, c.tailY, startX, startY)
	}
	if got := c.cellAt(startX, startY); got != snakeCellSRight {
		t.Errorf("start cell = %d, want right-facing body %d", got, snakeCellSRight)
	}
	food := cellsOf(c, isFood)
	if len(food) != 4 {
		t.Fatalf("%d food cells, want 4", len(food))
	}
	if body := cellsOf(c, isBody); len(body) != 1 {
		t.Errorf("%d body cells, want 1", len(body))
	}
	if c.occupiedCells != 7 {
		t.Errorf("occupiedCells = %d, want 7 (C: 3 + 4 food)", c.occupiedCells)
	}
	if c.inhibitTailStep != 4 || c.nextDir != snakeDirRight {
		t.Errorf("inhibitTailStep=%d nextDir=%d, want 4 and right", c.inhibitTailStep, c.nextDir)
	}
}

func TestCellPackingRoundTrips(t *testing.T) {
	// Three bits per cell straddle byte boundaries; writing one cell must not
	// disturb its neighbours, including the very last cell of the board.
	var c snakeContext
	type pos struct{ x, y int8 }
	cells := []pos{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {23, 0}, {0, 1}, {23, 17}, {22, 17}}
	for i, p := range cells {
		c.putCellAt(p.x, p.y, snakeCell(i%5+1))
	}
	for i, p := range cells {
		if got, want := c.cellAt(p.x, p.y), snakeCell(i%5+1); got != want {
			t.Errorf("cell %v = %d, want %d", p, got, want)
		}
	}
	c.putCellAt(2, 0, snakeCellNothing)
	if c.cellAt(1, 0) != snakeCell(2) || c.cellAt(3, 0) != snakeCell(4) || c.cellAt(2, 0) != snakeCellNothing {
		t.Error("clearing a cell disturbed its neighbours")
	}
}

func TestInitialFrameColours(t *testing.T) {
	g, _, _ := setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	img := c.Capture(t)
	cell := func(x, y int8) image.Rectangle {
		return image.Rect(int(x)*snakeBlockSizeInPixels, int(y)*snakeBlockSizeInPixels, (int(x)+1)*snakeBlockSizeInPixels, (int(y)+1)*snakeBlockSizeInPixels)
	}
	sdltest.AssertRegion(t, img, cell(startX, startY), color.RGBA{255, 255, 0, 255})
	foods := cellsOf(&g.ctx, isFood)
	for _, p := range foods {
		sdltest.AssertRegion(t, img, cell(p[0], p[1]), color.RGBA{80, 80, 255, 255})
	}
	// Everything else is the black background.
	black := color.RGBA{0, 0, 0, 255}
	special := map[[2]int8]bool{{startX, startY}: true}
	for _, p := range foods {
		special[p] = true
	}
	for x := range int8(snakeGameWidth) {
		for y := range int8(snakeGameHeight) {
			if !special[[2]int8{x, y}] {
				sdltest.AssertRegion(t, img, cell(x, y), black)
			}
		}
	}
}

func TestDrawDoesNotAdvanceGame(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, windowWidth, windowHeight)
	clock.Advance(10 * stepRateInMilliseconds)
	before := g.ctx
	sdltest.OnMain(t, func() { g.draw(c.Renderer) })
	if g.ctx != before {
		t.Error("draw changed the game state")
	}
}

func TestIgnoredInputs(t *testing.T) {
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	// Key release, an unmapped key and a user event must not steer or quit.
	in.Key(false, sdl.SCANCODE_UP, sdl.SDLK_UP, 0)
	in.Press(sdl.SCANCODE_SPACE)
	in.User(1)
	if feed(t, g) {
		t.Fatal("ignored input quit the game")
	}
	tick(g, clock)
	if g.ctx.headX != startX+1 || g.ctx.headY != startY {
		t.Errorf("head = (%d,%d), want it to keep going right", g.ctx.headX, g.ctx.headY)
	}
}

func TestRedirIsJudgedAgainstHeadCellNotPendingDirection(t *testing.T) {
	// Like the C code, redir compares with the direction already drawn into the
	// head cell (right), not with nextDir, so Left is refused even right after
	// Up was accepted in the same tick.
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	in.Press(sdl.SCANCODE_UP)
	in.Press(sdl.SCANCODE_LEFT) // head cell still faces right, so left is refused
	feed(t, g)
	tick(g, clock)
	if g.ctx.headX != startX || g.ctx.headY != startY-1 {
		t.Errorf("head = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, startX, startY-1)
	}
}

func TestReversalIsRejectedAfterTurning(t *testing.T) {
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	clearFood(&g.ctx)
	press(t, g, clock, in, sdl.SCANCODE_UP)   // head cell now faces up
	press(t, g, clock, in, sdl.SCANCODE_DOWN) // reversal refused
	if g.ctx.headX != startX || g.ctx.headY != startY-2 {
		t.Errorf("head = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, startX, startY-2)
	}
}

func TestWrapAroundAllEdges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sc     sdl.Scancode
		steps  int
		wantX  int8
		wantY  int8
		before [2]int8 // head one step before the wrap
	}{
		{"right", sdl.SCANCODE_RIGHT, snakeGameWidth - startX, 0, startY, [2]int8{snakeGameWidth - 1, startY}},
		{"up", sdl.SCANCODE_UP, startY + 1, startX, snakeGameHeight - 1, [2]int8{startX, 0}},
		{"down", sdl.SCANCODE_DOWN, snakeGameHeight - startY, startX, 0, [2]int8{startX, snakeGameHeight - 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, clock, in := setup(t, sdl.INIT_VIDEO)
			clearFood(&g.ctx)
			in.Press(tc.sc)
			feed(t, g)
			for range tc.steps - 1 {
				tick(g, clock)
			}
			if got := [2]int8{g.ctx.headX, g.ctx.headY}; got != tc.before {
				t.Fatalf("head before wrap = %v, want %v", got, tc.before)
			}
			tick(g, clock)
			if g.ctx.headX != tc.wantX || g.ctx.headY != tc.wantY {
				t.Errorf("head after wrap = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, tc.wantX, tc.wantY)
			}
		})
	}
}

func TestWrapAroundLeft(t *testing.T) {
	// Left is refused at the start, so go up, left, and wrap along the row.
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	clearFood(&g.ctx)
	press(t, g, clock, in, sdl.SCANCODE_UP)
	press(t, g, clock, in, sdl.SCANCODE_LEFT)
	// The grid is snakeGameWidth wide, so reaching column 0 needs fewer ticks.
	for i := 0; g.ctx.headX > 0; i++ {
		if i > snakeGameWidth {
			t.Fatalf("head never reached column 0 after %d ticks (headX = %d)", i, g.ctx.headX)
		}
		tick(g, clock)
	}
	tick(g, clock)
	if g.ctx.headX != snakeGameWidth-1 {
		t.Errorf("headX = %d after wrapping left, want %d", g.ctx.headX, snakeGameWidth-1)
	}
}

func TestTailFollowsAfterInitialInhibit(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	clearFood(&g.ctx)
	for range 3 {
		tick(g, clock)
	}
	if g.ctx.tailX != startX {
		t.Fatalf("tail moved after 3 steps (tailX=%d)", g.ctx.tailX)
	}
	tick(g, clock)
	if g.ctx.tailX != startX+1 {
		t.Errorf("tailX = %d after 4 steps, want %d", g.ctx.tailX, startX+1)
	}
	// The snake keeps a constant length of 4 cells when not eating.
	if n := len(cellsOf(&g.ctx, isBody)); n != 4 {
		t.Errorf("%d body cells, want 4", n)
	}
}

// eatAhead puts a single food directly ahead of the head (to the right) and steps.
func eatAhead(g *game, clock *sdltest.Clock) {
	clearFood(&g.ctx)
	g.ctx.putCellAt(g.ctx.headX+1, g.ctx.headY, snakeCellFood)
	tick(g, clock)
}

func TestEatingGrowsAndRespawnsFood(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	c := &g.ctx
	occupied := c.occupiedCells
	eatAhead(g, clock)
	if c.occupiedCells != occupied+1 {
		t.Errorf("occupiedCells = %d, want %d", c.occupiedCells, occupied+1)
	}
	if n := len(cellsOf(c, isFood)); n != 1 {
		t.Errorf("%d food cells after eating, want exactly the respawned one", n)
	}
	if c.cellAt(c.headX, c.headY) == snakeCellFood {
		t.Error("food still under the head")
	}
	// The tail stays put while growing: both start and next cell are body.
	if c.tailX != startX || len(cellsOf(c, isBody)) != 2 {
		t.Errorf("tailX=%d body=%d, want tail fixed and 2 body cells", c.tailX, len(cellsOf(c, isBody)))
	}
}

func TestSelfCollisionRestarts(t *testing.T) {
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	c := &g.ctx
	for range 5 { // grow to six cells in a row: x = 12..17
		eatAhead(g, clock)
	}
	clearFood(c)
	if c.headX != startX+5 {
		t.Fatalf("headX = %d, want %d", c.headX, startX+5)
	}
	press(t, g, clock, in, sdl.SCANCODE_UP)
	press(t, g, clock, in, sdl.SCANCODE_LEFT)
	if c.headX != startX+4 || c.headY != startY-1 {
		t.Fatalf("head = (%d,%d), want (%d,%d)", c.headX, c.headY, startX+4, startY-1)
	}
	press(t, g, clock, in, sdl.SCANCODE_DOWN) // into the body
	if c.headX != startX || c.headY != startY || c.occupiedCells != 7 {
		t.Errorf("after collision head=(%d,%d) occupied=%d, want a fresh game", c.headX, c.headY, c.occupiedCells)
	}
	if n := len(cellsOf(c, isBody)); n != 1 {
		t.Errorf("%d body cells after restart, want 1", n)
	}
	if n := len(cellsOf(c, isFood)); n != 4 {
		t.Errorf("%d food cells after restart, want 4", n)
	}
}

func TestEatingOnFullBoardRestarts(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO)
	g.ctx.occupiedCells = snakeGameWidth * snakeGameHeight
	eatAhead(g, clock)
	if g.ctx.headX != startX || g.ctx.occupiedCells != 7 {
		t.Errorf("head=%d occupied=%d, want a restarted game", g.ctx.headX, g.ctx.occupiedCells)
	}
}

func TestRestartKeyDuringPlayRestoresTail(t *testing.T) {
	g, clock, in := setup(t, sdl.INIT_VIDEO)
	for range 6 {
		tick(g, clock)
	}
	in.Press(sdl.SCANCODE_R)
	feed(t, g)
	c := &g.ctx
	if c.tailX != startX || c.inhibitTailStep != 4 || len(cellsOf(c, isFood)) != 4 || len(cellsOf(c, isBody)) != 1 {
		t.Errorf("restart did not reset: tailX=%d inhibit=%d", c.tailX, c.inhibitTailStep)
	}
}

func TestHatDirectionsAndIgnoredValues(t *testing.T) {
	g, clock, _ := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	pad := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g)
	clearFood(&g.ctx)
	hat := func(v uint8) {
		pad.SetHat(0, v)
		feed(t, g)
	}
	hat(sdl.HAT_UP)
	tick(g, clock)
	if g.ctx.headY != startY-1 {
		t.Fatalf("headY = %d after hat up, want %d", g.ctx.headY, startY-1)
	}
	// Centred and diagonal positions are ignored: still heading up.
	hat(sdl.HAT_CENTERED)
	hat(sdl.HAT_LEFTDOWN)
	tick(g, clock)
	if g.ctx.headX != startX || g.ctx.headY != startY-2 {
		t.Errorf("head = (%d,%d), want (%d,%d)", g.ctx.headX, g.ctx.headY, startX, startY-2)
	}
	hat(sdl.HAT_LEFT)
	tick(g, clock)
	if g.ctx.headX != startX-1 {
		t.Errorf("headX = %d after hat left, want %d", g.ctx.headX, startX-1)
	}
	hat(sdl.HAT_DOWN)
	tick(g, clock)
	hat(sdl.HAT_RIGHT)
	tick(g, clock)
	if g.ctx.headX != startX || g.ctx.headY != startY-1 {
		t.Errorf("head = (%d,%d) after hat down then right, want (%d,%d)", g.ctx.headX, g.ctx.headY, startX, startY-1)
	}
}

func TestJoystickHotPlug(t *testing.T) {
	g, _, _ := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	if g.joystick != nil {
		t.Fatal("joystick open with no device attached")
	}
	first := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	second := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g)
	if g.joystick == nil {
		t.Fatal("no joystick opened for an added device")
	}
	opened := sdltest.Call(t, func() sdl.JoystickID { return sdl.GetJoystickID(g.joystick) })
	if opened != first.ID {
		t.Errorf("opened joystick %d, want the first device %d (a second one is ignored)", opened, first.ID)
	}

	// Removing a device the game did not open keeps the open one.
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(second.ID) })
	feed(t, g)
	if g.joystick == nil {
		t.Fatal("removing an unrelated device closed the game's joystick")
	}
	// Removing the open one releases it.
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(first.ID) })
	feed(t, g)
	if g.joystick != nil {
		t.Error("joystick still held after its device was removed")
	}
}

func TestJoystickReopenedAfterRemoval(t *testing.T) {
	g, _, _ := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	first := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g)
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(first.ID) })
	feed(t, g)
	second := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g)
	if g.joystick == nil || sdltest.Call(t, func() sdl.JoystickID { return sdl.GetJoystickID(g.joystick) }) != second.ID {
		t.Error("game did not pick up the replacement device")
	}
}

func TestCloseReleasesJoystickAndIsIdempotent(t *testing.T) {
	g, _, _ := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	feed(t, g)
	if g.joystick == nil {
		t.Fatal("joystick not opened")
	}
	sdltest.OnMain(t, g.close)
	sdltest.OnMain(t, g.close)
	if g.joystick != nil {
		t.Error("close left the joystick handle set")
	}
}

func TestQuitEventAfterOtherEventsIsReported(t *testing.T) {
	g, _, in := setup(t, sdl.INIT_VIDEO)
	in.Press(sdl.SCANCODE_UP)
	in.Quit()
	if !feed(t, g) {
		t.Error("quit not reported")
	}
}

// TestEventAccessorsMatchABI pushes events with distinctive field values and
// reads them back through the accessors the example relies on.
func TestEventAccessorsMatchABI(t *testing.T) {
	_, _, in := setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	in.Key(true, sdl.SCANCODE_ESCAPE, sdl.SDLK_ESCAPE, sdl.KMOD_LSHIFT)
	in.JoyHat(0x1234, 3, sdl.HAT_RIGHTUP)
	in.Quit()
	events := sdltest.Drain(t)

	keys := sdltest.EventsOfType(events, sdl.EVENT_KEY_DOWN)
	if len(keys) != 1 {
		t.Fatalf("%d key-down events, want 1", len(keys))
	}
	if k := keys[0].Key(); k.Key != sdl.SDLK_ESCAPE || k.Scancode != sdl.SCANCODE_ESCAPE || !k.Down || k.Mod != sdl.KMOD_LSHIFT {
		t.Errorf("key event = %+v", *k)
	}
	hats := sdltest.EventsOfType(events, sdl.EVENT_JOYSTICK_HAT_MOTION)
	if len(hats) != 1 {
		t.Fatalf("%d hat events, want 1", len(hats))
	}
	if h := hats[0].Jhat(); h.Which != 0x1234 || h.Hat != 3 || h.Value != sdl.HAT_RIGHTUP {
		t.Errorf("hat event = %+v", *h)
	}
	if len(sdltest.EventsOfType(events, sdl.EVENT_QUIT)) != 1 {
		t.Error("quit event lost")
	}
}

func TestJoystickAddedEventCarriesDeviceID(t *testing.T) {
	setup(t, sdl.INIT_VIDEO|sdl.INIT_JOYSTICK)
	pad := sdltest.AttachJoystick(t, sdltest.JoystickSpec{Hats: 1})
	added := sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_JOYSTICK_ADDED)
	if len(added) != 1 || added[0].Jdevice().Which != pad.ID {
		t.Fatalf("added events = %d, want one carrying ID %d", len(added), pad.ID)
	}
	sdltest.OnMain(t, func() { sdl.DetachVirtualJoystick(pad.ID) })
	removed := sdltest.EventsOfType(sdltest.Drain(t), sdl.EVENT_JOYSTICK_REMOVED)
	if len(removed) != 1 || removed[0].Jdevice().Which != pad.ID {
		t.Fatalf("removed events = %d, want one carrying ID %d", len(removed), pad.ID)
	}
}

func TestRunSingleFrame(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(1) })
	if err != nil {
		t.Fatalf("run(1): %v", err)
	}
}

func TestRunReturnsAfterRequestedFrames(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3) })
	if err != nil {
		t.Fatalf("run(3): %v", err)
	}
}
