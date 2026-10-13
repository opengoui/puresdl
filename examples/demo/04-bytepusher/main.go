// Command 04-bytepusher is an implementation of the BytePusher VM.
// It is a port of SDL/examples/demo/04-bytepusher/bytepusher.c
//
// For example programs and more information about BytePusher, see
// https://esolangs.org/wiki/BytePusher
//
// Run it with:
//
//	go run ./examples/demo/04-bytepusher [-frames N] [file.bp]
//
// Drop a BytePusher file on the window (or pass it as an argument) to run it.
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

// VM and display parameters. The io* constants are RAM addresses (or offsets)
// of the BytePusher memory-mapped registers.
const (
	screenW               = 256
	screenH               = 256
	ramSize               = 0x1000000
	framesPerSecond       = 60
	samplesPerFrame       = 256
	nsPerSecond           = uint64(sdl.NS_PER_SECOND)
	maxAudioLatencyFrames = 5

	ioKeyboard   = 0
	ioPC         = 2
	ioScreenPage = 5
	ioAudioBank  = 6
)

// bytePusher is the VM plus its SDL resources.
type bytePusher struct {
	ram         [ramSize + 8]uint8
	lastTick    uint64
	tickAcc     uint64
	window      *sdl.Window
	renderer    *sdl.Renderer
	now         func() uint64 // monotonic time in nanoseconds
	palette     *sdl.Palette
	texture     *sdl.Texture
	audioStream *sdl.AudioStream // may be nil if no audio device is available

	status      string
	statusTicks int
	keystate    uint16
	displayHelp bool
	positional  bool
}

// readU16 reads a big-endian 16-bit value at RAM address addr.
func (vm *bytePusher) readU16(addr uint32) uint16 {
	return uint16(vm.ram[addr])<<8 | uint16(vm.ram[addr+1])
}

// readU24 reads a big-endian 24-bit value at RAM address addr.
func (vm *bytePusher) readU24(addr uint32) uint32 {
	return uint32(vm.ram[addr])<<16 | uint32(vm.ram[addr+1])<<8 | uint32(vm.ram[addr+2])
}

// setStatus formats a status line, truncated to fit the screen width, and shows
// it for three seconds worth of VM ticks.
func (vm *bytePusher) setStatus(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	// The C status buffer holds SCREEN_W/8 bytes including the terminator.
	if len(s) > screenW/8-1 {
		s = s[:screenW/8-1]
	}
	vm.status = s
	vm.statusTicks = framesPerSecond * 3
}

// load reads a program from stream into RAM and closes the stream. It reports
// whether the whole stream was read up to EOF; a nil stream or a read error
// returns false and turns the help screen on.
func (vm *bytePusher) load(stream *sdl.IOStream) bool {
	vm.displayHelp = true // set to false if load succeeds
	clear(vm.ram[:ramSize])
	if stream == nil {
		return false
	}

	ok := true
	bytesRead := 0
	for bytesRead < ramSize {
		n := int(sdl.ReadIOBytes(stream, vm.ram[bytesRead:ramSize]))
		bytesRead += n
		if n == 0 {
			ok = sdl.GetIOStatus(stream) == sdl.IO_STATUS_EOF
			break
		}
	}
	sdl.CloseIO(stream)

	if vm.audioStream != nil {
		sdl.ClearAudioStream(vm.audioStream)
	}
	vm.displayHelp = !ok
	return ok
}

// filename returns the last path component, accepting both separators.
func filename(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// loadFile loads the program at path, reports the outcome in the status line,
// and returns whether loading succeeded.
func (vm *bytePusher) loadFile(path string) bool {
	if vm.load(sdl.IOFromFile(path, "rb")) {
		vm.setStatus("loaded %s", filename(path))
		return true
	}
	vm.setStatus("load failed: %s", filename(path))
	return false
}

// print draws str with a one-pixel black drop shadow.
func (vm *bytePusher) print(x, y int, str string) {
	sdl.SetRenderDrawColor(vm.renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
	sdl.RenderDebugText(vm.renderer, float32(x+1), float32(y+1), str)
	sdl.SetRenderDrawColor(vm.renderer, 0xff, 0xff, 0xff, sdl.ALPHA_OPAQUE)
	sdl.RenderDebugText(vm.renderer, float32(x), float32(y), str)
	sdl.SetRenderDrawColor(vm.renderer, 0, 0, 0, sdl.ALPHA_OPAQUE)
}

// keycodeMask maps a symbolic key (0-9, keypad digits, A-F) to its bit in the
// 16-key keypad state, or 0 if the key is not a keypad key.
func keycodeMask(key sdl.Keycode) uint16 {
	var index int
	switch {
	case key >= sdl.SDLK_0 && key <= sdl.SDLK_9:
		index = int(key - sdl.SDLK_0)
	case key >= sdl.SDLK_KP_1 && key <= sdl.SDLK_KP_9:
		index = int(key-sdl.SDLK_KP_1) + 1
	case key == sdl.SDLK_KP_0:
		index = 0
	case key >= sdl.SDLK_A && key <= sdl.SDLK_F:
		index = int(key-sdl.SDLK_A) + 10
	default:
		return 0
	}
	return 1 << index
}

// scancodeMask maps a physical key in the 1234/QWER/ASDF/ZXCV block to its bit
// in the 16-key keypad state, or 0 for any other key.
func scancodeMask(sc sdl.Scancode) uint16 {
	var index int
	switch sc {
	case sdl.SCANCODE_1:
		index = 0x1
	case sdl.SCANCODE_2:
		index = 0x2
	case sdl.SCANCODE_3:
		index = 0x3
	case sdl.SCANCODE_4:
		index = 0xc
	case sdl.SCANCODE_Q:
		index = 0x4
	case sdl.SCANCODE_W:
		index = 0x5
	case sdl.SCANCODE_E:
		index = 0x6
	case sdl.SCANCODE_R:
		index = 0xd
	case sdl.SCANCODE_A:
		index = 0x7
	case sdl.SCANCODE_S:
		index = 0x8
	case sdl.SCANCODE_D:
		index = 0x9
	case sdl.SCANCODE_F:
		index = 0xe
	case sdl.SCANCODE_Z:
		index = 0xa
	case sdl.SCANCODE_X:
		index = 0x0
	case sdl.SCANCODE_C:
		index = 0xb
	case sdl.SCANCODE_V:
		index = 0xf
	default:
		return 0
	}
	return 1 << index
}

// iterate runs one VM tick (when due) and renders a frame.
func (vm *bytePusher) iterate() {
	tick := vm.now()
	delta := tick - vm.lastTick
	vm.lastTick = tick

	vm.tickAcc += delta * framesPerSecond
	updated := vm.tickAcc >= nsPerSecond
	skipAudio := vm.tickAcc >= maxAudioLatencyFrames*nsPerSecond

	if skipAudio && vm.audioStream != nil {
		// Don't let audio fall too far behind.
		sdl.ClearAudioStream(vm.audioStream)
	}

	for vm.tickAcc >= nsPerSecond {
		vm.tickAcc -= nsPerSecond

		vm.ram[ioKeyboard] = uint8(vm.keystate >> 8)
		vm.ram[ioKeyboard+1] = uint8(vm.keystate)

		pc := vm.readU24(ioPC)
		for range screenW * screenH {
			src := vm.readU24(pc)
			dst := vm.readU24(pc + 3)
			vm.ram[dst] = vm.ram[src]
			pc = vm.readU24(pc + 6)
		}

		if vm.audioStream != nil && (!skipAudio || vm.tickAcc < nsPerSecond) {
			bank := uint32(vm.readU16(ioAudioBank)) << 8
			sdl.PutAudioStreamBytes(vm.audioStream, vm.ram[bank:bank+samplesPerFrame])
		}
	}

	if updated {
		page := uint32(vm.ram[ioScreenPage]) << 16
		sdl.UpdateTextureBytes(vm.texture, nil, vm.ram[page:page+screenW*screenH], screenW)
	}

	sdl.RenderClear(vm.renderer)

	if vm.displayHelp {
		vm.print(4, 4, "Drop a BytePusher file in this")
		vm.print(8, 12, "window to load and run it!")
		vm.print(4, 28, "Press ENTER to switch between")
		vm.print(8, 36, "positional and symbolic input.")
	} else {
		sdl.RenderTexture(vm.renderer, vm.texture, nil, nil)
	}

	if vm.statusTicks > 0 {
		if updated {
			vm.statusTicks--
		}
		vm.print(4, screenH-12, vm.status)
	}

	sdl.RenderPresent(vm.renderer)
}

// handleEvent applies one event (file drop, key input, quit) and returns false
// when the app should quit.
func (vm *bytePusher) handleEvent(event *sdl.Event) bool {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return false

	case sdl.EVENT_DROP_FILE:
		vm.loadFile(event.Drop().DataString())

	case sdl.EVENT_KEY_DOWN:
		key := event.Key()
		if key.Key == sdl.SDLK_ESCAPE {
			return false
		}
		if key.Key == sdl.SDLK_RETURN {
			vm.positional = !vm.positional
			vm.keystate = 0
			if vm.positional {
				vm.setStatus("switched to positional input")
			} else {
				vm.setStatus("switched to symbolic input")
			}
		}
		if vm.positional {
			vm.keystate |= scancodeMask(key.Scancode)
		} else {
			vm.keystate |= keycodeMask(key.Key)
		}

	case sdl.EVENT_KEY_UP:
		key := event.Key()
		if vm.positional {
			vm.keystate &^= scancodeMask(key.Scancode)
		} else {
			vm.keystate &^= keycodeMask(key.Key)
		}
	}
	return true
}

// main parses flags and runs the VM, exiting with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "04-bytepusher:", err)
		os.Exit(1)
	}
}

// run initializes SDL, the VM and audio, optionally loads program, and runs the
// event/iterate loop until quit or, when maxFrames is positive, that many
// frames. A missing audio device is not an error. It must be called on the main
// thread.
func run(maxFrames int, program string) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	if !sdl.SetAppMetadata("SDL 3 BytePusher", "1.0", "com.example.SDL3BytePusher") {
		return fmt.Errorf("SDL_SetAppMetadata: %s", sdl.GetError())
	}
	for _, kv := range [][2]string{
		{sdl.PROP_APP_METADATA_URL_STRING, "https://examples.libsdl.org/SDL3/demo/04-bytepusher/"},
		{sdl.PROP_APP_METADATA_CREATOR_STRING, "SDL team"},
		{sdl.PROP_APP_METADATA_COPYRIGHT_STRING, "Placed in the public domain"},
		{sdl.PROP_APP_METADATA_TYPE_STRING, "game"},
	} {
		if !sdl.SetAppMetadataPropertyValue(kv[0], kv[1]) {
			return fmt.Errorf("SDL_SetAppMetadataProperty: %s", sdl.GetError())
		}
	}

	if !sdl.Init(sdl.INIT_AUDIO | sdl.INIT_VIDEO) {
		return fmt.Errorf("SDL_Init: %s", sdl.GetError())
	}
	defer sdl.Quit()

	vm := &bytePusher{}

	zoom := int32(2)
	var usable sdl.Rect
	if sdl.GetDisplayUsableBounds(sdl.GetPrimaryDisplay(), &usable) {
		zoomW := (usable.W - usable.X) * 2 / 3 / screenW
		zoomH := (usable.H - usable.Y) * 2 / 3 / screenH
		zoom = max(min(zoomW, zoomH), 1)
	}

	if !sdl.CreateWindowAndRenderer("SDL 3 BytePusher", screenW*zoom, screenH*zoom,
		sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &vm.window, &vm.renderer) {
		return fmt.Errorf("SDL_CreateWindowAndRenderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(vm.window)
	defer sdl.DestroyRenderer(vm.renderer)

	if err := vm.init(sdl.GetTicksNS); err != nil {
		return err
	}
	defer vm.close()

	spec := audioSpec()
	vm.audioStream = sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, sdl.NoAudioStreamCallback, nil)
	if vm.audioStream == nil {
		// The C program fails here; keep running silently instead.
		fmt.Fprintln(os.Stderr, "04-bytepusher: no audio:", sdl.GetError())
	} else {
		sdl.SetAudioStreamGain(vm.audioStream, 0.1) // examples are loud!
		sdl.ResumeAudioStreamDevice(vm.audioStream)
	}

	if program != "" {
		vm.loadFile(program)
	}

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			if !vm.handleEvent(&event) {
				return nil
			}
		}
		vm.iterate()
	}
	return nil
}

// audioSpec returns the mono signed 8-bit format the VM emits, one bank of
// samplesPerFrame samples per frame.
func audioSpec() sdl.AudioSpec {
	return sdl.AudioSpec{Format: sdl.AUDIO_S8, Channels: 1, Freq: samplesPerFrame * framesPerSecond}
}

// init sets up everything that depends only on vm.renderer: logical
// presentation, the colour-cube palette, the screen texture and the initial
// timing state. Audio is attached separately through vm.audioStream.
func (vm *bytePusher) init(now func() uint64) error {
	vm.displayHelp = true
	vm.now = now

	if !sdl.SetRenderLogicalPresentation(vm.renderer, screenW, screenH, sdl.LOGICAL_PRESENTATION_INTEGER_SCALE) {
		return fmt.Errorf("SDL_SetRenderLogicalPresentation: %s", sdl.GetError())
	}

	// Palette: a 6x6x6 colour cube, remaining entries black.
	vm.palette = sdl.CreatePalette(256)
	if vm.palette == nil {
		return fmt.Errorf("SDL_CreatePalette: %s", sdl.GetError())
	}
	colors := vm.palette.ColorSlice()
	i := 0
	for r := range 6 {
		for g := range 6 {
			for b := range 6 {
				colors[i] = sdl.Color{R: uint8(r * 0x33), G: uint8(g * 0x33), B: uint8(b * 0x33), A: sdl.ALPHA_OPAQUE}
				i++
			}
		}
	}
	for ; i < 256; i++ {
		colors[i] = sdl.Color{A: sdl.ALPHA_OPAQUE}
	}

	vm.texture = sdl.CreateTexture(vm.renderer, sdl.PIXELFORMAT_INDEX8, sdl.TEXTUREACCESS_STREAMING, screenW, screenH)
	if vm.texture == nil {
		sdl.DestroyPalette(vm.palette)
		vm.palette = nil
		return fmt.Errorf("SDL_CreateTexture: %s", sdl.GetError())
	}
	sdl.SetTexturePalette(vm.texture, vm.palette)
	sdl.SetTextureScaleMode(vm.texture, sdl.SCALEMODE_NEAREST)

	vm.setStatus("renderer: %s", sdl.GetRendererName(vm.renderer))

	vm.lastTick = vm.now()
	vm.tickAcc = nsPerSecond
	return nil
}

// close releases the audio stream, texture and palette. The window and
// renderer belong to the caller.
func (vm *bytePusher) close() {
	if vm.audioStream != nil {
		sdl.DestroyAudioStream(vm.audioStream)
		vm.audioStream = nil
	}
	if vm.texture != nil {
		sdl.DestroyTexture(vm.texture)
		vm.texture = nil
	}
	if vm.palette != nil {
		sdl.DestroyPalette(vm.palette)
		vm.palette = nil
	}
}
