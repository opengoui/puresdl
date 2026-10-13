package main

import (
	"image"
	"image/color"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

var (
	black   = color.RGBA{0, 0, 0, 255}
	white   = color.RGBA{255, 255, 255, 255}
	gray    = color.RGBA{64, 64, 64, 255}
	green   = color.RGBA{0, 255, 0, 255}
	magenta = color.RGBA{255, 0, 255, 255}
	red     = color.RGBA{255, 0, 0, 255}
	cyan    = color.RGBA{0, 255, 255, 255}
)

// fakeClock is a deterministic nanosecond time source. Every read advances it
// by step; delay advances it by the requested amount and records the request.
type fakeClock struct {
	ns     uint64
	step   uint64
	delays []uint64
}

func (c *fakeClock) now() uint64 {
	v := c.ns
	c.ns += c.step
	return v
}

func (c *fakeClock) delay(ns uint64) {
	c.delays = append(c.delays, ns)
	c.ns += ns
}

func newState(c *fakeClock) *appState { return newAppState(c.now, c.delay) }

// push queues a copy of e. SDL must be initialized.
func push(t *testing.T, e *sdl.Event) {
	t.Helper()
	if !sdl.PushEvent(e) {
		t.Fatalf("PushEvent(%#x): %s", uint32(e.Type()), sdl.GetError())
	}
}

func mouseMotion(t *testing.T, which sdl.MouseID, xrel, yrel float32) {
	t.Helper()
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_MOUSE_MOTION)
	m := e.Motion()
	m.Which, m.Xrel, m.Yrel = which, xrel, yrel
	push(t, e)
}

func mouseButton(t *testing.T, which sdl.MouseID, down bool) {
	t.Helper()
	typ := sdl.EVENT_MOUSE_BUTTON_UP
	if down {
		typ = sdl.EVENT_MOUSE_BUTTON_DOWN
	}
	e := new(sdl.Event)
	e.SetType(typ)
	b := e.Button()
	b.Which, b.Button, b.Down = which, sdl.BUTTON_LEFT, down
	push(t, e)
}

func mouseRemoved(t *testing.T, which sdl.MouseID) {
	t.Helper()
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_MOUSE_REMOVED)
	e.Mdevice().Which = which
	push(t, e)
}

func keyboardRemoved(t *testing.T, which sdl.KeyboardID) {
	t.Helper()
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_KEYBOARD_REMOVED)
	e.Kdevice().Which = which
	push(t, e)
}

func key(t *testing.T, which sdl.KeyboardID, sc sdl.Scancode, down bool) {
	t.Helper()
	typ := sdl.EVENT_KEY_UP
	if down {
		typ = sdl.EVENT_KEY_DOWN
	}
	e := new(sdl.Event)
	e.SetType(typ)
	k := e.Key()
	k.Which, k.Scancode, k.Down = which, sc, down
	push(t, e)
}

// feed pumps SDL and applies every queued event like the run loop does.
func feed(t *testing.T, as *appState) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if as.event(&e) {
				quit = true
			}
		}
	})
	return quit
}

func setup(t *testing.T) (*appState, *fakeClock) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := &fakeClock{}
	return newState(c), c
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestInitialPlayersAndEdges(t *testing.T) {
	sdltest.Skip(t)
	as := newState(&fakeClock{})
	if as.playerCount != 1 {
		t.Errorf("playerCount = %d, want 1", as.playerCount)
	}
	want := []struct {
		pos   [3]float64
		yaw   uint32
		color [3]uint8
	}{
		{[3]float64{8, 0, 8}, 0x20000000, [3]uint8{0, 255, 0}},
		{[3]float64{-8, 0, -8}, 0xA0000000, [3]uint8{255, 0, 255}},
		{[3]float64{8, 0, -8}, 0x60000000, [3]uint8{255, 0, 0}},
		{[3]float64{-8, 0, 8}, 0xE0000000, [3]uint8{0, 255, 255}},
	}
	for i, w := range want {
		p := as.players[i]
		if p.pos != w.pos || p.yaw != w.yaw || p.color != w.color {
			t.Errorf("player %d = pos %v yaw %#x color %v, want %v %#x %v", i, p.pos, p.yaw, p.color, w.pos, w.yaw, w.color)
		}
		if p.vel != [3]float64{} || p.pitch != -0x08000000 || p.radius != 0.5 || p.height != 1.5 ||
			p.wasd != 0 || p.mouse != 0 || p.keyboard != 0 {
			t.Errorf("player %d has unexpected initial state %+v", i, p)
		}
	}
	// 12 cube edges followed by 2 x 16 floor grid lines.
	if got := len(as.edges); got != 44 {
		t.Fatalf("len(edges) = %d, want 44", got)
	}
	if as.edges[0] != [6]float32{-16, -16, -16, 16, -16, -16} {
		t.Errorf("edges[0] = %v", as.edges[0])
	}
	if as.edges[1] != [6]float32{16, -16, -16, 16, 16, -16} {
		t.Errorf("edges[1] = %v", as.edges[1])
	}
	if as.edges[12] != [6]float32{-16, -16, -16, 16, -16, -16} {
		t.Errorf("edges[12] = %v", as.edges[12])
	}
	if as.edges[27] != [6]float32{-16, -16, 14, 16, -16, 14} {
		t.Errorf("edges[27] = %v", as.edges[27])
	}
	if as.edges[28] != [6]float32{-16, -16, -16, -16, -16, 16} {
		t.Errorf("edges[28] = %v", as.edges[28])
	}
	if as.edges[43] != [6]float32{14, -16, -16, 14, -16, 16} {
		t.Errorf("edges[43] = %v", as.edges[43])
	}
	for i, e := range as.edges {
		for _, v := range e {
			if v < -16 || v > 16 {
				t.Errorf("edge %d has coordinate %v outside the box", i, v)
			}
		}
		if i >= 12 && (e[1] != -16 || e[4] != -16) {
			t.Errorf("grid edge %d is not on the floor: %v", i, e)
		}
	}
}

func TestKeyboardClaimsPlayerSlots(t *testing.T) {
	as, _ := setup(t)

	// The first key press of an unknown keyboard only claims a slot.
	key(t, 7, sdl.SCANCODE_W, true)
	feed(t, as)
	if as.players[0].keyboard != 7 || as.playerCount != 1 || as.players[0].wasd != 0 {
		t.Fatalf("after claim: keyboard=%d count=%d wasd=%d", as.players[0].keyboard, as.playerCount, as.players[0].wasd)
	}
	// A second keyboard claims slot 1 and grows the split screen.
	key(t, 8, sdl.SCANCODE_A, true)
	feed(t, as)
	if as.players[1].keyboard != 8 || as.playerCount != 2 {
		t.Fatalf("second keyboard: slot1=%d count=%d", as.players[1].keyboard, as.playerCount)
	}
	// Keyboard id 0 (virtual/unknown) never claims a slot.
	key(t, 0, sdl.SCANCODE_W, true)
	feed(t, as)
	if as.players[2].keyboard != 0 || as.playerCount != 2 {
		t.Errorf("id 0 claimed a slot: %d, count %d", as.players[2].keyboard, as.playerCount)
	}
	// Slots 2 and 3 fill up; a fifth keyboard is ignored.
	for _, id := range []sdl.KeyboardID{9, 10, 11} {
		key(t, id, sdl.SCANCODE_W, true)
	}
	feed(t, as)
	if as.playerCount != 4 || as.players[2].keyboard != 9 || as.players[3].keyboard != 10 {
		t.Errorf("count=%d slots=%d,%d", as.playerCount, as.players[2].keyboard, as.players[3].keyboard)
	}
	for i := range as.players {
		if as.players[i].keyboard == 11 {
			t.Error("fifth keyboard was given a slot")
		}
	}
	// A claimed slot is not reused by the same keyboard.
	key(t, 7, sdl.SCANCODE_S, true)
	feed(t, as)
	if as.playerCount != 4 || as.players[0].wasd != 4 {
		t.Errorf("owner press: count=%d wasd=%d", as.playerCount, as.players[0].wasd)
	}
}

func TestMovementKeysSetAndClearBits(t *testing.T) {
	as, _ := setup(t)
	key(t, 5, sdl.SCANCODE_UNKNOWN, true) // claim slot 0
	feed(t, as)
	p := &as.players[0]
	keys := []struct {
		sc  sdl.Scancode
		bit uint8
	}{
		{sdl.SCANCODE_W, 1}, {sdl.SCANCODE_A, 2}, {sdl.SCANCODE_S, 4}, {sdl.SCANCODE_D, 8}, {sdl.SCANCODE_SPACE, 16},
	}
	var all uint8
	for _, k := range keys {
		key(t, 5, k.sc, true)
		feed(t, as)
		all |= k.bit
		if p.wasd != all {
			t.Fatalf("after pressing %d: wasd=%#b, want %#b", k.sc, p.wasd, all)
		}
	}
	// Unrelated keys do not touch the mask, neither down nor up.
	key(t, 5, sdl.SCANCODE_Q, true)
	key(t, 5, sdl.SCANCODE_Q, false)
	feed(t, as)
	if p.wasd != 31 {
		t.Fatalf("unrelated key changed wasd to %#b", p.wasd)
	}
	for _, k := range keys {
		key(t, 5, k.sc, false)
		feed(t, as)
		all &^= k.bit
		if p.wasd != all {
			t.Fatalf("after releasing %d: wasd=%#b, want %#b", k.sc, p.wasd, all)
		}
	}
	// Releasing a key of an unknown keyboard changes nothing.
	key(t, 5, sdl.SCANCODE_W, true)
	feed(t, as)
	key(t, 99, sdl.SCANCODE_W, false)
	feed(t, as)
	if p.wasd != 1 {
		t.Errorf("foreign key-up cleared wasd: %#b", p.wasd)
	}
}

func TestQuitPaths(t *testing.T) {
	as, _ := setup(t)

	// Escape on key release quits even from an unknown keyboard.
	key(t, 3, sdl.SCANCODE_ESCAPE, true)
	if feed(t, as) {
		t.Error("Escape key-down quit; only key-up should")
	}
	key(t, 3, sdl.SCANCODE_ESCAPE, false)
	if !feed(t, as) {
		t.Error("Escape key-up did not quit")
	}

	sdltest.NewInput(t, 0).Quit()
	if !feed(t, as) {
		t.Error("EVENT_QUIT did not quit")
	}

	// Events the C program does not handle must neither quit nor change state.
	before, count := as.players, as.playerCount
	in := sdltest.NewInput(t, 0)
	in.CloseRequested()
	in.MouseWheel(0, 1)
	in.Text("x")
	in.ClipboardUpdate()
	in.Resized(100, 100)
	e := new(sdl.Event)
	e.SetType(sdl.EVENT_MOUSE_ADDED)
	e.Mdevice().Which = 4
	push(t, e)
	e = new(sdl.Event)
	e.SetType(sdl.EVENT_KEYBOARD_ADDED)
	e.Kdevice().Which = 4
	push(t, e)
	mouseButton(t, 4, false)
	if feed(t, as) {
		t.Error("unhandled events requested quit")
	}
	if as.players != before || as.playerCount != count {
		t.Error("unhandled events changed the game state")
	}
}

func TestMouseClaimAndLook(t *testing.T) {
	as, _ := setup(t)

	// Motion from an unknown mouse claims slot 0 without turning.
	yaw0, pitch0 := as.players[0].yaw, as.players[0].pitch
	mouseMotion(t, 21, 5, 5)
	feed(t, as)
	p := &as.players[0]
	if p.mouse != 21 || p.yaw != yaw0 || p.pitch != pitch0 {
		t.Fatalf("claim: mouse=%d yaw=%#x pitch=%#x", p.mouse, p.yaw, p.pitch)
	}
	// Mouse id 0 never claims.
	mouseMotion(t, 0, 3, 3)
	feed(t, as)
	if as.players[1].mouse != 0 || as.playerCount != 1 {
		t.Fatalf("mouse 0 claimed a slot")
	}

	// Moving right (positive xrel) decreases yaw by 0x80000 per unit;
	// moving down (positive yrel) lowers the pitch.
	mouseMotion(t, 21, 2, 3)
	feed(t, as)
	if p.yaw != yaw0-2*0x80000 || p.pitch != pitch0-3*0x80000 {
		t.Errorf("yaw=%#x pitch=%#x", p.yaw, p.pitch)
	}
	// Fractional deltas truncate toward zero.
	yaw, pitch := p.yaw, p.pitch
	mouseMotion(t, 21, 1.9, -1.9)
	feed(t, as)
	if p.yaw != yaw-0x80000 || p.pitch != pitch+0x80000 {
		t.Errorf("fractional: yaw=%#x pitch=%#x", p.yaw, p.pitch)
	}
	mouseMotion(t, 21, -0.9, 0.9)
	feed(t, as)
	if p.yaw != yaw-0x80000 || p.pitch != pitch+0x80000 {
		t.Errorf("sub-unit motion turned the player: yaw=%#x pitch=%#x", p.yaw, p.pitch)
	}
	// Yaw wraps around the full circle.
	p.yaw = 0
	mouseMotion(t, 21, 1, 0)
	feed(t, as)
	if p.yaw != 0xFFF80000 {
		t.Errorf("yaw wrap = %#x", p.yaw)
	}
	// Pitch clamps to straight up / straight down.
	p.pitch = 0x30000000
	mouseMotion(t, 21, 0, -1000)
	feed(t, as)
	if p.pitch != 0x40000000 {
		t.Errorf("pitch up clamp = %#x", p.pitch)
	}
	p.pitch = -0x30000000
	mouseMotion(t, 21, 0, 1000)
	feed(t, as)
	if p.pitch != -0x40000000 {
		t.Errorf("pitch down clamp = %#x", p.pitch)
	}
	// A second mouse takes slot 1 and the split grows; slots stop at four.
	for _, id := range []sdl.MouseID{22, 23, 24, 25} {
		mouseMotion(t, id, 0, 0)
	}
	feed(t, as)
	if as.playerCount != 4 || as.players[1].mouse != 22 || as.players[3].mouse != 24 {
		t.Errorf("count=%d slots=%d,%d,%d", as.playerCount, as.players[1].mouse, as.players[2].mouse, as.players[3].mouse)
	}
}

func TestDeviceRemovalFreesSlots(t *testing.T) {
	as, _ := setup(t)
	mouseMotion(t, 21, 0, 0)
	mouseMotion(t, 22, 0, 0)
	key(t, 31, sdl.SCANCODE_W, true)
	key(t, 32, sdl.SCANCODE_W, true)
	feed(t, as)
	if as.playerCount != 2 || as.players[0].mouse != 21 || as.players[1].mouse != 22 ||
		as.players[0].keyboard != 31 || as.players[1].keyboard != 32 {
		t.Fatalf("setup: %+v", as.players[:2])
	}

	mouseRemoved(t, 21)
	keyboardRemoved(t, 32)
	feed(t, as)
	if as.players[0].mouse != 0 || as.players[1].mouse != 22 || as.players[0].keyboard != 31 || as.players[1].keyboard != 0 {
		t.Errorf("removal: mice %d,%d keyboards %d,%d", as.players[0].mouse, as.players[1].mouse, as.players[0].keyboard, as.players[1].keyboard)
	}
	if as.playerCount != 2 {
		t.Errorf("playerCount shrank to %d", as.playerCount)
	}
	// Removing an unrelated device is a no-op.
	mouseRemoved(t, 999)
	keyboardRemoved(t, 999)
	feed(t, as)
	if as.players[1].mouse != 22 || as.players[0].keyboard != 31 {
		t.Error("unrelated removal changed ownership")
	}
	// A new device re-uses the freed slot, not a new one.
	mouseMotion(t, 40, 0, 0)
	key(t, 41, sdl.SCANCODE_W, true)
	feed(t, as)
	if as.players[0].mouse != 40 || as.players[1].keyboard != 41 || as.playerCount != 2 {
		t.Errorf("reclaim: mouse0=%d keyboard1=%d count=%d", as.players[0].mouse, as.players[1].keyboard, as.playerCount)
	}
}

func TestUpdatePhysics(t *testing.T) {
	const rate, mult = 6.0, 60.0
	const sec = 1e9

	t.Run("gravity single step", func(t *testing.T) {
		sdltest.Skip(t)
		ps := []player{{radius: 0.5, height: 1.5}}
		update(ps, 0.1*sec)
		if !near(ps[0].pos[1], -0.125, 1e-12) || !near(ps[0].vel[1], -2.5, 1e-12) {
			t.Errorf("pos.y=%v vel.y=%v", ps[0].pos[1], ps[0].vel[1])
		}
	})
	t.Run("zero dt is a no-op", func(t *testing.T) {
		sdltest.Skip(t)
		ps := []player{{radius: 0.5, height: 1.5, wasd: 15, pos: [3]float64{1, 2, 3}, vel: [3]float64{1, 1, 1}}}
		want := ps[0]
		update(ps, 0)
		if ps[0] != want {
			t.Errorf("got %+v, want %+v", ps[0], want)
		}
	})
	t.Run("floor stops falling", func(t *testing.T) {
		sdltest.Skip(t)
		ps := []player{{radius: 0.5, height: 1.5}}
		for range 200 {
			update(ps, 0.02*sec)
		}
		if ps[0].pos[1] != -14.5 || ps[0].vel[1] != 0 {
			t.Errorf("pos.y=%v vel.y=%v, want floor -14.5 at rest", ps[0].pos[1], ps[0].vel[1])
		}
	})
	t.Run("space jumps from the floor", func(t *testing.T) {
		sdltest.Skip(t)
		ps := []player{{radius: 0.5, height: 1.5, wasd: 16, pos: [3]float64{0, -14.5, 0}}}
		update(ps, 0.02*sec)
		if ps[0].vel[1] != 8.4375 || ps[0].pos[1] != -14.5 {
			t.Errorf("pos.y=%v vel.y=%v", ps[0].pos[1], ps[0].vel[1])
		}
		// Without space the same contact just stops.
		ps[0].wasd = 0
		ps[0].vel[1] = -3
		update(ps, 0.02*sec)
		if ps[0].vel[1] != 0 {
			t.Errorf("vel.y without space = %v", ps[0].vel[1])
		}
	})
	t.Run("W accelerates along the closed form", func(t *testing.T) {
		sdltest.Skip(t)
		// yaw 0: W is -z. Exact integration of dv/dt = a - rate*v from rest.
		const dt = 0.5
		ps := []player{{radius: 0.5, height: 1.5, wasd: 1}}
		update(ps, dt*sec)
		a := -mult
		wantV := a / rate * (1 - math.Exp(-rate*dt))
		wantP := a / rate * (dt - (1-math.Exp(-rate*dt))/rate)
		if !near(ps[0].vel[2], wantV, 1e-9) || !near(ps[0].pos[2], wantP, 1e-9) || ps[0].pos[0] != 0 || ps[0].vel[0] != 0 {
			t.Errorf("vel=%v pos=%v want vz=%v pz=%v", ps[0].vel, ps[0].pos, wantV, wantP)
		}
	})
	t.Run("terminal speed is 10 in every direction", func(t *testing.T) {
		sdltest.Skip(t)
		for _, tc := range []struct {
			name string
			wasd uint8
			yaw  uint32
			dir  [2]float64 // expected unit direction in x, z
		}{
			{"W yaw0", 1, 0, [2]float64{0, -1}},
			{"S yaw0", 4, 0, [2]float64{0, 1}},
			{"A yaw0", 2, 0, [2]float64{-1, 0}},
			{"D yaw0", 8, 0, [2]float64{1, 0}},
			{"W yaw90", 1, 0x40000000, [2]float64{-1, 0}},
			{"W+D yaw0 normalised", 1 | 8, 0, [2]float64{math.Sqrt2 / 2, -math.Sqrt2 / 2}},
			{"W+S cancel", 1 | 4, 0, [2]float64{0, 0}},
		} {
			ps := []player{{radius: 0.5, height: 1.5, wasd: tc.wasd, yaw: tc.yaw}}
			for range 300 {
				update(ps, 0.01*sec)
				ps[0].pos = [3]float64{} // stay clear of the walls
			}
			if !near(ps[0].vel[0], 10*tc.dir[0], 1e-3) || !near(ps[0].vel[2], 10*tc.dir[1], 1e-3) {
				t.Errorf("%s: vel x,z = %v,%v, want %v", tc.name, ps[0].vel[0], ps[0].vel[2], tc.dir)
			}
		}
	})
	t.Run("walls clamp and zero velocity", func(t *testing.T) {
		sdltest.Skip(t)
		ps := []player{{radius: 0.5, height: 1.5, wasd: 1 | 8}}
		for range 600 {
			update(ps, 0.02*sec)
		}
		if ps[0].pos[0] != 15.5 || ps[0].pos[2] != -15.5 {
			t.Errorf("pos = %v, want x=15.5 z=-15.5", ps[0].pos)
		}
		if ps[0].vel[0] != 0 || ps[0].vel[2] != 0 {
			t.Errorf("vel = %v, want zero horizontal velocity at walls", ps[0].vel)
		}
	})
}

func TestShoot(t *testing.T) {
	sdltest.Skip(t)
	mk := func(ty [3]float64) []player {
		return []player{
			{radius: 0.5, height: 1.5},
			{radius: 0.5, height: 1.5, pos: ty},
		}
	}
	// Yaw 0 / pitch 0 looks down -z.
	for _, tc := range []struct {
		name   string
		target [3]float64
		pitch  int32
		hit    bool
	}{
		{"straight ahead", [3]float64{0, 0, -5}, 0, true},
		{"behind", [3]float64{0, 0, 5}, 0, false},
		{"far to the side", [3]float64{5, 0, -5}, 0, false},
		{"clipping the radius", [3]float64{0.4, 0, -5}, 0, true},
		{"just outside the radius", [3]float64{0.6, 0, -5}, 0, false},
		{"feet sphere", [3]float64{0, 1, -5}, 0, true},
		{"above both spheres", [3]float64{0, 3, -5}, 0, false},
		{"aiming up at a raised target", [3]float64{0, 5, -5}, 0x20000000, true},
		{"aiming up at a level target", [3]float64{0, 0, -5}, 0x20000000, false},
	} {
		sdl.Srand(1)
		ps := mk(tc.target)
		ps[0].pitch = tc.pitch
		as := &appState{playerCount: 2}
		as.players[0], as.players[1] = ps[0], ps[1]
		as.shoot(0)
		moved := as.players[1].pos != tc.target
		if moved != tc.hit {
			t.Errorf("%s: hit = %v, want %v (pos %v)", tc.name, moved, tc.hit, as.players[1].pos)
		}
		if as.players[0].pos != [3]float64{} {
			t.Errorf("%s: shooter moved to %v", tc.name, as.players[0].pos)
		}
	}
}

func TestShootRespawnsFromTheSeededSequence(t *testing.T) {
	sdltest.Skip(t)
	as := &appState{playerCount: 2}
	for i := range as.players {
		as.players[i] = player{radius: 0.5, height: 1.5}
	}
	as.players[1].pos = [3]float64{0, 0, -5}

	sdl.Srand(42)
	r := [3]int32{sdl.Rand(256), sdl.Rand(256), sdl.Rand(256)}
	want := [3]float64{}
	for k := range want {
		want[k] = float64(16*(r[k]-128)) / 256
	}
	sdl.Srand(42)
	as.shoot(0)
	if as.players[1].pos != want {
		t.Errorf("respawn = %v, want %v (x, y, z drawn in that order)", as.players[1].pos, want)
	}
	for k, v := range as.players[1].pos {
		if v < -8 || v >= 8 {
			t.Errorf("respawn[%d] = %v outside [-8, 8)", k, v)
		}
	}
}

func TestMouseButtonShootsOnlyForOwner(t *testing.T) {
	as, _ := setup(t)
	mouseMotion(t, 21, 0, 0)
	mouseMotion(t, 22, 0, 0)
	feed(t, as)
	if as.playerCount != 2 {
		t.Fatalf("playerCount = %d", as.playerCount)
	}
	as.players[0].pos, as.players[0].yaw, as.players[0].pitch = [3]float64{}, 0, 0
	target := [3]float64{0, 0, -5}
	as.players[1].pos = target

	// A button release does not shoot.
	mouseButton(t, 21, false)
	feed(t, as)
	if as.players[1].pos != target {
		t.Error("button release shot")
	}
	// An unknown mouse (and id 0) does not shoot.
	mouseButton(t, 77, true)
	mouseButton(t, 0, true)
	feed(t, as)
	if as.players[1].pos != target {
		t.Error("unowned mouse shot")
	}
	// The owner does.
	mouseButton(t, 21, true)
	feed(t, as)
	if as.players[1].pos == target {
		t.Error("owner click did not hit the target")
	}
	if as.players[0].pos != [3]float64{} {
		t.Error("shooter moved")
	}
}

func TestIterateAppliesPhysicsOnlyToActivePlayers(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := &fakeClock{ns: 100_000_000} // first frame: dt = 100 ms since past = 0
	as := newState(c)
	cv := sdltest.NewCanvas(t, 64, 48)
	as.renderer = cv.Renderer
	idle := as.players[1]
	sdltest.OnMain(t, as.iterate)
	if !near(as.players[0].pos[1], -0.125, 1e-12) {
		t.Errorf("player 0 y = %v, want -0.125", as.players[0].pos[1])
	}
	if as.players[1] != idle {
		t.Errorf("inactive player was simulated: %+v", as.players[1])
	}
	// The next frame uses the delta to the previous frame (c.ns advanced by
	// the compensating delay), not the absolute time.
	if len(c.delays) != 1 || c.delays[0] != 999999 {
		t.Fatalf("delays = %v, want one 999999ns pad", c.delays)
	}
	before := as.players[0].pos[1]
	c.ns += 50_000_000 - 999999
	sdltest.OnMain(t, as.iterate)
	if as.past == 0 || as.accu != 2 {
		t.Errorf("past=%d accu=%d", as.past, as.accu)
	}
	// Frame two reads 150 ms with past = 100 ms: dt = 50 ms, velocity -2.5.
	wantDy := -0.5*25*0.05*0.05 - 2.5*0.05
	if !near(as.players[0].pos[1]-before, wantDy, 1e-12) {
		t.Errorf("frame 2 dy = %v, want %v", as.players[0].pos[1]-before, wantDy)
	}
}

func TestIterateFrameDelayAndFPSCounter(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := &fakeClock{}
	as := newState(c)
	as.renderer = sdltest.NewCanvas(t, 32, 16).Renderer

	// A frame that finishes instantly is padded to 999999 ns; one that took
	// longer is not.
	sdltest.OnMain(t, as.iterate)
	if len(c.delays) != 1 || c.delays[0] != 999999 {
		t.Fatalf("delays = %v", c.delays)
	}
	c.step = 2_000_000 // each clock read costs 2 ms: the frame is "slow"
	sdltest.OnMain(t, as.iterate)
	if len(c.delays) != 1 {
		t.Errorf("slow frame was delayed: %v", c.delays)
	}

	// The fps text appears on the first frame that starts more than one
	// second after the last update, and shows the frames counted before it.
	c.step = 0
	var started, prevStarted uint64
	var counted uint64
	for as.debugString == "" {
		prevStarted, started, counted = started, c.ns, as.accu
		sdltest.OnMain(t, as.iterate)
		if started > 1_000_000_000_000 {
			t.Fatal("fps text never appeared")
		}
	}
	if started <= 999_999_999 || prevStarted > 999_999_999 {
		t.Errorf("fps text appeared at %d ns (previous frame at %d)", started, prevStarted)
	}
	want := strconv.FormatUint(counted, 10) + " fps"
	if as.debugString != want || as.accu != 1 || as.last != started {
		t.Errorf("debugString = %q accu = %d last = %d, want %q, 1, %d", as.debugString, as.accu, as.last, want, started)
	}
}

func drawn(t *testing.T, as *appState, w, h int32) *image.RGBA {
	t.Helper()
	c := sdltest.NewCanvas(t, w, h)
	as.renderer = c.Renderer
	sdltest.OnMain(t, as.draw)
	return c.Capture(t)
}

func countColor(img *image.RGBA, r image.Rectangle, want color.RGBA) (n int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) == want {
				n++
			}
		}
	}
	return n
}

func TestDrawSplitscreenLayouts(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	as := newState(&fakeClock{})

	// One player: full window, crosshair in the middle, wireframe visible.
	img := drawn(t, as, 640, 480)
	for _, p := range [][2]int{{320, 240}, {320, 231}, {320, 249}, {311, 240}, {329, 240}} {
		sdltest.AssertPixel(t, img, p[0], p[1], white)
	}
	if n := countColor(img, img.Bounds(), gray); n == 0 {
		t.Error("no wireframe pixels drawn")
	}

	// Two players split top/bottom.
	as.playerCount = 2
	img = drawn(t, as, 640, 480)
	sdltest.AssertPixel(t, img, 320, 120, white)
	sdltest.AssertPixel(t, img, 320, 360, white)
	if n := countColor(img, image.Rect(0, 0, 640, 240), gray); n == 0 {
		t.Error("top view has no wireframe")
	}
	if n := countColor(img, image.Rect(0, 240, 640, 480), gray); n == 0 {
		t.Error("bottom view has no wireframe")
	}

	// Three players: 2x2 grid, the fourth cell stays black (also proves the
	// per-view clip rectangle contains the other views' lines).
	as.playerCount = 3
	img = drawn(t, as, 640, 480)
	for _, p := range [][2]int{{160, 120}, {480, 120}, {160, 360}} {
		sdltest.AssertPixel(t, img, p[0], p[1], white)
	}
	sdltest.AssertRegion(t, img, image.Rect(320, 240, 640, 480), black)

	// Four players fill the grid.
	as.playerCount = 4
	img = drawn(t, as, 640, 480)
	sdltest.AssertPixel(t, img, 480, 360, white)
	if n := countColor(img, image.Rect(320, 240, 640, 480), gray); n == 0 {
		t.Error("fourth view has no wireframe")
	}

	// A different output size scales the layout.
	as.playerCount = 1
	img = drawn(t, as, 800, 600)
	sdltest.AssertPixel(t, img, 400, 300, white)
	sdltest.AssertPixel(t, img, 400, 291, white)
	as.playerCount = 2
	img = drawn(t, as, 200, 400)
	sdltest.AssertPixel(t, img, 100, 100, white)
	sdltest.AssertPixel(t, img, 100, 300, white)
}

func TestDrawShowsOtherPlayersAsColoredCircles(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	as := newState(&fakeClock{})
	as.playerCount = 2
	top, bottom := image.Rect(0, 0, 640, 240), image.Rect(0, 240, 640, 480)

	// The default spawn points face each other across the box.
	img := drawn(t, as, 640, 480)
	if n := countColor(img, top, magenta); n == 0 {
		t.Error("player 0 does not see player 1's magenta marker")
	}
	if n := countColor(img, bottom, green); n == 0 {
		t.Error("player 1 does not see player 0's green marker")
	}
	if n := countColor(img, top, green) + countColor(img, bottom, magenta); n != 0 {
		t.Errorf("a player sees its own colour: %d pixels", n)
	}

	// Turning player 0 around hides player 1 (dz >= 0 is skipped).
	as.players[0].yaw += 0x80000000
	img = drawn(t, as, 640, 480)
	if n := countColor(img, top, magenta); n != 0 {
		t.Errorf("marker behind the camera was drawn: %d pixels", n)
	}
	if n := countColor(img, bottom, green); n == 0 {
		t.Error("turning player 0 hid it from player 1")
	}

	// Three players: both others are visible in their own colours.
	as.players[0].yaw -= 0x80000000
	as.playerCount = 3
	img = drawn(t, as, 640, 480)
	if countColor(img, image.Rect(0, 0, 320, 240), cyan)+countColor(img, image.Rect(0, 0, 320, 240), red) == 0 &&
		countColor(img, image.Rect(0, 0, 320, 240), magenta) == 0 {
		t.Error("player 0 sees nobody in the three-player layout")
	}
}

func TestDrawDebugText(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	as := newState(&fakeClock{})
	corner := image.Rect(0, 0, 56, 8)

	img := drawn(t, as, 640, 480)
	if n := countColor(img, corner, white); n != 0 {
		t.Errorf("empty debug text drew %d white pixels", n)
	}
	as.debugString = "60 fps"
	img = drawn(t, as, 640, 480)
	if n := countColor(img, corner, white); n == 0 {
		t.Error("debug text not drawn at the top-left corner")
	}
	// Text is drawn after the clip is reset, so it lands in the corner even
	// when that is inside another view's area.
	as.playerCount = 4
	img = drawn(t, as, 640, 480)
	if n := countColor(img, corner, white); n == 0 {
		t.Error("debug text missing in the four-player layout")
	}
}

func TestOpenConfiguresWindowAndRenderer(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	as := newState(&fakeClock{})
	var errMsg string
	sdltest.OnMain(t, func() {
		if err := as.open(); err != nil {
			errMsg = err.Error()
		}
	})
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	t.Cleanup(func() { sdltest.OnMain(t, as.close) })

	type info struct {
		title        string
		w, h         int32
		resizable    bool
		relative     bool
		vsync        int32
		vsyncOK      bool
		hint         string
		outW, outH   int32
		outOK        bool
		rendererName string
	}
	got := sdltest.Call(t, func() (i info) {
		i.title = sdl.GetWindowTitle(as.window)
		sdl.GetWindowSize(as.window, &i.w, &i.h)
		i.resizable = sdl.GetWindowFlags(as.window)&sdl.WINDOW_RESIZABLE != 0
		i.relative = sdl.GetWindowRelativeMouseMode(as.window)
		i.vsyncOK = sdl.GetRenderVSync(as.renderer, &i.vsync)
		i.hint = sdl.GetHint(sdl.HINT_WINDOWS_RAW_KEYBOARD)
		i.outOK = sdl.GetRenderOutputSize(as.renderer, &i.outW, &i.outH)
		return i
	})
	if got.title != "examples/demo/woodeneye-008" || got.w != 640 || got.h != 480 || !got.resizable {
		t.Errorf("window = %q %dx%d resizable=%v", got.title, got.w, got.h, got.resizable)
	}
	if !got.relative {
		t.Error("relative mouse mode is off")
	}
	if got.hint != "1" {
		t.Errorf("HINT_WINDOWS_RAW_KEYBOARD = %q, want 1", got.hint)
	}
	if got.vsyncOK && got.vsync != 0 {
		t.Errorf("vsync = %d, want disabled", got.vsync)
	}
	if !got.outOK || got.outW <= 0 || got.outH <= 0 {
		t.Errorf("render output size %dx%d ok=%v", got.outW, got.outH, got.outOK)
	}

	// Drawing to the real window renderer and presenting keeps working, and
	// a window resize changes the layout the next frame uses.
	sdltest.ResizeWindow(t, as.window, 400, 300)
	sdltest.OnMain(t, func() { as.iterate() })
	img := sdltest.Capture(t, as.renderer)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= 0 || h <= 0 {
		t.Fatalf("captured %dx%d", w, h)
	}
	sdltest.AssertPixel(t, img, w/2, h/2, white)
}

func TestSetMetadata(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_VIDEO)
	if err := sdltest.Call(t, setMetadata); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		sdl.PROP_APP_METADATA_NAME_STRING:       "Example splitscreen shooter game",
		sdl.PROP_APP_METADATA_VERSION_STRING:    "1.0",
		sdl.PROP_APP_METADATA_IDENTIFIER_STRING: "com.example.woodeneye-008",
		sdl.PROP_APP_METADATA_CREATOR_STRING:    "SDL team",
		sdl.PROP_APP_METADATA_COPYRIGHT_STRING:  "Placed in the public domain",
		sdl.PROP_APP_METADATA_URL_STRING:        "https://examples.libsdl.org/SDL3/demo/02-woodeneye-008/",
		sdl.PROP_APP_METADATA_TYPE_STRING:       "game",
	} {
		if got := sdltest.Call(t, func() string { return sdl.GetAppMetadataProperty(name) }); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	for range 2 { // a second run proves Quit/Init cleanup left no state behind
		var err error
		sdltest.OnMain(t, func() { err = run(3) })
		if err != nil {
			t.Fatalf("run(3) = %v", err)
		}
	}
}
