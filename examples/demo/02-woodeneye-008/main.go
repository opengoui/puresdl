// Command woodeneye-008 is a minimal splitscreen FPS with multiple mice
// and keyboards, drawn with the 2D render API. Each mouse/keyboard that
// produces input claims a player slot (up to four).
// It's a port of SDL/examples/demo/02-woodeneye-008/woodeneye-008.c.
//
// Run it with:
//
//	go run ./examples/demo/02-woodeneye-008
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init pins the main goroutine to its OS thread, since SDL video and event
// calls must run on the thread that initialized them.
func init() {
	runtime.LockOSThread()
}

// Map and rendering parameters. binRad converts the binary angle
// representation (a full turn is 2^32) of yaw, and of pitch (a quarter turn
// is 2^30), to radians.
const (
	mapBoxScale     = 16
	mapBoxEdgesLen  = 12 + mapBoxScale*2
	maxPlayerCount  = 4
	circleDrawSides = 32
	binRad          = math.Pi / 2147483648.0
)

// player is one splitscreen participant bound to a mouse and a keyboard.
// A zero mouse or keyboard ID means the slot's device is unassigned. wasd is a
// bitmask: 1=W, 2=A, 4=S, 8=D, 16=jump.
type player struct {
	mouse    sdl.MouseID
	keyboard sdl.KeyboardID
	pos, vel [3]float64
	yaw      uint32
	pitch    int32
	radius   float32
	height   float32
	color    [3]uint8
	wasd     uint8
}

// appState is the whole application state: the SDL handles, the players, the
// map box edges, and the per-frame bookkeeping. It must only be used from the
// main thread.
type appState struct {
	window      *sdl.Window
	renderer    *sdl.Renderer
	playerCount int
	players     [maxPlayerCount]player
	edges       [mapBoxEdgesLen][6]float32

	// Per-iteration state (function-local statics in the C code).
	accu, last, past uint64
	debugString      string

	// now returns monotonic nanoseconds and delay sleeps for the given
	// nanoseconds; tests replace sdl.GetTicksNS and sdl.DelayNS.
	now   func() uint64
	delay func(ns uint64)
}

// newAppState returns the initial game state (one player, the map box edges)
// without touching SDL.
func newAppState(now func() uint64, delay func(ns uint64)) *appState {
	as := &appState{now: now, delay: delay, playerCount: 1}
	as.initPlayers()
	as.initEdges()
	return as
}

// main parses flags and runs the demo, exiting with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "woodeneye-008:", err)
		os.Exit(1)
	}
}

// whoseMouse returns the index of the player owning mouse id, or -1.
func (as *appState) whoseMouse(id sdl.MouseID) int {
	for i := 0; i < as.playerCount; i++ {
		if as.players[i].mouse == id {
			return i
		}
	}
	return -1
}

// whoseKeyboard returns the index of the player owning keyboard id, or -1.
func (as *appState) whoseKeyboard(id sdl.KeyboardID) int {
	for i := 0; i < as.playerCount; i++ {
		if as.players[i].keyboard == id {
			return i
		}
	}
	return -1
}

// shoot fires a ray from the shooter along its view direction. Any other
// player whose head or body sphere it hits is respawned at a random position.
func (as *appState) shoot(shooter int) {
	players := as.players[:as.playerCount]
	s := &players[shooter]
	x0, y0, z0 := s.pos[0], s.pos[1], s.pos[2]
	yawRad := binRad * float64(s.yaw)
	pitchRad := binRad * float64(s.pitch)
	cosYaw, sinYaw := math.Cos(yawRad), math.Sin(yawRad)
	cosPitch, sinPitch := math.Cos(pitchRad), math.Sin(pitchRad)
	vx := -sinYaw * cosPitch
	vy := sinPitch
	vz := -cosYaw * cosPitch
	for i := range players {
		if i == shooter {
			continue
		}
		target := &players[i]
		hit := 0
		for j := 0; j < 2; j++ {
			r := float64(target.radius)
			h := float64(target.height)
			dx := target.pos[0] - x0
			dy := target.pos[1] - y0
			if j != 0 {
				dy += r - h
			}
			dz := target.pos[2] - z0
			vd := vx*dx + vy*dy + vz*dz
			dd := dx*dx + dy*dy + dz*dz
			vv := vx*vx + vy*vy + vz*vz
			rr := r * r
			if vd < 0 {
				continue
			}
			if vd*vd >= vv*(dd-rr) {
				hit++
			}
		}
		if hit != 0 {
			for k := 0; k < 3; k++ {
				target.pos[k] = float64(mapBoxScale*(sdl.Rand(256)-128)) / 256
			}
		}
	}
}

// update integrates player movement over dtNS nanoseconds: WASD acceleration
// with drag, gravity, jumping, and clamping to the map box.
func update(players []player, dtNS uint64) {
	for i := range players {
		p := &players[i]
		const rate = 6.0
		const mult = 60.0
		const grav = 25.0
		t := float64(dtNS) * 1e-9
		drag := math.Exp(-t * rate)
		diff := 1.0 - drag
		rad := float64(p.yaw) * math.Pi / 2147483648.0
		cos, sin := math.Cos(rad), math.Sin(rad)
		wasd := p.wasd
		dirX := b2f(wasd&8 != 0) - b2f(wasd&2 != 0)
		dirZ := b2f(wasd&4 != 0) - b2f(wasd&1 != 0)
		norm := dirX*dirX + dirZ*dirZ
		var accX, accZ float64
		if norm != 0 {
			accX = mult * (cos*dirX + sin*dirZ) / math.Sqrt(norm)
			accZ = mult * (-sin*dirX + cos*dirZ) / math.Sqrt(norm)
		}
		velX, velY, velZ := p.vel[0], p.vel[1], p.vel[2]
		p.vel[0] -= velX * diff
		p.vel[1] -= grav * t
		p.vel[2] -= velZ * diff
		p.vel[0] += diff * accX / rate
		p.vel[2] += diff * accZ / rate
		p.pos[0] += (t-diff/rate)*accX/rate + diff*velX/rate
		p.pos[1] += -0.5*grav*t*t + velY*t
		p.pos[2] += (t-diff/rate)*accZ/rate + diff*velZ/rate
		const scale = float64(mapBoxScale)
		bound := scale - float64(p.radius)
		posX := math.Max(math.Min(bound, p.pos[0]), -bound)
		posY := math.Max(math.Min(bound, p.pos[1]), float64(p.height)-scale)
		posZ := math.Max(math.Min(bound, p.pos[2]), -bound)
		if p.pos[0] != posX {
			p.vel[0] = 0
		}
		if p.pos[1] != posY {
			if wasd&16 != 0 {
				p.vel[1] = 8.4375
			} else {
				p.vel[1] = 0
			}
		}
		if p.pos[2] != posZ {
			p.vel[2] = 0
		}
		p.pos = [3]float64{posX, posY, posZ}
	}
}

// b2f converts a bool to 1 or 0.
func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// drawCircle outlines a circle of radius r centered at (x, y) with line
// segments.
func drawCircle(renderer *sdl.Renderer, r, x, y float32) {
	var points [circleDrawSides + 1]sdl.FPoint
	for i := range points {
		ang := 2 * math.Pi * float32(i) / circleDrawSides
		points[i].X = x + r*float32(math.Cos(float64(ang)))
		points[i].Y = y + r*float32(math.Sin(float64(ang)))
	}
	sdl.RenderLinesSlice(renderer, points[:])
}

// drawClippedSegment projects the camera-space segment (a, b) onto the screen
// around origin (x, y) with focal length z, clipping it against the near plane
// at depth -w. Segments entirely in front of the near plane are skipped.
func drawClippedSegment(renderer *sdl.Renderer, ax, ay, az, bx, by, bz, x, y, z, w float32) {
	if az >= -w && bz >= -w {
		return
	}
	dx := ax - bx
	dy := ay - by
	if az > -w {
		t := (-w - bz) / (az - bz)
		ax = bx + dx*t
		ay = by + dy*t
		az = -w
	} else if bz > -w {
		t := (-w - az) / (bz - az)
		bx = ax - dx*t
		by = ay - dy*t
		bz = -w
	}
	ax = -z * ax / az
	ay = -z * ay / az
	bx = -z * bx / bz
	by = -z * by / bz
	sdl.RenderLine(renderer, x+ax, y-ay, x+bx, y-by)
}

// draw renders every player's viewport and the debug text; it does not present.
func (as *appState) draw() {
	renderer := as.renderer
	var w, h int32
	if !sdl.GetRenderOutputSize(renderer, &w, &h) {
		return
	}
	sdl.SetRenderDrawColor(renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(renderer)
	players := as.players[:as.playerCount]
	if len(players) > 0 {
		wf, hf := float32(w), float32(h)
		partHor, partVer := 1, 1
		if len(players) > 2 {
			partHor = 2
		}
		if len(players) > 1 {
			partVer = 2
		}
		sizeHor := wf / float32(partHor)
		sizeVer := hf / float32(partVer)
		for i := range players {
			pl := &players[i]
			modX := float32(i % partHor)
			modY := float32(i / partHor)
			horOrigin := (modX + 0.5) * sizeHor
			verOrigin := (modY + 0.5) * sizeVer
			camOrigin := float32(0.5 * math.Sqrt(float64(sizeHor*sizeHor+sizeVer*sizeVer)))
			rect := sdl.Rect{X: int32(modX * sizeHor), Y: int32(modY * sizeVer), W: int32(sizeHor), H: int32(sizeVer)}
			sdl.SetRenderClipRect(renderer, &rect)
			x0, y0, z0 := pl.pos[0], pl.pos[1], pl.pos[2]
			yawRad := binRad * float64(pl.yaw)
			pitchRad := binRad * float64(pl.pitch)
			cosYaw, sinYaw := math.Cos(yawRad), math.Sin(yawRad)
			cosPitch, sinPitch := math.Cos(pitchRad), math.Sin(pitchRad)
			mat := [9]float64{
				cosYaw, 0, -sinYaw,
				sinYaw * sinPitch, cosPitch, cosYaw * sinPitch,
				sinYaw * cosPitch, -sinPitch, cosYaw * cosPitch,
			}
			sdl.SetRenderDrawColor(renderer, 64, 64, 64, 255)
			for k := range as.edges {
				line := &as.edges[k]
				l0, l1, l2 := float64(line[0])-x0, float64(line[1])-y0, float64(line[2])-z0
				l3, l4, l5 := float64(line[3])-x0, float64(line[4])-y0, float64(line[5])-z0
				ax := float32(mat[0]*l0 + mat[1]*l1 + mat[2]*l2)
				ay := float32(mat[3]*l0 + mat[4]*l1 + mat[5]*l2)
				az := float32(mat[6]*l0 + mat[7]*l1 + mat[8]*l2)
				bx := float32(mat[0]*l3 + mat[1]*l4 + mat[2]*l5)
				by := float32(mat[3]*l3 + mat[4]*l4 + mat[5]*l5)
				bz := float32(mat[6]*l3 + mat[7]*l4 + mat[8]*l5)
				drawClippedSegment(renderer, ax, ay, az, bx, by, bz, horOrigin, verOrigin, camOrigin, 1)
			}
			for j := range players {
				if i == j {
					continue
				}
				target := &players[j]
				sdl.SetRenderDrawColor(renderer, target.color[0], target.color[1], target.color[2], 255)
				for k := 0; k < 2; k++ {
					rx := target.pos[0] - pl.pos[0]
					ry := target.pos[1] - pl.pos[1] + float64((target.radius-target.height)*float32(k))
					rz := target.pos[2] - pl.pos[2]
					dx := mat[0]*rx + mat[1]*ry + mat[2]*rz
					dy := mat[3]*rx + mat[4]*ry + mat[5]*rz
					dz := mat[6]*rx + mat[7]*ry + mat[8]*rz
					rEff := float64(target.radius) * float64(camOrigin) / dz
					if !(dz < 0) {
						continue
					}
					drawCircle(renderer, float32(rEff),
						float32(float64(horOrigin)-float64(camOrigin)*dx/dz),
						float32(float64(verOrigin)+float64(camOrigin)*dy/dz))
				}
			}
			sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
			sdl.RenderLine(renderer, horOrigin, verOrigin-10, horOrigin, verOrigin+10)
			sdl.RenderLine(renderer, horOrigin-10, verOrigin, horOrigin+10, verOrigin)
		}
	}
	sdl.SetRenderClipRect(renderer, nil)
	sdl.SetRenderDrawColor(renderer, 255, 255, 255, 255)
	sdl.RenderDebugText(renderer, 0, 0, as.debugString)
}

// initPlayers places all player slots at the box corners with distinct
// headings and colors, and resets their velocity.
func (as *appState) initPlayers() {
	for i := range as.players {
		p := &as.players[i]
		sgn := func(bit int) float64 {
			if i&bit != 0 {
				return -1
			}
			return 1
		}
		p.pos = [3]float64{8.0 * sgn(1), 0, 8.0 * sgn(1) * sgn(2)}
		p.vel = [3]float64{}
		p.yaw = 0x20000000
		if i&1 != 0 {
			p.yaw += 0x80000000
		}
		if i&2 != 0 {
			p.yaw += 0x40000000
		}
		p.pitch = -0x08000000
		p.radius = 0.5
		p.height = 1.5
		var c [3]uint8
		for k, bit := range [3]int{2, 1, 4} {
			if (1<<(i/2))&bit != 0 {
				c[k] = 0
			} else {
				c[k] = 0xff
			}
			if i&1 == 0 {
				c[k] = ^c[k]
			}
		}
		p.color = c
	}
}

// initEdges fills as.edges with the 12 edges of the map cube followed by the
// floor grid lines.
func (as *appState) initEdges() {
	const r = float32(mapBoxScale)
	m := [24]int{
		0, 1, 1, 3, 3, 2, 2, 0,
		7, 6, 6, 4, 4, 5, 5, 7,
		6, 2, 3, 7, 0, 4, 5, 1,
	}
	pick := func(v, j int) float32 {
		if v&(1<<j) != 0 {
			return r
		}
		return -r
	}
	for i := 0; i < 12; i++ {
		for j := 0; j < 3; j++ {
			as.edges[i][j] = pick(m[i*2], j)
			as.edges[i][j+3] = pick(m[i*2+1], j)
		}
	}
	for i := 0; i < mapBoxScale; i++ {
		d := float32(i * 2)
		for j := 0; j < 2; j++ {
			end := -r
			if j != 0 {
				end = r
			}
			e := &as.edges[i+12]
			e[3*j+0] = end
			e[3*j+1] = -r
			e[3*j+2] = d - r
			e = &as.edges[i+12+mapBoxScale]
			e[3*j+0] = d - r
			e[3*j+1] = -r
			e[3*j+2] = end
		}
	}
}

// event handles one event and reports whether the app should quit.
func (as *appState) event(ev *sdl.Event) bool {
	players := as.players[:]
	switch sdl.EventType(ev.Type()) {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_MOUSE_REMOVED:
		for i := 0; i < as.playerCount; i++ {
			if players[i].mouse == ev.Mdevice().Which {
				players[i].mouse = 0
			}
		}
	case sdl.EVENT_KEYBOARD_REMOVED:
		for i := 0; i < as.playerCount; i++ {
			if players[i].keyboard == ev.Kdevice().Which {
				players[i].keyboard = 0
			}
		}
	case sdl.EVENT_MOUSE_MOTION:
		m := ev.Motion()
		id := m.Which
		if index := as.whoseMouse(id); index >= 0 {
			p := &players[index]
			p.yaw -= uint32(int32(m.Xrel) * 0x00080000)
			p.pitch = max(-0x40000000, min(0x40000000, p.pitch-int32(m.Yrel)*0x00080000))
		} else if id != 0 {
			for i := range players {
				if players[i].mouse == 0 {
					players[i].mouse = id
					as.playerCount = max(as.playerCount, i+1)
					break
				}
			}
		}
	case sdl.EVENT_MOUSE_BUTTON_DOWN:
		if index := as.whoseMouse(ev.Button().Which); index >= 0 {
			as.shoot(index)
		}
	case sdl.EVENT_KEY_DOWN:
		k := ev.Key()
		if index := as.whoseKeyboard(k.Which); index >= 0 {
			switch k.Scancode {
			case sdl.SCANCODE_W:
				players[index].wasd |= 1
			case sdl.SCANCODE_A:
				players[index].wasd |= 2
			case sdl.SCANCODE_S:
				players[index].wasd |= 4
			case sdl.SCANCODE_D:
				players[index].wasd |= 8
			case sdl.SCANCODE_SPACE:
				players[index].wasd |= 16
			}
		} else if k.Which != 0 {
			for i := range players {
				if players[i].keyboard == 0 {
					players[i].keyboard = k.Which
					as.playerCount = max(as.playerCount, i+1)
					break
				}
			}
		}
	case sdl.EVENT_KEY_UP:
		k := ev.Key()
		if k.Scancode == sdl.SCANCODE_ESCAPE {
			return true
		}
		if index := as.whoseKeyboard(k.Which); index >= 0 {
			switch k.Scancode {
			case sdl.SCANCODE_W:
				players[index].wasd &= 30
			case sdl.SCANCODE_A:
				players[index].wasd &= 29
			case sdl.SCANCODE_S:
				players[index].wasd &= 27
			case sdl.SCANCODE_D:
				players[index].wasd &= 23
			case sdl.SCANCODE_SPACE:
				players[index].wasd &= 15
			}
		}
	}
	return false
}

// iterate runs one frame: it updates the players, draws and presents, refreshes
// the fps text once a second, and sleeps to cap the frame rate near 1000 fps.
func (as *appState) iterate() {
	now := as.now()
	dt := now - as.past
	update(as.players[:as.playerCount], dt)
	as.draw()
	sdl.RenderPresent(as.renderer)
	if now-as.last > 999999999 {
		as.last = now
		as.debugString = fmt.Sprintf("%d fps", as.accu)
		as.accu = 0
	}
	as.past = now
	as.accu++
	if elapsed := as.now() - now; elapsed < 999999 {
		as.delay(999999 - elapsed)
	}
}

// setMetadata publishes the app metadata the C program sets in SDL_AppInit.
func setMetadata() error {
	if !sdl.SetAppMetadata("Example splitscreen shooter game", "1.0", "com.example.woodeneye-008") {
		return fmt.Errorf("SDL_SetAppMetadata: %s", sdl.GetError())
	}
	for _, kv := range [][2]string{
		{sdl.PROP_APP_METADATA_URL_STRING, "https://examples.libsdl.org/SDL3/demo/02-woodeneye-008/"},
		{sdl.PROP_APP_METADATA_CREATOR_STRING, "SDL team"},
		{sdl.PROP_APP_METADATA_COPYRIGHT_STRING, "Placed in the public domain"},
		{sdl.PROP_APP_METADATA_TYPE_STRING, "game"},
	} {
		if !sdl.SetAppMetadataPropertyValue(kv[0], kv[1]) {
			return fmt.Errorf("SDL_SetAppMetadataProperty: %s", sdl.GetError())
		}
	}
	return nil
}

// open creates the window and renderer and applies the C program's
// configuration: vsync off, relative mouse mode, raw keyboard hint.
func (as *appState) open() error {
	if !sdl.CreateWindowAndRenderer("examples/demo/woodeneye-008", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &as.window, &as.renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	sdl.SetRenderVSync(as.renderer, sdl.RENDERER_VSYNC_DISABLED)
	sdl.SetWindowRelativeMouseMode(as.window, true)
	sdl.SetHintWithPriority(sdl.HINT_WINDOWS_RAW_KEYBOARD, "1", sdl.HINT_OVERRIDE)
	return nil
}

// close destroys the renderer and window created by open.
func (as *appState) close() {
	sdl.DestroyRenderer(as.renderer)
	sdl.DestroyWindow(as.window)
	as.renderer, as.window = nil, nil
}

// run initializes SDL and the window, then runs the event/iterate loop until a
// quit request or, when maxFrames is positive, that many frames. It must be
// called on the main thread.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	if err := setMetadata(); err != nil {
		return err
	}

	as := newAppState(sdl.GetTicksNS, sdl.DelayNS)
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()
	if err := as.open(); err != nil {
		return err
	}
	defer as.close()

	var ev sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&ev) {
			if as.event(&ev) {
				return nil
			}
		}
		as.iterate()
	}
	return nil
}
