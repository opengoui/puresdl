// Command 01-snake is a complete game of Snake. The game state is packed
// into a compact bit matrix, three bits per cell.
// It is a port of SDL/examples/demo/01-snake/snake.c.
//
// Controls: arrow keys or a joystick hat steer, R restarts, Esc or Q quits.
//
// Run it with:
//
//	go run ./examples/demo/01-snake [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to its OS thread, since SDL video and event
// calls must run on the thread that initialized them.
func init() {
	runtime.LockOSThread()
}

// Game geometry and timing. Sizes are in pixels unless named as cells.
const (
	stepRateInMilliseconds = 125
	snakeBlockSizeInPixels = 24

	snakeGameWidth  = 24
	snakeGameHeight = 18
	snakeMatrixSize = snakeGameWidth * snakeGameHeight

	windowWidth  = snakeBlockSizeInPixels * snakeGameWidth
	windowHeight = snakeBlockSizeInPixels * snakeGameHeight

	// snakeCellMaxBits is floor(log2(snakeCellFood)) + 1.
	snakeCellMaxBits = 3
	snakeCellSetBits = (1 << snakeCellMaxBits) - 1
)

// snakeCell is the content of one grid cell, stored in snakeCellMaxBits bits.
type snakeCell uint

// Cell contents. The snake body cells record the direction the snake moved
// when leaving that cell, which lets the tail follow the body.
const (
	snakeCellNothing snakeCell = iota
	snakeCellSRight
	snakeCellSUp
	snakeCellSLeft
	snakeCellSDown
	snakeCellFood
)

// snakeDirection is the heading of the snake head.
type snakeDirection int8

// Snake headings. Their order matches snakeCellSRight..snakeCellSDown minus one.
const (
	snakeDirRight snakeDirection = iota
	snakeDirUp
	snakeDirLeft
	snakeDirDown
)

// snakeContext is the whole game state. The int8 fields mirror the C chars so
// the wrap-around arithmetic behaves identically.
type snakeContext struct {
	// One extra byte so 16-bit accesses at the last cell stay in range.
	cells           [(snakeMatrixSize*snakeCellMaxBits)/8 + 1]byte
	headX, headY    int8
	tailX, tailY    int8
	nextDir         snakeDirection
	inhibitTailStep int8
	occupiedCells   uint
}

// shift returns the bit offset of cell (x, y) in the packed matrix.
func shift(x, y int) int {
	return (x + y*snakeGameWidth) * snakeCellMaxBits
}

// cellAt returns the content of cell (x, y).
func (c *snakeContext) cellAt(x, y int8) snakeCell {
	s := shift(int(x), int(y))
	i := s / 8
	rng := uint16(c.cells[i]) | uint16(c.cells[i+1])<<8
	return snakeCell((rng >> (s % 8)) & snakeCellSetBits)
}

// putCellAt stores ct in cell (x, y), leaving neighboring cells' bits intact.
func (c *snakeContext) putCellAt(x, y int8, ct snakeCell) {
	s := shift(int(x), int(y))
	adjust := s % 8
	i := s / 8
	rng := uint16(c.cells[i]) | uint16(c.cells[i+1])<<8
	rng &^= snakeCellSetBits << adjust
	rng |= (uint16(ct) & snakeCellSetBits) << adjust
	c.cells[i] = byte(rng)
	c.cells[i+1] = byte(rng >> 8)
}

// cellsFull reports whether every grid cell is occupied.
func (c *snakeContext) cellsFull() bool {
	return c.occupiedCells == snakeGameWidth*snakeGameHeight
}

// newFoodPos places food on a random empty cell. It loops until one is found,
// so the grid must not be full.
func (c *snakeContext) newFoodPos() {
	for {
		x := int8(sdl.Rand(snakeGameWidth))
		y := int8(sdl.Rand(snakeGameHeight))
		if c.cellAt(x, y) == snakeCellNothing {
			c.putCellAt(x, y, snakeCellFood)
			return
		}
	}
}

// initialize resets the board to a short snake in the center with four foods.
func (c *snakeContext) initialize() {
	c.cells = [len(c.cells)]byte{}
	c.headX, c.tailX = snakeGameWidth/2, snakeGameWidth/2
	c.headY, c.tailY = snakeGameHeight/2, snakeGameHeight/2
	c.nextDir = snakeDirRight
	c.inhibitTailStep = 4
	c.occupiedCells = 3
	c.putCellAt(c.tailX, c.tailY, snakeCellSRight)
	for range 4 {
		c.newFoodPos()
		c.occupiedCells++
	}
}

// redir sets the next heading to dir unless that would reverse the snake onto
// its own neck.
func (c *snakeContext) redir(dir snakeDirection) {
	ct := c.cellAt(c.headX, c.headY)
	if (dir == snakeDirRight && ct != snakeCellSLeft) ||
		(dir == snakeDirUp && ct != snakeCellSDown) ||
		(dir == snakeDirLeft && ct != snakeCellSRight) ||
		(dir == snakeDirDown && ct != snakeCellSUp) {
		c.nextDir = dir
	}
}

// wrapAround wraps *val into [0, limit) after a single-step overflow.
func wrapAround(val *int8, limit int8) {
	if *val < 0 {
		*val = limit - 1
	} else if *val > limit-1 {
		*val = 0
	}
}

// step advances the game by one tick: it moves the tail (unless growing), moves
// the head, and handles collisions and eating. A collision or a full board
// restarts the game.
func (c *snakeContext) step() {
	dirAsCell := snakeCell(c.nextDir) + 1
	// Move tail forward.
	c.inhibitTailStep--
	if c.inhibitTailStep == 0 {
		c.inhibitTailStep++
		ct := c.cellAt(c.tailX, c.tailY)
		c.putCellAt(c.tailX, c.tailY, snakeCellNothing)
		switch ct {
		case snakeCellSRight:
			c.tailX++
		case snakeCellSUp:
			c.tailY--
		case snakeCellSLeft:
			c.tailX--
		case snakeCellSDown:
			c.tailY++
		}
		wrapAround(&c.tailX, snakeGameWidth)
		wrapAround(&c.tailY, snakeGameHeight)
	}
	// Move head forward.
	prevX, prevY := c.headX, c.headY
	switch c.nextDir {
	case snakeDirRight:
		c.headX++
	case snakeDirUp:
		c.headY--
	case snakeDirLeft:
		c.headX--
	case snakeDirDown:
		c.headY++
	}
	wrapAround(&c.headX, snakeGameWidth)
	wrapAround(&c.headY, snakeGameHeight)
	// Collisions.
	ct := c.cellAt(c.headX, c.headY)
	if ct != snakeCellNothing && ct != snakeCellFood {
		c.initialize()
		return
	}
	c.putCellAt(prevX, prevY, dirAsCell)
	c.putCellAt(c.headX, c.headY, dirAsCell)
	if ct == snakeCellFood {
		if c.cellsFull() {
			c.initialize()
			return
		}
		c.newFoodPos()
		c.inhibitTailStep++
		c.occupiedCells++
	}
}

// handleKey applies a key press and reports whether the game should quit.
func (c *snakeContext) handleKey(key sdl.Keycode) (quit bool) {
	switch key {
	case sdl.SDLK_ESCAPE, sdl.SDLK_Q:
		return true
	case sdl.SDLK_R:
		c.initialize()
	case sdl.SDLK_RIGHT:
		c.redir(snakeDirRight)
	case sdl.SDLK_UP:
		c.redir(snakeDirUp)
	case sdl.SDLK_LEFT:
		c.redir(snakeDirLeft)
	case sdl.SDLK_DOWN:
		c.redir(snakeDirDown)
	}
	return false
}

// handleHat steers the snake from a joystick hat position.
func (c *snakeContext) handleHat(hat uint8) {
	switch hat {
	case sdl.HAT_RIGHT:
		c.redir(snakeDirRight)
	case sdl.HAT_UP:
		c.redir(snakeDirUp)
	case sdl.HAT_LEFT:
		c.redir(snakeDirLeft)
	case sdl.HAT_DOWN:
		c.redir(snakeDirDown)
	}
}

// main parses flags and runs the game, exiting with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "01-snake:", err)
		os.Exit(1)
	}
}

// game bundles the snake state with everything handleEvent and advance need,
// so tests can drive it without a window or a loop.
type game struct {
	ctx      snakeContext
	joystick *sdl.Joystick
	lastStep uint64
	// now is the time source in milliseconds; tests replace sdl.GetTicks.
	now func() uint64
}

// newGame returns a freshly initialized game that reads time from now.
func newGame(now func() uint64) *game {
	g := &game{now: now}
	g.ctx.initialize()
	g.lastStep = now()
	return g
}

// handleEvent applies one event and reports whether the game should quit.
func (g *game) handleEvent(e *sdl.Event) (quit bool) {
	switch sdl.EventType(e.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_JOYSTICK_ADDED:
		if g.joystick == nil {
			which := e.Jdevice().Which
			g.joystick = sdl.OpenJoystick(which)
			if g.joystick == nil {
				sdl.Log("Failed to open joystick ID %d: %s", uint32(which), sdl.GetError())
			}
		}
	case sdl.EVENT_JOYSTICK_REMOVED:
		if g.joystick != nil && sdl.GetJoystickID(g.joystick) == e.Jdevice().Which {
			sdl.CloseJoystick(g.joystick)
			g.joystick = nil
		}
	case sdl.EVENT_JOYSTICK_HAT_MOTION:
		g.ctx.handleHat(e.Jhat().Value)
	case sdl.EVENT_KEY_DOWN:
		return g.ctx.handleKey(e.Key().Key)
	}
	return false
}

// advance runs the game logic if we're at or past the time to run it; if
// we're really behind, it runs several times.
func (g *game) advance() {
	now := g.now()
	for now-g.lastStep >= stepRateInMilliseconds {
		g.ctx.step()
		g.lastStep += stepRateInMilliseconds
	}
}

// close releases the joystick, if one is open. It is safe to call repeatedly.
func (g *game) close() {
	if g.joystick != nil {
		sdl.CloseJoystick(g.joystick)
		g.joystick = nil
	}
}

// draw renders the current state; it does not present.
func (g *game) draw(renderer *sdl.Renderer) {
	ctx := &g.ctx
	r := sdl.FRect{W: snakeBlockSizeInPixels, H: snakeBlockSizeInPixels}
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)
	for i := range int8(snakeGameWidth) {
		for j := range int8(snakeGameHeight) {
			ct := ctx.cellAt(i, j)
			if ct == snakeCellNothing {
				continue
			}
			r.X = float32(int(i) * snakeBlockSizeInPixels)
			r.Y = float32(int(j) * snakeBlockSizeInPixels)
			if ct == snakeCellFood {
				sdl.SetRenderDrawColor(renderer, 80, 80, 255, sdl.ALPHA_OPAQUE)
			} else { // body
				sdl.SetRenderDrawColor(renderer, 0, 128, 0, sdl.ALPHA_OPAQUE)
			}
			sdl.RenderFillRect(renderer, &r)
		}
	}
	sdl.SetRenderDrawColor(renderer, 255, 255, 0, sdl.ALPHA_OPAQUE) // head
	r.X = float32(int(ctx.headX) * snakeBlockSizeInPixels)
	r.Y = float32(int(ctx.headY) * snakeBlockSizeInPixels)
	sdl.RenderFillRect(renderer, &r)
}

// run initializes SDL, creates the window and renderer, and runs the game loop
// until a quit event or, when maxFrames is positive, that many frames have
// been drawn. It must be called on the main thread.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	if !sdl.SetAppMetadata("Example Snake game", "1.0", "com.example.Snake") {
		return fmt.Errorf("SDL_SetAppMetadata: %s", sdl.GetError())
	}
	for _, kv := range [][2]string{
		{sdl.PROP_APP_METADATA_URL_STRING, "https://examples.libsdl.org/SDL3/demo/01-snake/"},
		{sdl.PROP_APP_METADATA_CREATOR_STRING, "SDL team"},
		{sdl.PROP_APP_METADATA_COPYRIGHT_STRING, "Placed in the public domain"},
		{sdl.PROP_APP_METADATA_TYPE_STRING, "game"},
	} {
		if !sdl.SetAppMetadataPropertyValue(kv[0], kv[1]) {
			return fmt.Errorf("SDL_SetAppMetadataProperty: %s", sdl.GetError())
		}
	}
	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/demo/snake", windowWidth, windowHeight, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)
	sdl.SetRenderLogicalPresentation(renderer, windowWidth, windowHeight, sdl.LOGICAL_PRESENTATION_LETTERBOX)

	g := newGame(sdl.GetTicks)
	defer g.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if g.handleEvent(&event) {
				return nil
			}
		}
		g.advance()
		g.draw(renderer)
		sdl.RenderPresent(renderer)
	}
	return nil
}
