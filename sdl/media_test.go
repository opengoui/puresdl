package sdl

import (
	"math"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"unsafe"
)

// mediaInit initialises the video and audio subsystems on a locked thread and
// registers cleanup.
func mediaInit(t *testing.T, flags InitFlags) {
	t.Helper()
	runtime.LockOSThread()
	if !Init(flags) {
		runtime.UnlockOSThread()
		t.Fatalf("Init(%#x): %s", flags, GetError())
	}
	t.Cleanup(func() {
		Quit()
		runtime.UnlockOSThread()
	})
}

func mediaPixel(t *testing.T, s *Surface, x, y int32) [4]uint8 {
	t.Helper()
	var r, g, b, a byte
	if !ReadSurfacePixel(s, x, y, &r, &g, &b, &a) {
		t.Fatalf("ReadSurfacePixel(%d,%d): %s", x, y, GetError())
	}
	return [4]uint8{r, g, b, a}
}

func TestMediaDisplaysAndWindow(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	if got := GetCurrentVideoDriver(); got != "dummy" {
		t.Fatalf("video driver = %q", got)
	}
	ids := GetDisplays()
	if len(ids) < 1 {
		t.Fatalf("GetDisplays count = %d (%s)", len(ids), GetError())
	}
	for _, id := range ids {
		if id == 0 {
			t.Error("zero display id")
		}
	}
	if GetPrimaryDisplay() == 0 {
		t.Error("no primary display")
	}
	var rect Rect
	if !GetDisplayBounds(ids[0], &rect) || rect.W <= 0 || rect.H <= 0 {
		t.Errorf("GetDisplayBounds = %+v, %s", rect, GetError())
	}
	if GetDisplayName(ids[0]) == "" {
		t.Error("empty display name")
	}
	if m := GetCurrentDisplayMode(ids[0]); m == nil || m.W <= 0 || m.DisplayID != ids[0] {
		t.Errorf("GetCurrentDisplayMode = %+v", m)
	}
	// The dummy driver may expose no fullscreen modes; when it does they
	// are deep-copied out of the SDL allocation.
	if modes := GetFullscreenDisplayModes(ids[0]); len(modes) > 0 {
		if modes[0].W <= 0 || modes[0].H <= 0 {
			t.Errorf("mode[0] = %+v", modes[0])
		}
	}

	// Plain CreateWindow.
	w := CreateWindow("plain", 123, 77, WINDOW_HIDDEN)
	if w == nil {
		t.Fatal(GetError())
	}
	var ww, wh int32
	if !GetWindowSize(w, &ww, &wh) || ww != 123 || wh != 77 {
		t.Errorf("size = %dx%d", ww, wh)
	}
	if GetWindowFlags(w)&WINDOW_HIDDEN == 0 {
		t.Error("window not hidden")
	}
	SetWindowTitle(w, "renamed")
	if got := GetWindowTitle(w); got != "renamed" {
		t.Errorf("title = %q", got)
	}
	SetWindowSize(w, 200, 100)
	PumpEvents()
	GetWindowSize(w, &ww, &wh)
	if ww != 200 || wh != 100 {
		t.Errorf("resized = %dx%d", ww, wh)
	}
	if GetWindowFromID(GetWindowID(w)) != w {
		t.Error("GetWindowFromID mismatch")
	}
	DestroyWindow(w)

	// Window created from properties.
	props := CreateProperties()
	defer DestroyProperties(props)
	SetStringProperty(props, PROP_WINDOW_CREATE_TITLE_STRING, Ptr("props-title"))
	SetNumberProperty(props, PROP_WINDOW_CREATE_WIDTH_NUMBER, 320)
	SetNumberProperty(props, PROP_WINDOW_CREATE_HEIGHT_NUMBER, 240)
	SetBooleanProperty(props, PROP_WINDOW_CREATE_HIDDEN_BOOLEAN, true)
	SetBooleanProperty(props, PROP_WINDOW_CREATE_RESIZABLE_BOOLEAN, true)
	w = CreateWindowWithProperties(props)
	if w == nil {
		t.Fatal(GetError())
	}
	defer DestroyWindow(w)
	GetWindowSize(w, &ww, &wh)
	if ww != 320 || wh != 240 {
		t.Errorf("props size = %dx%d", ww, wh)
	}
	if GetWindowTitle(w) != "props-title" {
		t.Errorf("props title = %q", GetWindowTitle(w))
	}
	if f := GetWindowFlags(w); f&WINDOW_HIDDEN == 0 || f&WINDOW_RESIZABLE == 0 {
		t.Errorf("flags = %#x", uint64(f))
	}
	wp := GetWindowProperties(w)
	if wp == 0 {
		t.Error("no window properties")
	}
	if GetStringProperty(wp, "no.such.prop", "dflt") != "dflt" {
		t.Error("default string property not returned")
	}
	if GetNumberProperty(props, PROP_WINDOW_CREATE_WIDTH_NUMBER, -1) != 320 ||
		!GetBooleanProperty(props, PROP_WINDOW_CREATE_HIDDEN_BOOLEAN, false) {
		t.Error("property round trip failed")
	}
}

func TestMediaSoftwareRenderer(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	w := CreateWindow("render", 64, 48, WINDOW_HIDDEN)
	if w == nil {
		t.Fatal(GetError())
	}
	defer DestroyWindow(w)
	r := CreateRenderer(w, "software")
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)
	if got := GetRendererName(r); got != "software" {
		t.Errorf("renderer name = %q", got)
	}
	var found bool
	for i := int32(0); i < GetNumRenderDrivers(); i++ {
		if GetRenderDriver(i) == "software" {
			found = true
		}
	}
	if !found {
		t.Error("software not listed among render drivers")
	}
	var ow, oh int32
	if !GetRenderOutputSize(r, &ow, &oh) || ow != 64 || oh != 48 {
		t.Errorf("output size = %dx%d", ow, oh)
	}

	SetRenderDrawColor(r, 0, 0, 0, 255)
	RenderClear(r)
	SetRenderDrawColor(r, 255, 0, 0, 255)
	if !RenderFillRect(r, &FRect{X: 10, Y: 10, W: 10, H: 10}) {
		t.Fatal(GetError())
	}
	SetRenderDrawColor(r, 0, 255, 0, 255)
	RenderLine(r, 0, 40, 63, 40)
	SetRenderDrawColor(r, 0, 0, 255, 255)
	RenderPoint(r, 50, 5)
	SetRenderDrawColor(r, 255, 255, 0, 255)
	pts := []FPoint{{X: 30, Y: 5}, {X: 31, Y: 5}}
	RenderPoints(r, &pts[0], int32(len(pts)))
	SetRenderDrawColor(r, 0, 255, 255, 255)
	rects := []FRect{{X: 40, Y: 20, W: 2, H: 2}, {X: 44, Y: 20, W: 2, H: 2}}
	RenderFillRects(r, &rects[0], int32(len(rects)))

	if !RenderPointsSlice(r, nil) || !RenderLinesSlice(r, nil) || !RenderRectsSlice(r, []FRect{}) || !RenderFillRectsSlice(r, nil) {
		t.Error("empty slice helper should be a successful no-op")
	}
	SetRenderDrawColor(r, 255, 0, 255, 255)
	if !RenderFillRectsSlice(r, []FRect{{X: 55, Y: 30, W: 2, H: 2}}) {
		t.Fatal(GetError())
	}
	if !RenderPointsSlice(r, []FPoint{{X: 58, Y: 5}}) {
		t.Fatal(GetError())
	}

	surf := RenderReadPixels(r, nil)
	if surf == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(surf)
	if got := mediaPixel(t, surf, 55, 30); got != [4]uint8{255, 0, 255, 255} {
		t.Errorf("RenderFillRectsSlice pixel = %v", got)
	}
	if got := mediaPixel(t, surf, 58, 5); got != [4]uint8{255, 0, 255, 255} {
		t.Errorf("RenderPointsSlice pixel = %v", got)
	}
	if surf.W != 64 || surf.H != 48 || surf.Pixels == nil || surf.Pitch < 64*4 {
		t.Fatalf("surface = %dx%d pitch %d", surf.W, surf.H, surf.Pitch)
	}
	checks := []struct {
		x, y int32
		want [4]uint8
	}{
		{12, 12, [4]uint8{255, 0, 0, 255}},
		{9, 9, [4]uint8{0, 0, 0, 255}},
		{20, 20, [4]uint8{0, 0, 0, 255}},
		{30, 40, [4]uint8{0, 255, 0, 255}},
		{50, 5, [4]uint8{0, 0, 255, 255}},
		{30, 5, [4]uint8{255, 255, 0, 255}},
		{31, 5, [4]uint8{255, 255, 0, 255}},
		{41, 21, [4]uint8{0, 255, 255, 255}},
		{45, 21, [4]uint8{0, 255, 255, 255}},
		{43, 21, [4]uint8{0, 0, 0, 255}},
	}
	for _, c := range checks {
		if got := mediaPixel(t, surf, c.x, c.y); got != c.want {
			t.Errorf("pixel(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}

	// Viewport round trip and clip rect.
	vp := Rect{X: 5, Y: 6, W: 30, H: 20}
	if !SetRenderViewport(r, &vp) {
		t.Fatal(GetError())
	}
	var got Rect
	GetRenderViewport(r, &got)
	if got != vp {
		t.Errorf("viewport = %+v", got)
	}
	SetRenderViewport(r, nil)

	RenderPresent(r)
}

func TestMediaTexture(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(32, 32, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dst)
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)

	tex := CreateTexture(r, PIXELFORMAT_RGBA32, TEXTUREACCESS_STATIC, 4, 4)
	if tex == nil {
		t.Fatal(GetError())
	}
	defer DestroyTexture(tex)
	var tw, th float32
	if !GetTextureSize(tex, &tw, &th) || tw != 4 || th != 4 {
		t.Errorf("texture size = %vx%v", tw, th)
	}
	px := make([]byte, 4*4*4)
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = 200, 100, 50, 255
	}
	if !UpdateTexture(tex, nil, unsafe.Pointer(&px[0]), 16) {
		t.Fatal(GetError())
	}
	SetRenderDrawColor(r, 0, 0, 0, 255)
	RenderClear(r)
	if !RenderTexture(r, tex, nil, &FRect{X: 8, Y: 8, W: 8, H: 8}) {
		t.Fatal(GetError())
	}
	RenderPresent(r)
	out := RenderReadPixels(r, &Rect{X: 0, Y: 0, W: 32, H: 32})
	if out == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(out)
	if got, want := mediaPixel(t, out, 10, 10), [4]uint8{200, 100, 50, 255}; got != want {
		t.Errorf("textured pixel = %v, want %v", got, want)
	}
	if got, want := mediaPixel(t, out, 20, 20), [4]uint8{0, 0, 0, 255}; got != want {
		t.Errorf("outside pixel = %v, want %v", got, want)
	}
	// Colour modulation halves red.
	SetTextureColorMod(tex, 0, 0, 0)
	RenderClear(r)
	RenderTexture(r, tex, nil, &FRect{X: 8, Y: 8, W: 8, H: 8})
	out2 := RenderReadPixels(r, nil)
	defer DestroySurface(out2)
	if got := mediaPixel(t, out2, 10, 10); got[0] != 0 || got[1] != 0 || got[2] != 0 {
		t.Errorf("modulated pixel = %v", got)
	}

	// Geometry with a Vertex slice.
	SetRenderDrawColor(r, 0, 0, 0, 255)
	RenderClear(r)
	white := FColor{R: 1, G: 1, B: 1, A: 1}
	verts := []Vertex{
		{Position: FPoint{X: 0, Y: 0}, Color: white},
		{Position: FPoint{X: 32, Y: 0}, Color: white},
		{Position: FPoint{X: 0, Y: 32}, Color: white},
	}
	if !RenderGeometry(r, nil, &verts[0], 3, nil, 0) {
		t.Fatal(GetError())
	}
	out3 := RenderReadPixels(r, nil)
	defer DestroySurface(out3)
	if got := mediaPixel(t, out3, 2, 2); got != [4]uint8{255, 255, 255, 255} {
		t.Errorf("geometry pixel = %v", got)
	}
}

var mediaSentinel byte

func TestMediaEvents(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	w := CreateWindow("ev", 40, 40, WINDOW_HIDDEN)
	if w == nil {
		t.Fatal(GetError())
	}
	defer DestroyWindow(w)
	PumpEvents()
	FlushEvents(0, 0xFFFFFFFF)

	custom := EventType(RegisterEvents(1))
	if custom < EVENT_USER {
		t.Fatalf("RegisterEvents = %#x", custom)
	}
	var ev Event
	ev.SetType(custom)
	ev.User().Code = 42
	ev.User().Data1 = unsafe.Pointer(&mediaSentinel)
	if !PushEvent(&ev) {
		t.Fatal(GetError())
	}
	if !HasEvent(custom) {
		t.Error("HasEvent false after push")
	}
	var got Event
	if !PollEvent(&got) || got.Type() != custom || got.User().Code != 42 ||
		got.User().Data1 != unsafe.Pointer(&mediaSentinel) {
		t.Errorf("PollEvent = type %#x user %+v", got.Type(), got.User())
	}
	if PollEvent(&got) {
		t.Errorf("unexpected extra event %#x", got.Type())
	}

	// Key event round trip through the queue (bool fields).
	var kev Event
	k := kev.Key()
	k.Type = EVENT_KEY_DOWN
	k.Scancode = SCANCODE_A
	k.Down = true
	k.Repeat = true
	if !PushEvent(&kev) {
		t.Fatal(GetError())
	}
	var kout Event
	if !PollEvent(&kout) || kout.Key().Scancode != SCANCODE_A || !kout.Key().Down || !kout.Key().Repeat {
		t.Errorf("key event = %+v", kout.Key())
	}

	// WaitEventTimeout times out with an empty queue and delivers otherwise.
	if WaitEventTimeout(&got, 5) {
		t.Errorf("WaitEventTimeout returned event %#x", got.Type())
	}
	ev.SetType(custom)
	PushEvent(&ev)
	if !WaitEventTimeout(&got, 100) || got.Type() != custom {
		t.Error("WaitEventTimeout missed pushed event")
	}

	// Event watch callback with a struct pointer argument.
	var seen atomic.Int32
	var seenCode atomic.Int32
	watch := NewEventFilter(func(_ unsafe.Pointer, e *Event) bool {
		if e.Type() == custom {
			seen.Add(1)
			seenCode.Store(e.User().Code)
		}
		return true
	})
	if !AddEventWatch(watch, nil) {
		t.Fatal(GetError())
	}
	ev.User().Code = 7
	PushEvent(&ev)
	if seen.Load() != 1 || seenCode.Load() != 7 {
		t.Errorf("watch saw %d events, code %d", seen.Load(), seenCode.Load())
	}
	FlushEvents(0, 0xFFFFFFFF)
}

func TestMediaInputState(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	var nk int32
	state := GetKeyboardState(&nk)
	if state == nil || nk <= int32(SCANCODE_A) {
		t.Fatalf("GetKeyboardState n=%d", nk)
	}
	keys := unsafe.Slice(state, nk)
	for i, down := range keys {
		if down {
			t.Errorf("key %d reported down", i)
		}
	}
	if m := GetModState(); m != 0 {
		t.Errorf("mod state = %#x", uint16(m))
	}
	if got := GetKeyName(GetKeyFromScancode(SCANCODE_A, 0, false)); got != "A" {
		t.Errorf("key name = %q", got)
	}
	var x, y float32
	if b := GetMouseState(&x, &y); b != 0 {
		t.Errorf("mouse buttons = %#x", uint32(b))
	}
	_ = GetKeyboards()
	_ = GetMice()
	if GetKeyboardFocus() != nil || GetMouseFocus() != nil {
		t.Error("unexpected focus window")
	}
	_ = HasKeyboard()
}

func TestMediaEmptyDeviceLists(t *testing.T) {
	mediaInit(t, INIT_VIDEO|INIT_AUDIO|INIT_JOYSTICK|INIT_GAMEPAD|INIT_SENSOR|INIT_HAPTIC|INIT_CAMERA)
	// The host may have real controllers, sensors, haptics or cameras, so
	// check the shape of each list and that it agrees with the Has* queries.
	checkIDs := func(name string, ids []uint32) {
		seen := map[uint32]bool{}
		for _, id := range ids {
			if id == 0 || seen[id] {
				t.Errorf("%s: zero or duplicate instance ID in %v", name, ids)
			}
			seen[id] = true
		}
	}
	var js, gs, ss, hs, cs []uint32
	for _, id := range GetJoysticks() {
		js = append(js, uint32(id))
	}
	for _, id := range GetGamepads() {
		gs = append(gs, uint32(id))
	}
	for _, id := range GetSensors() {
		ss = append(ss, uint32(id))
	}
	for _, id := range GetHaptics() {
		hs = append(hs, uint32(id))
	}
	for _, id := range GetCameras() {
		cs = append(cs, uint32(id))
	}
	checkIDs("joysticks", js)
	checkIDs("gamepads", gs)
	checkIDs("sensors", ss)
	checkIDs("haptics", hs)
	checkIDs("cameras", cs)
	if HasJoystick() != (len(js) > 0) || HasGamepad() != (len(gs) > 0) {
		t.Errorf("HasJoystick/HasGamepad disagree with %d joysticks, %d gamepads", len(js), len(gs))
	}
	// SDL lists its virtual mouse and pen touch devices (IDs -1 and -2)
	// on some platforms; only real devices count.
	for _, id := range GetTouchDevices() {
		if id != TouchID(math.MaxUint64) && id != TouchID(math.MaxUint64-1) {
			t.Errorf("unexpected touch device %d", id)
		}
	}
}

func TestMediaClipboard(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	w := CreateWindow("clip", 10, 10, WINDOW_HIDDEN)
	if w == nil {
		t.Fatal(GetError())
	}
	defer DestroyWindow(w)
	if !SetClipboardText("héllo clipboard") {
		t.Fatal(GetError())
	}
	if !HasClipboardText() {
		t.Error("HasClipboardText false")
	}
	if got := GetClipboardText(); got != "héllo clipboard" {
		t.Errorf("clipboard = %q", got)
	}
	SetClipboardText("")
	if HasClipboardText() {
		t.Error("clipboard not cleared")
	}
	if GetClipboardText() != "" {
		t.Error("expected empty clipboard text")
	}
}

func TestMediaAudio(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	if got := GetNumAudioDrivers(); got < 1 {
		t.Fatalf("audio drivers = %d", got)
	}
	var found bool
	for i := int32(0); i < GetNumAudioDrivers(); i++ {
		if GetAudioDriver(i) == "dummy" {
			found = true
		}
	}
	if !found {
		t.Error("dummy audio driver not listed")
	}
	_ = GetAudioPlaybackDevices()

	var calls atomic.Int32
	cb := NewAudioStreamCallback(func(_ unsafe.Pointer, s *AudioStream, additional, total int32) {
		calls.Add(1)
		if additional > 0 {
			buf := make([]byte, additional)
			PutAudioStreamData(s, unsafe.Pointer(&buf[0]), additional)
		}
	})
	spec := AudioSpec{Format: AUDIO_S16, Channels: 2, Freq: 22050}
	stream := OpenAudioDeviceStream(AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, cb, nil)
	if stream == nil {
		t.Skipf("no dummy playback device: %s", GetError())
	}
	defer DestroyAudioStream(stream)
	var src, dst AudioSpec
	if !GetAudioStreamFormat(stream, &src, &dst) || src != spec {
		t.Errorf("stream src format = %+v, want %+v (%s)", src, spec, GetError())
	}
	if GetAudioStreamDevice(stream) == 0 {
		t.Error("stream has no device")
	}
	if !ResumeAudioStreamDevice(stream) {
		t.Fatal(GetError())
	}
	// Queue data and read it back; the dummy device may drain concurrently, so
	// only require that a bounded amount is available or already consumed.
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	if !PutAudioStreamData(stream, unsafe.Pointer(&data[0]), int32(len(data))) {
		t.Fatal(GetError())
	}
	if avail := GetAudioStreamAvailable(stream); avail < 0 || avail > int32(len(data))+1<<20 {
		t.Errorf("available = %d", avail)
	}
	// A standalone conversion stream is deterministic.
	conv := CreateAudioStream(&spec, &spec)
	if conv == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(conv)
	PutAudioStreamData(conv, unsafe.Pointer(&data[0]), int32(len(data)))
	FlushAudioStream(conv)
	if got := GetAudioStreamAvailable(conv); got != int32(len(data)) {
		t.Errorf("conv available = %d", got)
	}
	out := make([]byte, len(data))
	if got := GetAudioStreamData(conv, unsafe.Pointer(&out[0]), int32(len(out))); got != int32(len(out)) {
		t.Fatalf("read %d bytes", got)
	}
	for i := range out {
		if out[i] != data[i] {
			t.Fatalf("byte %d = %d, want %d", i, out[i], data[i])
		}
	}
}

func TestMediaPlanarSlices(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	spec := AudioSpec{Format: AUDIO_U8, Channels: 2, Freq: 22050}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)
	if !PutAudioStreamPlanarDataSlices(st, nil, 4) {
		t.Error("no planes should be a successful no-op")
	}
	if got := GetAudioStreamAvailable(st); got != 0 {
		t.Fatalf("available after no-op = %d", got)
	}
	left := []byte{1, 3, 5, 7}
	right := []byte{2, 4, 6, 8}
	if !PutAudioStreamPlanarDataSlices(st, [][]byte{left, right}, 4) {
		t.Fatal(GetError())
	}
	FlushAudioStream(st)
	out := make([]byte, 8)
	if got := GetAudioStreamData(st, unsafe.Pointer(&out[0]), 8); got != 8 {
		t.Fatalf("read %d bytes", got)
	}
	for i, b := range out {
		if b != byte(i+1) {
			t.Fatalf("interleaved = %v", out)
		}
	}
}

func TestMediaMisc(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	if GetPlatform() == "" || GetNumRenderDrivers() < 1 {
		t.Error("platform or render drivers missing")
	}
	for _, l := range GetPreferredLocales() {
		if l.Language == "" {
			t.Errorf("locale without language: %+v", l)
		}
	}
	if GetPixelFormatName(PIXELFORMAT_ARGB8888) != "SDL_PIXELFORMAT_ARGB8888" {
		t.Errorf("format name = %q", GetPixelFormatName(PIXELFORMAT_ARGB8888))
	}
	if d := GetPixelFormatDetails(PIXELFORMAT_ARGB8888); d == nil || d.BitsPerPixel != 32 || d.BytesPerPixel != 4 {
		t.Errorf("format details = %+v", d)
	}
	// Dialog and message box need real UI; only verify the symbols bind.
	for _, name := range []string{"SDL_ShowSimpleMessageBox", "SDL_ShowOpenFileDialog"} {
		if !Available(name) {
			t.Errorf("%s unavailable", name)
		}
	}
	if a := GetTicks(); a > GetTicks() {
		t.Error("GetTicks went backwards")
	}
}

// testWAV builds a minimal PCM WAV file (mono, 8-bit, 8000 Hz).
func testWAV(data []byte) []byte {
	le32 := func(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }
	var w []byte
	w = append(w, "RIFF"...)
	w = append(w, le32(uint32(36+len(data)))...)
	w = append(w, "WAVEfmt "...)
	w = append(w, le32(16)...)
	w = append(w, 1, 0, 1, 0) // PCM, mono
	w = append(w, le32(8000)...)
	w = append(w, le32(8000)...)
	w = append(w, 1, 0, 8, 0) // block align, bits
	w = append(w, "data"...)
	w = append(w, le32(uint32(len(data)))...)
	return append(w, data...)
}

func TestIOFromBytesRoundTrip(t *testing.T) {
	if IOFromBytes(nil) != nil || IOFromBytes([]byte{}) != nil {
		t.Error("empty input should give a nil stream")
	}
	src := []byte{1, 2, 3, 4, 5}
	io := IOFromBytes(src)
	if io == nil {
		t.Fatal(GetError())
	}
	defer CloseIO(io)
	src[0] = 99 // the stream owns a copy
	if got := ReadIOBytes(io, nil); got != 0 {
		t.Errorf("ReadIOBytes(empty) = %d", got)
	}
	dst := make([]byte, 8)
	if n := ReadIOBytes(io, dst); n != 5 {
		t.Fatalf("read %d bytes, want 5", n)
	}
	if want := []byte{1, 2, 3, 4, 5, 0, 0, 0}; !slices.Equal(dst, want) {
		t.Errorf("dst = %v, want %v", dst, want)
	}
}

func TestLoadWAVBytes(t *testing.T) {
	pcm := []byte{10, 20, 30, 40}
	spec, data, err := LoadWAVBytes(testWAV(pcm))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Channels != 1 || spec.Freq != 8000 || spec.Format != AUDIO_U8 {
		t.Errorf("spec = %+v", spec)
	}
	if !slices.Equal(data, pcm) {
		t.Errorf("data = %v, want %v", data, pcm)
	}
	if _, _, err := LoadWAVBytes([]byte("not a wav file at all")); err == nil {
		t.Error("garbage should fail")
	}
	if _, _, err := LoadWAVBytes(nil); err == nil {
		t.Error("empty input should fail")
	}
}

func TestPutAudioStreamBytes(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	spec := AudioSpec{Format: AUDIO_U8, Channels: 1, Freq: 8000}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)
	if !PutAudioStreamBytes(st, nil) || GetAudioStreamAvailable(st) != 0 {
		t.Error("empty slice should be a successful no-op")
	}
	if !PutAudioStreamBytes(st, []byte{1, 2, 3, 4}) {
		t.Fatal(GetError())
	}
	FlushAudioStream(st)
	if got := GetAudioStreamAvailable(st); got != 4 {
		t.Errorf("available = %d, want 4", got)
	}
}

func TestUpdateTextureBytes(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(8, 8, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dst)
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)
	tex := CreateTexture(r, PIXELFORMAT_RGBA32, TEXTUREACCESS_STATIC, 2, 2)
	if tex == nil {
		t.Fatal(GetError())
	}
	defer DestroyTexture(tex)
	if !UpdateTextureBytes(tex, nil, nil, 8) {
		t.Error("empty slice should be a successful no-op")
	}
	px := make([]byte, 16)
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = 10, 20, 30, 255
	}
	if !UpdateTextureBytes(tex, nil, px, 8) {
		t.Fatal(GetError())
	}
	RenderClear(r)
	RenderTexture(r, tex, nil, &FRect{W: 8, H: 8})
	out := RenderReadPixels(r, nil)
	if out == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(out)
	if got, want := mediaPixel(t, out, 4, 4), [4]uint8{10, 20, 30, 255}; got != want {
		t.Errorf("pixel = %v, want %v", got, want)
	}
}

// TestNoAudioStreamCallback opens a stream with the generated NULL callback
// constant, which SDL accepts as "no callback", and checks it is usable.
func TestNoAudioStreamCallback(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	var _ AudioStreamCallback = NoAudioStreamCallback
	spec := AudioSpec{Format: AUDIO_S16, Channels: 2, Freq: 22050}
	stream := OpenAudioDeviceStream(AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, NoAudioStreamCallback, nil)
	if stream == nil {
		t.Skipf("no dummy playback device: %s", GetError())
	}
	defer DestroyAudioStream(stream)
	data := make([]byte, 64)
	if !PutAudioStreamBytes(stream, data) {
		t.Fatal(GetError())
	}
}

// TestUpdateTextureBytesPlanar checks that a planar texture needs the chroma
// planes in the slice: a luma-only slice must be rejected before SDL reads
// past it.
func TestUpdateTextureBytesPlanar(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(8, 8, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dst)
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)
	for _, tc := range []struct {
		name   string
		format PixelFormat
		full   int // luma plane plus chroma planes for a 4x4 texture, pitch 4
	}{
		{"YV12", PIXELFORMAT_YV12, 16 + 2*2*2},
		{"IYUV", PIXELFORMAT_IYUV, 16 + 2*2*2},
		{"NV12", PIXELFORMAT_NV12, 16 + 2*4},
		{"NV21", PIXELFORMAT_NV21, 16 + 2*4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tex := CreateTexture(r, tc.format, TEXTUREACCESS_STATIC, 4, 4)
			if tex == nil {
				t.Skipf("planar texture unsupported: %s", GetError())
			}
			defer DestroyTexture(tex)
			for _, n := range []int{16, tc.full - 1} {
				ClearError()
				if UpdateTextureBytes(tex, nil, make([]byte, n), 4) {
					t.Errorf("%d bytes accepted, want rejection (need %d)", n, tc.full)
				}
				if GetError() == "" {
					t.Errorf("%d bytes: no SDL error set", n)
				}
			}
			if !UpdateTextureBytes(tex, nil, make([]byte, tc.full), 4) {
				t.Errorf("%d bytes rejected: %s", tc.full, GetError())
			}
		})
	}
}
