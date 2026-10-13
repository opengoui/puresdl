package main

import (
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

// frameNS is slightly more than one VM frame (1/60 s), so every step of the
// fake clock yields exactly one VM tick: the 20 ns surplus per step cannot add
// up to a whole extra tick within a test.
const frameNS = 16_666_667

type fixture struct {
	vm    *bytePusher
	ns    uint64
	in    *sdltest.Input
	probe *sdltest.AudioProbe
	cv    *sdltest.Canvas
}

func (f *fixture) step(frames uint64) { f.ns += frames * frameNS }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO|sdl.INIT_AUDIO)
	f := &fixture{in: sdltest.NewInput(t, 0)}
	f.cv = sdltest.NewCanvas(t, screenW, screenH)
	f.probe = sdltest.NewAudioProbe(t, audioSpec())
	f.vm = &bytePusher{renderer: f.cv.Renderer, audioStream: f.probe.Stream}
	var err error
	sdltest.OnMain(t, func() { err = f.vm.init(func() uint64 { return f.ns }) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The probe owns the stream; do not let close destroy it twice.
		f.vm.audioStream = nil
		sdltest.OnMain(t, f.vm.close)
	})
	return f
}

func (f *fixture) iterate(t *testing.T) {
	t.Helper()
	sdltest.OnMain(t, f.vm.iterate)
}

// feed delivers all queued SDL events to the VM like the run loop does.
func (f *fixture) feed(t *testing.T) (quit bool) {
	t.Helper()
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			if !f.vm.handleEvent(&e) {
				quit = true
			}
		}
	})
	return quit
}

func (f *fixture) capture(t *testing.T) *image.RGBA { return f.cv.Capture(t) }

func hasWhite(img *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) == (color.RGBA{255, 255, 255, 255}) {
				return true
			}
		}
	}
	return false
}

var (
	helpArea   = image.Rect(0, 0, screenW, 48)
	statusArea = image.Rect(0, screenH-12, screenW, screenH)
)

func writeProgram(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// program returns a minimal BytePusher image: PC=0x100 pointing at a
// self-looping instruction that copies ram[0x200] to ram[0x300].
func program() []byte {
	b := make([]byte, 0x400)
	b[2], b[3], b[4] = 0, 0x01, 0x00 // PC
	b[5] = 1                         // screen page 0x01xxxx
	b[6], b[7] = 0x02, 0x00          // audio bank 0x0200xx
	copy(b[0x100:], []byte{0, 2, 0, 0, 3, 0, 0, 1, 0})
	b[0x200] = 0x42
	return b
}

func TestKeyMasks(t *testing.T) {
	// Hex keypad layout of the BytePusher spec: bit index == key value.
	symbolic := map[sdl.Keycode]uint16{
		sdl.SDLK_0: 1 << 0, sdl.SDLK_9: 1 << 9, sdl.SDLK_KP_0: 1 << 0,
		sdl.SDLK_KP_1: 1 << 1, sdl.SDLK_KP_9: 1 << 9,
		sdl.SDLK_A: 1 << 0xa, sdl.SDLK_F: 1 << 0xf,
		sdl.SDLK_G: 0, sdl.SDLK_SPACE: 0,
	}
	for k, want := range symbolic {
		if got := keycodeMask(k); got != want {
			t.Errorf("keycodeMask(%d) = %#x, want %#x", k, got, want)
		}
	}
	positional := map[sdl.Scancode]int{
		sdl.SCANCODE_1: 1, sdl.SCANCODE_2: 2, sdl.SCANCODE_3: 3, sdl.SCANCODE_4: 0xc,
		sdl.SCANCODE_Q: 4, sdl.SCANCODE_W: 5, sdl.SCANCODE_E: 6, sdl.SCANCODE_R: 0xd,
		sdl.SCANCODE_A: 7, sdl.SCANCODE_S: 8, sdl.SCANCODE_D: 9, sdl.SCANCODE_F: 0xe,
		sdl.SCANCODE_Z: 0xa, sdl.SCANCODE_X: 0, sdl.SCANCODE_C: 0xb, sdl.SCANCODE_V: 0xf,
	}
	for sc, idx := range positional {
		if got := scancodeMask(sc); got != 1<<idx {
			t.Errorf("scancodeMask(%d) = %#x, want %#x", sc, got, 1<<idx)
		}
	}
	if scancodeMask(sdl.SCANCODE_5) != 0 || scancodeMask(sdl.SCANCODE_SPACE) != 0 {
		t.Error("unmapped scancodes must yield no bits")
	}
}

func TestFilename(t *testing.T) {
	for in, want := range map[string]string{
		"a.bp": "a.bp", "/x/y/a.bp": "a.bp", `C:\x\a.bp`: "a.bp", `/x\y/a.bp`: "a.bp", "/x/": "",
	} {
		if got := filename(in); got != want {
			t.Errorf("filename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitialFrameShowsHelpAndRendererStatus(t *testing.T) {
	f := newFixture(t)
	if !strings.HasPrefix(f.vm.status, "renderer: ") || f.vm.statusTicks != framesPerSecond*3 {
		t.Fatalf("status = %q ticks %d", f.vm.status, f.vm.statusTicks)
	}
	f.iterate(t)
	img := f.capture(t)
	if !hasWhite(img, helpArea) {
		t.Error("help text not drawn before a program is loaded")
	}
	if !hasWhite(img, statusArea) {
		t.Error("status line not drawn")
	}
	// Outside text the cleared background is black (the very first clear still
	// uses the renderer's initial draw colour, so only RGB is asserted).
	if p := img.RGBAAt(200, 100); p.R != 0 || p.G != 0 || p.B != 0 {
		t.Errorf("background = %v", p)
	}
	f.step(1)
	f.iterate(t)
	sdltest.AssertPixel(t, f.capture(t), 200, 100, color.RGBA{0, 0, 0, 255})
}

func TestStatusExpiresAfterThreeSecondsOfVMTicks(t *testing.T) {
	f := newFixture(t)
	f.iterate(t) // first tick is owed from the start: decrements to 179
	if f.vm.statusTicks != framesPerSecond*3-1 {
		t.Fatalf("statusTicks = %d", f.vm.statusTicks)
	}
	// Without elapsed time no tick runs, so the counter must not move.
	f.iterate(t)
	if f.vm.statusTicks != framesPerSecond*3-1 {
		t.Fatalf("statusTicks moved without a tick: %d", f.vm.statusTicks)
	}
	for range framesPerSecond*3 - 1 {
		f.step(1)
		f.iterate(t)
	}
	if f.vm.statusTicks != 0 {
		t.Fatalf("statusTicks = %d, want 0", f.vm.statusTicks)
	}
	// The frame that consumed the last tick still shows the message (as in C);
	// the next frame must not.
	if !hasWhite(f.capture(t), statusArea) {
		t.Error("status missing on its final frame")
	}
	f.step(1)
	f.iterate(t)
	if hasWhite(f.capture(t), statusArea) {
		t.Error("status still drawn after expiry")
	}
}

func TestLoadFileRunsProgramAndShowsScreen(t *testing.T) {
	f := newFixture(t)
	path := writeProgram(t, "demo.bp", program())
	if !f.vm.loadFile(path) {
		t.Fatal("loadFile failed")
	}
	if f.vm.displayHelp || f.vm.status != "loaded demo.bp" {
		t.Fatalf("displayHelp=%v status=%q", f.vm.displayHelp, f.vm.status)
	}
	// Screen page 1: a row ramp of palette indices.
	for y := range screenH {
		for x := range screenW {
			f.vm.ram[1<<16+y*screenW+x] = uint8(x)
		}
	}
	f.iterate(t) // owed first tick
	if f.vm.ram[0x300] != 0x42 {
		t.Fatalf("instruction did not execute: ram[0x300] = %#x", f.vm.ram[0x300])
	}
	img := f.capture(t)
	if hasWhite(img, image.Rect(0, 0, 200, 48)) {
		t.Error("help still drawn after successful load")
	}
	for _, tc := range []struct {
		x    int
		want color.RGBA
	}{
		{0, color.RGBA{0, 0, 0, 255}},
		{1, color.RGBA{0, 0, 0x33, 255}},
		{6, color.RGBA{0, 0x33, 0, 255}},
		{36, color.RGBA{0x33, 0, 0, 255}},
		{43, color.RGBA{0x33, 0x33, 0x33, 255}},
		{215, color.RGBA{255, 255, 255, 255}},
		{216, color.RGBA{0, 0, 0, 255}}, // beyond the colour cube: black
		{255, color.RGBA{0, 0, 0, 255}},
	} {
		sdltest.AssertPixel(t, img, tc.x, 100, tc.want)
	}
}

func TestScreenPageSelectsFramebuffer(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", nil))
	f.vm.ram[ioScreenPage] = 3
	for i := range screenW * screenH {
		f.vm.ram[3<<16+i] = 215
	}
	f.iterate(t)
	sdltest.AssertRegion(t, f.capture(t), image.Rect(0, 0, 256, 240), color.RGBA{255, 255, 255, 255})
}

func TestTextureOnlyUpdatesWhenATickRuns(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", nil))
	f.vm.ram[ioScreenPage] = 3
	f.iterate(t) // tick: texture = black page 3
	for i := range screenW * screenH {
		f.vm.ram[3<<16+i] = 215
	}
	f.iterate(t) // no time elapsed: no tick, texture keeps the old pixels
	sdltest.AssertPixel(t, f.capture(t), 100, 100, color.RGBA{0, 0, 0, 255})
	f.step(1)
	f.iterate(t)
	sdltest.AssertPixel(t, f.capture(t), 100, 100, color.RGBA{255, 255, 255, 255})
}

func TestLoadFailureShowsHelpAndClearsState(t *testing.T) {
	f := newFixture(t)
	f.vm.ram[0x1000] = 9
	sdltest.OnMain(t, func() { sdl.PutAudioStreamBytes(f.probe.Stream, make([]byte, 64)) })
	missing := filepath.Join(t.TempDir(), "missing.bp")
	if f.vm.loadFile(missing) {
		t.Fatal("loading a missing file succeeded")
	}
	if !f.vm.displayHelp || f.vm.status != "load failed: missing.bp" {
		t.Fatalf("displayHelp=%v status=%q", f.vm.displayHelp, f.vm.status)
	}
	if f.vm.ram[0x1000] != 0 {
		t.Error("RAM was not cleared on failed load")
	}
	f.iterate(t)
	if !hasWhite(f.capture(t), helpArea) {
		t.Error("help text missing after failed load")
	}
}

func TestLoadClearsPendingAudio(t *testing.T) {
	f := newFixture(t)
	sdltest.OnMain(t, func() { sdl.PutAudioStreamBytes(f.probe.Stream, make([]byte, 100)) })
	if f.probe.Queued() != 100 {
		t.Fatalf("queued = %d", f.probe.Queued())
	}
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	if q := f.probe.Queued(); q != 0 {
		t.Errorf("queued after load = %d, want 0", q)
	}
}

func TestLoadTruncatesOversizedFile(t *testing.T) {
	f := newFixture(t)
	data := make([]byte, ramSize+100)
	data[ramSize-1] = 7
	data[ramSize+5] = 9
	if !f.vm.loadFile(writeProgram(t, "big.bp", data)) {
		t.Fatal("load of oversized file failed")
	}
	if f.vm.ram[ramSize-1] != 7 || f.vm.ram[ramSize+5] != 0 {
		t.Error("RAM must hold exactly the first 16 MiB; tail bytes must stay untouched")
	}
}

func TestDropFileEventLoadsProgram(t *testing.T) {
	f := newFixture(t)
	path := writeProgram(t, "dropped.bp", program())
	f.in.Drop(path)
	if f.feed(t) {
		t.Fatal("drop must not quit")
	}
	if f.vm.displayHelp || f.vm.status != "loaded dropped.bp" || f.vm.ram[0x200] != 0x42 {
		t.Errorf("displayHelp=%v status=%q ram[0x200]=%#x", f.vm.displayHelp, f.vm.status, f.vm.ram[0x200])
	}
}

func TestStatusIsTruncatedToThirtyOneBytes(t *testing.T) {
	f := newFixture(t)
	f.vm.setStatus("loaded %s", strings.Repeat("x", 100))
	if want := ("loaded " + strings.Repeat("x", 100))[:31]; f.vm.status != want {
		t.Errorf("status = %q, want %q", f.vm.status, want)
	}
}

func TestQuitPaths(t *testing.T) {
	f := newFixture(t)
	f.in.Quit()
	if !f.feed(t) {
		t.Error("EVENT_QUIT did not quit")
	}
	f.in.Press(sdl.SCANCODE_ESCAPE) // Escape quits on key down
	if !f.feed(t) {
		t.Error("Escape did not quit")
	}
	f.in.Press(sdl.SCANCODE_SPACE)
	if f.feed(t) {
		t.Error("an unrelated key quit")
	}
}

func (f *fixture) keyDown(t *testing.T, sc sdl.Scancode) {
	t.Helper()
	f.in.Key(true, sc, sdl.GetKeyFromScancode(sc, 0, false), 0)
	f.feed(t)
}

func (f *fixture) keyUp(t *testing.T, sc sdl.Scancode) {
	t.Helper()
	f.in.Key(false, sc, sdl.GetKeyFromScancode(sc, 0, false), 0)
	f.feed(t)
}

func TestSymbolicKeysSetAndClearBits(t *testing.T) {
	f := newFixture(t)
	f.keyDown(t, sdl.SCANCODE_A) // keycode 'a' => bit 0xa
	f.keyDown(t, sdl.SCANCODE_5)
	if want := uint16(1<<0xa | 1<<5); f.vm.keystate != want {
		t.Fatalf("keystate = %#x, want %#x", f.vm.keystate, want)
	}
	f.keyDown(t, sdl.SCANCODE_A) // key repeat is idempotent
	if f.vm.keystate != 1<<0xa|1<<5 {
		t.Fatalf("repeat changed keystate: %#x", f.vm.keystate)
	}
	f.keyUp(t, sdl.SCANCODE_A)
	if f.vm.keystate != 1<<5 {
		t.Fatalf("keystate after release = %#x", f.vm.keystate)
	}
	f.keyDown(t, sdl.SCANCODE_G) // outside 0-9/A-F
	if f.vm.keystate != 1<<5 {
		t.Fatalf("unmapped key changed keystate: %#x", f.vm.keystate)
	}
}

func TestEnterTogglesPositionalAndResetsKeys(t *testing.T) {
	f := newFixture(t)
	f.keyDown(t, sdl.SCANCODE_5)
	f.keyDown(t, sdl.SCANCODE_RETURN)
	if !f.vm.positional || f.vm.keystate != 0 || f.vm.status != "switched to positional input" || f.vm.statusTicks != 180 {
		t.Fatalf("positional=%v keystate=%#x status=%q", f.vm.positional, f.vm.keystate, f.vm.status)
	}
	// Positional: scancode Q is bit 4 regardless of keycode; '5' no longer maps.
	f.keyDown(t, sdl.SCANCODE_Q)
	f.keyDown(t, sdl.SCANCODE_5)
	if f.vm.keystate != 1<<4 {
		t.Fatalf("keystate = %#x, want %#x", f.vm.keystate, 1<<4)
	}
	f.keyUp(t, sdl.SCANCODE_Q)
	if f.vm.keystate != 0 {
		t.Fatalf("keystate after release = %#x", f.vm.keystate)
	}
	f.keyDown(t, sdl.SCANCODE_Q)
	f.keyDown(t, sdl.SCANCODE_RETURN)
	if f.vm.positional || f.vm.keystate != 0 || f.vm.status != "switched to symbolic input" {
		t.Fatalf("positional=%v keystate=%#x status=%q", f.vm.positional, f.vm.keystate, f.vm.status)
	}
}

func TestKeystateIsPublishedBigEndianToRAM(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	f.keyDown(t, sdl.SCANCODE_F) // bit 0xf: high byte 0x80
	f.keyDown(t, sdl.SCANCODE_1) // bit 1: low byte 0x02
	f.iterate(t)
	if f.vm.ram[0] != 0x80 || f.vm.ram[1] != 0x02 {
		t.Errorf("ram[0:2] = %#x %#x, want 0x80 0x02", f.vm.ram[0], f.vm.ram[1])
	}
}

func TestAudioBankQueuedPerTick(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	for i := range samplesPerFrame {
		f.vm.ram[0x200<<8+i] = uint8(i)
	}
	f.iterate(t)
	got := f.probe.ReadAll()
	if len(got) != samplesPerFrame {
		t.Fatalf("queued %d bytes, want %d", len(got), samplesPerFrame)
	}
	for i, b := range got {
		if b != uint8(i) {
			t.Fatalf("sample %d = %d, want %d", i, b, i)
		}
	}
}

func TestCatchUpRunsOneTickPerOwedFrame(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	f.iterate(t)
	f.probe.ReadAll()
	f.step(3)
	f.iterate(t)
	if q := f.probe.Queued(); q != 3*samplesPerFrame {
		t.Errorf("queued %d, want %d", q, 3*samplesPerFrame)
	}
}

func TestFallingTooFarBehindDropsStaleAudio(t *testing.T) {
	f := newFixture(t)
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	f.iterate(t)
	f.step(maxAudioLatencyFrames) // owes >= 5 frames: audio is skipped
	f.iterate(t)
	if q := f.probe.Queued(); q != samplesPerFrame {
		t.Errorf("queued %d, want only the final frame (%d)", q, samplesPerFrame)
	}
}

func TestNilAudioStreamIsTolerated(t *testing.T) {
	f := newFixture(t)
	f.vm.audioStream = nil
	f.vm.loadFile(writeProgram(t, "p.bp", program()))
	f.step(maxAudioLatencyFrames)
	f.iterate(t)
	if f.vm.ram[0x300] != 0x42 {
		t.Error("VM did not run without audio")
	}
}

func TestResizeKeepsIntegerScaledLogicalPresentation(t *testing.T) {
	f := newFixture(t)
	var w, h int32
	var mode sdl.RendererLogicalPresentation
	sdltest.OnMain(t, func() { sdl.GetRenderLogicalPresentation(f.vm.renderer, &w, &h, &mode) })
	if w != screenW || h != screenH || mode != sdl.LOGICAL_PRESENTATION_INTEGER_SCALE {
		t.Errorf("logical presentation = %dx%d mode %d", w, h, mode)
	}
	f.in.Resized(1024, 768)
	if f.feed(t) {
		t.Error("resize must not quit")
	}
}

func TestCloseReleasesResources(t *testing.T) {
	f := newFixture(t)
	f.vm.audioStream = nil
	sdltest.OnMain(t, f.vm.close)
	if f.vm.texture != nil || f.vm.palette != nil {
		t.Error("close left texture/palette set")
	}
	sdltest.OnMain(t, f.vm.close) // idempotent
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(3, "") })
	if err != nil {
		t.Fatal(err)
	}
	path := writeProgram(t, "p.bp", program())
	sdltest.OnMain(t, func() { err = run(3, path) })
	if err != nil {
		t.Fatal(err)
	}
}
