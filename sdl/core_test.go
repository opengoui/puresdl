package sdl

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// coreSkipUnless skips the test when any of the given symbols is absent from
// the installed SDL3.
func coreSkipUnless(t *testing.T, symbols ...string) {
	t.Helper()
	for _, s := range symbols {
		if !Available(s) {
			t.Skipf("%s is not exported by this SDL3 build", s)
		}
	}
}

// coreCStrings copies a NULL-terminated char** returned by SDL and frees it.
func coreCStrings(p **byte) []string {
	if p == nil {
		return nil
	}
	var out []string
	for i := 0; ; i++ {
		s := *(**byte)(unsafe.Add(unsafe.Pointer(p), i*int(unsafe.Sizeof(uintptr(0)))))
		if s == nil {
			break
		}
		out = append(out, GoString(s))
	}
	Free(unsafe.Pointer(p))
	return out
}

func coreSurface(t *testing.T, w, h int32, f PixelFormat) *Surface {
	t.Helper()
	s := CreateSurface(w, h, f)
	if s == nil {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { DestroySurface(s) })
	return s
}

func TestCoreSurfaceLayoutAndFill(t *testing.T) {
	s := coreSurface(t, 8, 4, PIXELFORMAT_ARGB8888)
	if s.W != 8 || s.H != 4 || s.Format != PIXELFORMAT_ARGB8888 {
		t.Fatalf("surface header = %dx%d format %#x", s.W, s.H, uint32(s.Format))
	}
	if s.Pitch < 8*4 || s.Pixels == nil {
		t.Fatalf("pitch=%d pixels=%v", s.Pitch, s.Pixels)
	}
	if s.Refcount != 1 {
		t.Errorf("Refcount = %d, want 1", s.Refcount)
	}
	if unsafe.Sizeof(Surface{}) != 48 {
		t.Errorf("sizeof(Surface) = %d, want 48", unsafe.Sizeof(Surface{}))
	}

	red := MapSurfaceRGBA(s, 255, 0, 0, 255)
	if red != 0xFFFF0000 {
		t.Errorf("MapSurfaceRGBA red = %#x", red)
	}
	if !FillSurfaceRect(s, nil, red) {
		t.Fatal(GetError())
	}
	r := Rect{2, 1, 3, 2}
	green := MapSurfaceRGB(s, 0, 255, 0)
	if !FillSurfaceRect(s, &r, green) {
		t.Fatal(GetError())
	}
	px := func(x, y int32) uint32 {
		return *(*uint32)(unsafe.Add(s.Pixels, uintptr(y)*uintptr(s.Pitch)+uintptr(x)*4))
	}
	if px(0, 0) != red || px(2, 1) != green || px(4, 2) != green || px(5, 2) != red || px(2, 3) != red {
		t.Errorf("fill result px = %#x %#x %#x %#x %#x", px(0, 0), px(2, 1), px(4, 2), px(5, 2), px(2, 3))
	}

	var cr, cg, cb, ca uint8
	if !ReadSurfacePixel(s, 3, 1, &cr, &cg, &cb, &ca) {
		t.Fatal(GetError())
	}
	if cr != 0 || cg != 255 || cb != 0 || ca != 255 {
		t.Errorf("ReadSurfacePixel = %d %d %d %d", cr, cg, cb, ca)
	}
	if !WriteSurfacePixel(s, 0, 0, 1, 2, 3, 4) {
		t.Fatal(GetError())
	}
	if got := px(0, 0); got != 0x04010203 {
		t.Errorf("WriteSurfacePixel stored %#x", got)
	}
	var fr, fg, fb, fa float32
	if !ReadSurfacePixelFloat(s, 3, 1, &fr, &fg, &fb, &fa) || fg != 1 || fr != 0 || fa != 1 {
		t.Errorf("ReadSurfacePixelFloat = %v %v %v %v", fr, fg, fb, fa)
	}
	if !ClearSurface(s, 0, 0, 1, 1) || px(7, 3) != 0xFF0000FF {
		t.Errorf("ClearSurface px = %#x", px(7, 3))
	}
}

func TestCoreSurfaceModsClipAndBlend(t *testing.T) {
	s := coreSurface(t, 4, 4, PIXELFORMAT_RGBA32)
	if !SetSurfaceColorMod(s, 10, 20, 30) || !SetSurfaceAlphaMod(s, 77) {
		t.Fatal(GetError())
	}
	var r, g, b, a uint8
	if !GetSurfaceColorMod(s, &r, &g, &b) || r != 10 || g != 20 || b != 30 {
		t.Errorf("color mod = %d %d %d", r, g, b)
	}
	if !GetSurfaceAlphaMod(s, &a) || a != 77 {
		t.Errorf("alpha mod = %d", a)
	}
	if !SetSurfaceBlendMode(s, BLENDMODE_ADD) {
		t.Fatal(GetError())
	}
	var bm BlendMode
	if !GetSurfaceBlendMode(s, &bm) || bm != BLENDMODE_ADD {
		t.Errorf("blend mode = %#x", uint32(bm))
	}
	clip := Rect{1, 1, 2, 2}
	if !SetSurfaceClipRect(s, &clip) {
		t.Fatal(GetError())
	}
	var got Rect
	if !GetSurfaceClipRect(s, &got) || got != clip {
		t.Errorf("clip = %+v", got)
	}
	if !SetSurfaceColorKey(s, true, 0xAB) {
		t.Fatal(GetError())
	}
	var key uint32
	if !SurfaceHasColorKey(s) || !GetSurfaceColorKey(s, &key) || key != 0xAB {
		t.Errorf("color key = %#x", key)
	}
	// Clearing with enabled=false removes the key.
	SetSurfaceColorKey(s, false, 0)
	if SurfaceHasColorKey(s) {
		t.Error("color key still set")
	}
}

func TestCoreSurfaceConvertBlitDuplicate(t *testing.T) {
	src := coreSurface(t, 4, 4, PIXELFORMAT_ARGB8888)
	FillSurfaceRect(src, nil, MapSurfaceRGBA(src, 10, 20, 30, 255))

	conv := ConvertSurface(src, PIXELFORMAT_RGB24)
	if conv == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(conv)
	if conv.Format != PIXELFORMAT_RGB24 {
		t.Fatalf("converted format = %#x", uint32(conv.Format))
	}
	var r, g, b, a uint8
	if !ReadSurfacePixel(conv, 1, 1, &r, &g, &b, &a) || r != 10 || g != 20 || b != 30 {
		t.Errorf("converted pixel = %d %d %d", r, g, b)
	}
	// RGB24 bytes are stored in memory order r,g,b.
	if p := unsafe.Slice((*byte)(conv.Pixels), 3); p[0] != 10 || p[1] != 20 || p[2] != 30 {
		t.Errorf("RGB24 memory = %v", p)
	}

	dup := DuplicateSurface(src)
	if dup == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dup)
	if dup.Pixels == src.Pixels || dup.W != 4 {
		t.Error("DuplicateSurface did not copy pixels")
	}

	dst := coreSurface(t, 8, 8, PIXELFORMAT_ARGB8888)
	dr := Rect{2, 2, 0, 0}
	if !BlitSurface(src, nil, dst, &dr) {
		t.Fatal(GetError())
	}
	if !ReadSurfacePixel(dst, 3, 3, &r, &g, &b, &a) || r != 10 || g != 20 || b != 30 {
		t.Errorf("blitted pixel = %d %d %d", r, g, b)
	}
	if !ReadSurfacePixel(dst, 0, 0, &r, &g, &b, &a) || r != 0 || a != 0 {
		t.Errorf("untouched pixel = %d %d %d %d", r, g, b, a)
	}

	scaled := ScaleSurface(src, 8, 6, SCALEMODE_NEAREST)
	if scaled == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(scaled)
	if scaled.W != 8 || scaled.H != 6 {
		t.Errorf("scaled = %dx%d", scaled.W, scaled.H)
	}
	if !FlipSurface(src, FLIP_HORIZONTAL) {
		t.Error(GetError())
	}
}

func TestCoreConvertPixels(t *testing.T) {
	// ARGB8888 0xAARRGGBB -> RGBA32 (bytes R,G,B,A in memory on LE).
	in := []uint32{0x80102030, 0xFF405060}
	out := make([]byte, 8)
	if !ConvertPixels(2, 1, PIXELFORMAT_ARGB8888, unsafe.Pointer(&in[0]), 8, PIXELFORMAT_RGBA32, unsafe.Pointer(&out[0]), 8) {
		t.Fatal(GetError())
	}
	if want := []byte{0x10, 0x20, 0x30, 0x80, 0x40, 0x50, 0x60, 0xFF}; !bytes.Equal(out, want) {
		t.Errorf("ConvertPixels = %x, want %x", out, want)
	}
}

func TestCoreCreateSurfaceFromAndBMPRoundTrip(t *testing.T) {
	pix := make([]uint32, 4*2)
	for i := range pix {
		pix[i] = 0xFF000000 | uint32(i)*0x010101
	}
	s := CreateSurfaceFrom(4, 2, PIXELFORMAT_ARGB8888, unsafe.Pointer(&pix[0]), 16)
	if s == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(s)
	if s.Pixels != unsafe.Pointer(&pix[0]) {
		t.Error("CreateSurfaceFrom did not wrap the supplied pixels")
	}
	path := filepath.Join(t.TempDir(), "x.bmp")
	if !SaveBMP(s, path) {
		t.Fatal(GetError())
	}
	back := LoadBMP(path)
	if back == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(back)
	if back.W != 4 || back.H != 2 {
		t.Fatalf("loaded %dx%d", back.W, back.H)
	}
	var r, g, b, a uint8
	if !ReadSurfacePixel(back, 3, 1, &r, &g, &b, &a) || r != 7 || g != 7 || b != 7 {
		t.Errorf("round-trip pixel = %d %d %d %d", r, g, b, a)
	}
	if LoadBMP(filepath.Join(t.TempDir(), "missing.bmp")) != nil || GetError() == "" {
		t.Error("loading a missing file should fail with an error message")
	}
}

func TestCorePixelFormatDetails(t *testing.T) {
	d := GetPixelFormatDetails(PIXELFORMAT_ARGB8888)
	if d == nil {
		t.Fatal(GetError())
	}
	want := PixelFormatDetails{
		Format: PIXELFORMAT_ARGB8888, BitsPerPixel: 32, BytesPerPixel: 4,
		Rmask: 0x00FF0000, Gmask: 0x0000FF00, Bmask: 0x000000FF, Amask: 0xFF000000,
		Rbits: 8, Gbits: 8, Bbits: 8, Abits: 8, Rshift: 16, Gshift: 8, Bshift: 0, Ashift: 24,
	}
	if *d != want {
		t.Errorf("details = %+v\nwant     %+v", *d, want)
	}
	if unsafe.Sizeof(PixelFormatDetails{}) != 32 {
		t.Errorf("sizeof(PixelFormatDetails) = %d, want 32", unsafe.Sizeof(PixelFormatDetails{}))
	}
	d565 := GetPixelFormatDetails(PIXELFORMAT_RGB565)
	if d565 == nil || d565.BitsPerPixel != 16 || d565.Rbits != 5 || d565.Gbits != 6 || d565.Bbits != 5 {
		t.Errorf("RGB565 details = %+v", d565)
	}

	if got := GetPixelFormatName(PIXELFORMAT_ARGB8888); got != "SDL_PIXELFORMAT_ARGB8888" {
		t.Errorf("name = %q", got)
	}
	var bpp int32
	var rm, gm, bm, am uint32
	if !GetMasksForPixelFormat(PIXELFORMAT_ABGR8888, &bpp, &rm, &gm, &bm, &am) {
		t.Fatal(GetError())
	}
	if bpp != 32 || rm != 0xFF || gm != 0xFF00 || bm != 0xFF0000 || am != 0xFF000000 {
		t.Errorf("masks = %d %#x %#x %#x %#x", bpp, rm, gm, bm, am)
	}
	if f := GetPixelFormatForMasks(bpp, rm, gm, bm, am); f != PIXELFORMAT_ABGR8888 {
		t.Errorf("format for masks = %#x", uint32(f))
	}

	var r, g, b, a uint8
	px := MapRGBA(d, nil, 0x11, 0x22, 0x33, 0x44)
	if px != 0x44112233 {
		t.Errorf("MapRGBA = %#x", px)
	}
	GetRGBA(px, d, nil, &r, &g, &b, &a)
	if r != 0x11 || g != 0x22 || b != 0x33 || a != 0x44 {
		t.Errorf("GetRGBA = %x %x %x %x", r, g, b, a)
	}
	if px := MapRGB(d, nil, 1, 2, 3); px != 0xFF010203 {
		t.Errorf("MapRGB = %#x", px)
	}
}

func TestCorePalette(t *testing.T) {
	pal := CreatePalette(4)
	if pal == nil {
		t.Fatal(GetError())
	}
	defer DestroyPalette(pal)
	if pal.Ncolors != 4 {
		t.Fatalf("Ncolors = %d", pal.Ncolors)
	}
	colors := []Color{{1, 2, 3, 255}, {4, 5, 6, 255}}
	if !SetPaletteColors(pal, &colors[0], 1, 2) {
		t.Fatal(GetError())
	}
	got := unsafe.Slice(pal.Colors, 4)
	if got[1] != colors[0] || got[2] != colors[1] {
		t.Errorf("palette colors = %+v", got)
	}

	s := coreSurface(t, 2, 2, PIXELFORMAT_INDEX8)
	sp := CreateSurfacePalette(s)
	if sp == nil {
		t.Fatal(GetError())
	}
	if GetSurfacePalette(s) != sp || sp.Ncolors != 256 {
		t.Errorf("surface palette ncolors = %d", sp.Ncolors)
	}
}

func TestCoreRects(t *testing.T) {
	a, b := Rect{0, 0, 10, 10}, Rect{5, 5, 10, 10}
	if !HasRectIntersection(&a, &b) {
		t.Error("expected intersection")
	}
	c := Rect{20, 20, 1, 1}
	if HasRectIntersection(&a, &c) {
		t.Error("unexpected intersection")
	}
	var out Rect
	if !GetRectUnion(&a, &b, &out) || out != (Rect{0, 0, 15, 15}) {
		t.Errorf("union = %+v", out)
	}
	pts := []Point{{1, 2}, {5, 9}, {3, 3}}
	if !GetRectEnclosingPoints(&pts[0], int32(len(pts)), nil, &out) || out != (Rect{1, 2, 5, 8}) {
		t.Errorf("enclosing = %+v", out)
	}
	clip := Rect{0, 0, 4, 4}
	if GetRectEnclosingPoints(&pts[0], int32(len(pts)), &clip, &out) && out != (Rect{1, 2, 3, 2}) {
		t.Errorf("clipped enclosing = %+v", out)
	}
	x1, y1, x2, y2 := int32(-5), int32(5), int32(15), int32(5)
	if !GetRectAndLineIntersection(&a, &x1, &y1, &x2, &y2) || x1 != 0 || x2 != 9 || y1 != 5 || y2 != 5 {
		t.Errorf("line clip = %d,%d -> %d,%d", x1, y1, x2, y2)
	}
	fa, fb := FRect{0, 0, 1, 1}, FRect{0.5, 0.5, 1, 1}
	var fo FRect
	if !GetRectIntersectionFloat(&fa, &fb, &fo) || fo != (FRect{0.5, 0.5, 0.5, 0.5}) {
		t.Errorf("float intersection = %+v", fo)
	}
	if !GetRectUnionFloat(&fa, &fb, &fo) || fo != (FRect{0, 0, 1.5, 1.5}) {
		t.Errorf("float union = %+v", fo)
	}
	fp := []FPoint{{0.5, 0.5}, {2.5, 3.5}}
	if !GetRectEnclosingPointsFloat(&fp[0], 2, nil, &fo) || fo.X != 0.5 || fo.Y != 0.5 || fo.W != 2 || fo.H != 3 {
		t.Errorf("float enclosing = %+v", fo)
	}
}

func TestCoreProperties(t *testing.T) {
	p := CreateProperties()
	if p == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(p)

	var target int
	cleaned := 0
	cleanup := NewCleanupPropertyCallback(func(userdata, value unsafe.Pointer) { cleaned++ })

	SetNumberProperty(p, "n", -1<<40)
	SetFloatProperty(p, "f", 1.5)
	SetBooleanProperty(p, "b", true)
	SetStringProperty(p, "s", Ptr("héllo"))
	if !SetPointerPropertyWithCleanup(p, "p", unsafe.Pointer(&target), cleanup, nil) {
		t.Fatal(GetError())
	}

	if got := GetNumberProperty(p, "n", 0); got != -1<<40 {
		t.Errorf("number = %d", got)
	}
	if got := GetFloatProperty(p, "f", 0); got != 1.5 {
		t.Errorf("float = %v", got)
	}
	if !GetBooleanProperty(p, "b", false) {
		t.Error("boolean = false")
	}
	if got := GetStringProperty(p, "s", ""); got != "héllo" {
		t.Errorf("string = %q", got)
	}
	if GetPointerProperty(p, "p", nil) != unsafe.Pointer(&target) {
		t.Error("pointer property mismatch")
	}
	for name, want := range map[string]PropertyType{
		"n": PROPERTY_TYPE_NUMBER, "f": PROPERTY_TYPE_FLOAT, "b": PROPERTY_TYPE_BOOLEAN,
		"s": PROPERTY_TYPE_STRING, "p": PROPERTY_TYPE_POINTER, "zz": PROPERTY_TYPE_INVALID,
	} {
		if got := GetPropertyType(p, name); got != want {
			t.Errorf("type of %q = %d, want %d", name, got, want)
		}
	}
	if !HasProperty(p, "n") || HasProperty(p, "zz") {
		t.Error("HasProperty misreports")
	}
	if got := GetNumberProperty(p, "zz", 99); got != 99 {
		t.Errorf("number default = %d", got)
	}
	if Available("SDL_GetNumProperties") {
		if got := GetNumProperties(p); got != 5 {
			t.Errorf("GetNumProperties = %d", got)
		}
	} else {
		t.Log("SDL_GetNumProperties absent from this SDL3; skipped")
	}

	var names []string
	enum := NewEnumeratePropertiesCallback(func(userdata unsafe.Pointer, props PropertiesID, name *byte) {
		names = append(names, GoString(name))
	})
	if !EnumerateProperties(p, enum, nil) {
		t.Fatal(GetError())
	}
	sort.Strings(names)
	if len(names) != 5 || names[0] != "b" || names[4] != "s" {
		t.Errorf("enumerated = %v", names)
	}

	// Copy and clear.
	q := CreateProperties()
	defer DestroyProperties(q)
	if !CopyProperties(p, q) || GetStringProperty(q, "s", "") != "héllo" {
		t.Error("CopyProperties lost the string")
	}
	if !ClearProperty(p, "p") || cleaned != 1 {
		t.Errorf("ClearProperty cleaned=%d", cleaned)
	}
	if !LockProperties(p) {
		t.Error(GetError())
	}
	UnlockProperties(p)

	if GetGlobalProperties() == 0 {
		t.Error("global properties is 0")
	}
}

func TestCoreIOStreamMemory(t *testing.T) {
	buf := make([]byte, 20)
	io := IOFromMem(unsafe.Pointer(&buf[0]), uint(len(buf)))
	if io == nil {
		t.Fatal(GetError())
	}
	if GetIOSize(io) != 20 {
		t.Errorf("size = %d", GetIOSize(io))
	}
	if !WriteU16LE(io, 0x1234) || !WriteU16BE(io, 0x1234) || !WriteU32LE(io, 0xA1B2C3D4) ||
		!WriteU32BE(io, 0xA1B2C3D4) || !WriteU64BE(io, 0x0102030405060708) {
		t.Fatal(GetError())
	}
	if want := []byte{0x34, 0x12, 0x12, 0x34, 0xD4, 0xC3, 0xB2, 0xA1, 0xA1, 0xB2, 0xC3, 0xD4, 1, 2, 3, 4, 5, 6, 7, 8}; !bytes.Equal(buf[:20], want) {
		t.Errorf("buf = %x, want %x", buf, want)
	}
	// Buffer is full; the next write must fail.
	if WriteU8(io, 1) {
		t.Error("write past the end succeeded")
	}
	if st := GetIOStatus(io); st != IO_STATUS_ERROR && st != IO_STATUS_READY {
		t.Errorf("status after full write = %d", st)
	}
	if got := SeekIO(io, 0, IO_SEEK_SET); got != 0 {
		t.Fatalf("seek = %d", got)
	}
	var u16 uint16
	var u32 uint32
	var u64 uint64
	if !ReadU16LE(io, &u16) || u16 != 0x1234 || !ReadU16BE(io, &u16) || u16 != 0x1234 {
		t.Errorf("u16 = %#x", u16)
	}
	if !ReadU32LE(io, &u32) || u32 != 0xA1B2C3D4 || !ReadU32BE(io, &u32) || u32 != 0xA1B2C3D4 {
		t.Errorf("u32 = %#x", u32)
	}
	if !ReadU64BE(io, &u64) || u64 != 0x0102030405060708 {
		t.Errorf("u64 = %#x", u64)
	}
	if TellIO(io) != 20 {
		t.Errorf("tell = %d", TellIO(io))
	}
	var u8 uint8
	if ReadU8(io, &u8) || GetIOStatus(io) != IO_STATUS_EOF {
		t.Errorf("read at EOF: status %d", GetIOStatus(io))
	}
	if SeekIO(io, -4, IO_SEEK_END) != 16 || SeekIO(io, 2, IO_SEEK_CUR) != 18 {
		t.Error("seek relative failed")
	}
	if !CloseIO(io) {
		t.Error(GetError())
	}
}

func TestCoreIOStreamSigned(t *testing.T) {
	buf := []byte{0xFF, 0xFE, 0xFF, 0x80, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	io := IOFromConstMem(unsafe.Pointer(&buf[0]), uint(len(buf)))
	if io == nil {
		t.Fatal(GetError())
	}
	defer CloseIO(io)
	var s8 int8
	if !ReadS8(io, &s8) || s8 != -1 {
		t.Errorf("ReadS8 = %d", s8)
	}
	var s16 int16
	if !ReadS16LE(io, &s16) || s16 != -2 {
		t.Errorf("ReadS16LE = %d", s16)
	}
	var s32 int32
	SeekIO(io, 0, IO_SEEK_SET)
	SeekIO(io, 3, IO_SEEK_SET)
	if !ReadS32LE(io, &s32) || s32 != 128 {
		t.Errorf("ReadS32LE = %d", s32)
	}
	var s64 int64
	if !ReadS64LE(io, &s64) && GetIOStatus(io) != IO_STATUS_EOF {
		t.Error(GetError())
	}
}

func TestCoreIOStreamDynamicAndFile(t *testing.T) {
	io := IOFromDynamicMem()
	if io == nil {
		t.Fatal(GetError())
	}
	msg := []byte("hello dynamic")
	if n := WriteIO(io, unsafe.Pointer(&msg[0]), uint(len(msg))); n != uint(len(msg)) {
		t.Fatalf("WriteIO = %d", n)
	}
	if n := IOprintf(io, " %d%%", 100); n != 5 {
		t.Errorf("IOprintf wrote %d, want 5", n)
	}
	props := GetIOProperties(io)
	mem := GetPointerProperty(props, PROP_IOSTREAM_DYNAMIC_MEMORY_POINTER, nil)
	if mem == nil {
		t.Fatal("no dynamic memory pointer")
	}
	if got := string(unsafe.Slice((*byte)(mem), 18)); got != "hello dynamic 100%" {
		t.Errorf("dynamic contents = %q", got)
	}
	if !FlushIO(io) {
		t.Error(GetError())
	}
	CloseIO(io)

	// File round trip with SaveFile/LoadFile.
	path := filepath.Join(t.TempDir(), "f.bin")
	data := []byte{9, 8, 7, 6, 5}
	if !SaveFile(path, unsafe.Pointer(&data[0]), uint(len(data))) {
		t.Fatal(GetError())
	}
	if onDisk, err := os.ReadFile(path); err != nil || !bytes.Equal(onDisk, data) {
		t.Errorf("on disk = %v %v", onDisk, err)
	}
	p := LoadFile(path)
	if len(p) != 5 {
		t.Fatalf("LoadFile = %v: %s", p, GetError())
	}
	if !bytes.Equal(p, data) {
		t.Error("LoadFile data mismatch")
	}

	f := IOFromFile(path, "rb")
	if f == nil {
		t.Fatal(GetError())
	}
	if GetIOSize(f) != 5 {
		t.Errorf("file size = %d", GetIOSize(f))
	}
	if fd := GetNumberProperty(GetIOProperties(f), PROP_IOSTREAM_FILE_DESCRIPTOR_NUMBER, -1); fd < 0 {
		t.Errorf("file descriptor property = %d", fd)
	}
	CloseIO(f)
	if IOFromFile(filepath.Join(t.TempDir(), "nope"), "rb") != nil {
		t.Error("opening a missing file succeeded")
	}
}

// coreMemStream is a Go-backed IOStream implementation.
type coreMemStream struct {
	data   []byte
	pos    int64
	closed bool
	reads  int
}

func TestCoreIOStreamCustomInterface(t *testing.T) {
	ms := &coreMemStream{data: []byte("0123456789")}
	iface := IOStreamInterface{
		Size: purego.NewCallback(func(ud unsafe.Pointer) int64 { return int64(len(ms.data)) }),
		Seek: purego.NewCallback(func(ud unsafe.Pointer, off int64, whence int32) int64 {
			switch IOWhence(whence) {
			case IO_SEEK_SET:
				ms.pos = off
			case IO_SEEK_CUR:
				ms.pos += off
			case IO_SEEK_END:
				ms.pos = int64(len(ms.data)) + off
			}
			return ms.pos
		}),
		Read: purego.NewCallback(func(ud, ptr unsafe.Pointer, size uintptr, status *IOStatus) uintptr {
			ms.reads++
			n := copy(unsafe.Slice((*byte)(ptr), size), ms.data[ms.pos:])
			ms.pos += int64(n)
			if uintptr(n) < size {
				*status = IO_STATUS_EOF
			}
			return uintptr(n)
		}),
		Write: purego.NewCallback(func(ud, ptr unsafe.Pointer, size uintptr, status *IOStatus) uintptr {
			ms.data = append(ms.data[:ms.pos], unsafe.Slice((*byte)(ptr), size)...)
			ms.pos = int64(len(ms.data))
			return size
		}),
		Flush: purego.NewCallback(func(ud unsafe.Pointer, status *IOStatus) uintptr { return 1 }),
		Close: purego.NewCallback(func(ud unsafe.Pointer) uintptr { ms.closed = true; return 1 }),
	}
	iface.Version = uint32(unsafe.Sizeof(iface))
	if iface.Version != 56 {
		t.Fatalf("sizeof(IOStreamInterface) = %d, want 56", iface.Version)
	}
	io := OpenIO(&iface, nil)
	if io == nil {
		t.Fatal(GetError())
	}
	if GetIOSize(io) != 10 {
		t.Errorf("size = %d", GetIOSize(io))
	}
	if SeekIO(io, 3, IO_SEEK_SET) != 3 {
		t.Error("seek failed")
	}
	buf := make([]byte, 4)
	if n := ReadIO(io, unsafe.Pointer(&buf[0]), 4); n != 4 || string(buf) != "3456" {
		t.Errorf("ReadIO = %d %q", n, buf)
	}
	if TellIO(io) != 7 {
		t.Errorf("tell = %d", TellIO(io))
	}
	big := make([]byte, 16)
	if n := ReadIO(io, unsafe.Pointer(&big[0]), 16); n != 3 || GetIOStatus(io) != IO_STATUS_EOF {
		t.Errorf("short read = %d status %d", n, GetIOStatus(io))
	}
	SeekIO(io, 0, IO_SEEK_END)
	extra := []byte("AB")
	if n := WriteIO(io, unsafe.Pointer(&extra[0]), 2); n != 2 || string(ms.data) != "0123456789AB" {
		t.Errorf("WriteIO = %d data %q", n, ms.data)
	}
	if !FlushIO(io) {
		t.Error("FlushIO failed")
	}
	if !CloseIO(io) || !ms.closed {
		t.Errorf("CloseIO closed=%v", ms.closed)
	}
}

func TestCoreFilesystemAndStorage(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if !CreateDirectory(sub) {
		t.Fatal(GetError())
	}
	file := filepath.Join(sub, "x.txt")
	if err := os.WriteFile(file, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	var info PathInfo
	if !GetPathInfo(file, &info) || info.Type != PATHTYPE_FILE || info.Size != 3 {
		t.Errorf("file info = %+v", info)
	}
	if info.ModifyTime == 0 {
		t.Error("ModifyTime = 0")
	}
	if !GetPathInfo(sub, &info) || info.Type != PATHTYPE_DIRECTORY {
		t.Errorf("dir info = %+v", info)
	}
	if GetPathInfo(filepath.Join(dir, "none"), &info) {
		t.Error("missing path reported as existing")
	}

	var seen []string
	cb := NewEnumerateDirectoryCallback(func(ud unsafe.Pointer, dirname, fname *byte) EnumerationResult {
		seen = append(seen, GoString(fname))
		if d := GoString(dirname); d == "" || d[len(d)-1] != '/' {
			t.Errorf("dirname %q does not end with a separator", d)
		}
		return ENUM_CONTINUE
	})
	if !EnumerateDirectory(sub, cb, nil) || len(seen) != 1 || seen[0] != "x.txt" {
		t.Errorf("enumerate = %v", seen)
	}
	// ENUM_FAILURE must make EnumerateDirectory return false.
	failCb := NewEnumerateDirectoryCallback(func(ud unsafe.Pointer, dirname, fname *byte) EnumerationResult { return ENUM_FAILURE })
	if EnumerateDirectory(sub, failCb, nil) {
		t.Error("ENUM_FAILURE did not propagate")
	}

	files := GlobDirectory(sub, "*.TXT", GLOB_CASEINSENSITIVE)
	if len(files) != 1 || files[0] != "x.txt" {
		t.Errorf("glob = %v", files)
	}

	copyTo := filepath.Join(dir, "copy.txt")
	if !CopyFile(file, copyTo) {
		t.Fatal(GetError())
	}
	renamed := filepath.Join(dir, "renamed.txt")
	if !RenamePath(copyTo, renamed) {
		t.Fatal(GetError())
	}
	if b, _ := os.ReadFile(renamed); string(b) != "abc" {
		t.Errorf("renamed contents = %q", b)
	}
	if !RemovePath(renamed) {
		t.Fatal(GetError())
	}
	if _, err := os.Stat(renamed); err == nil {
		t.Error("RemovePath left the file behind")
	}

	if got := GetCurrentDirectory(); got == "" {
		t.Errorf("GetCurrentDirectory empty: %s", GetError())
	}
	if GetBasePath() == "" {
		t.Errorf("GetBasePath empty: %s", GetError())
	}
	if got := GetUserFolder(FOLDER_HOME); got == "" {
		t.Logf("GetUserFolder(HOME) empty: %s", GetError())
	}

	// Storage over the temp dir.
	st := OpenFileStorage(dir)
	if st == nil {
		t.Fatal(GetError())
	}
	defer CloseStorage(st)
	if !StorageReady(st) {
		t.Fatal("storage not ready")
	}
	var size uint64
	if !GetStorageFileSize(st, "a/b/x.txt", &size) || size != 3 {
		t.Errorf("storage size = %d", size)
	}
	rd := make([]byte, 3)
	if !ReadStorageFile(st, "a/b/x.txt", unsafe.Pointer(&rd[0]), 3) || string(rd) != "abc" {
		t.Errorf("storage read = %q", rd)
	}
	w := []byte("written")
	if !WriteStorageFile(st, "w.txt", unsafe.Pointer(&w[0]), uint64(len(w))) {
		t.Fatal(GetError())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "w.txt")); string(b) != "written" {
		t.Errorf("storage write = %q", b)
	}
	if !CreateStorageDirectory(st, "newdir") {
		t.Error(GetError())
	}
	var pi PathInfo
	if !GetStoragePathInfo(st, "newdir", &pi) || pi.Type != PATHTYPE_DIRECTORY {
		t.Errorf("storage path info = %+v", pi)
	}
	if !CopyStorageFile(st, "w.txt", "w2.txt") || !RenameStoragePath(st, "w2.txt", "w3.txt") || !RemoveStoragePath(st, "w3.txt") {
		t.Error(GetError())
	}
	var names []string
	ecb := NewEnumerateDirectoryCallback(func(ud unsafe.Pointer, dirname, fname *byte) EnumerationResult {
		names = append(names, GoString(fname))
		return ENUM_CONTINUE
	})
	if !EnumerateStorageDirectory(st, "", ecb, nil) {
		t.Fatal(GetError())
	}
	sort.Strings(names)
	if len(names) != 3 || names[0] != "a" || names[1] != "newdir" || names[2] != "w.txt" {
		t.Errorf("storage enumerate = %v", names)
	}
	g := GlobStorageDirectory(st, "", "*.txt", 0)
	if len(g) != 1 || g[0] != "w.txt" {
		t.Errorf("storage glob = %v", g)
	}
	if GetStorageSpaceRemaining(st) == 0 {
		t.Log("GetStorageSpaceRemaining = 0")
	}
}

func TestCoreTimeAndTicks(t *testing.T) {
	a := GetTicksNS()
	DelayNS(2_000_000)
	b := GetTicksNS()
	if b-a < 2_000_000 {
		t.Errorf("DelayNS(2ms) elapsed %d ns", b-a)
	}
	if GetTicks() > GetTicksNS()/1_000_000+5 {
		t.Error("GetTicks inconsistent with GetTicksNS")
	}
	if GetPerformanceFrequency() == 0 || GetPerformanceCounter() == 0 {
		t.Error("performance counter is zero")
	}
	Delay(1)
	DelayPrecise(500_000)

	var tm Time
	if !GetCurrentTime(&tm) {
		t.Fatal(GetError())
	}
	if d := time.Since(time.Unix(0, int64(tm))); d < -time.Minute || d > time.Minute {
		t.Errorf("GetCurrentTime off by %v", d)
	}
	var dt DateTime
	if !TimeToDateTime(Time(1_700_000_000)*1_000_000_000, &dt, false) {
		t.Fatal(GetError())
	}
	// 2023-11-14 22:13:20 UTC, a Tuesday.
	want := DateTime{Year: 2023, Month: 11, Day: 14, Hour: 22, Minute: 13, Second: 20, DayOfWeek: 2}
	if dt != want {
		t.Errorf("DateTime = %+v, want %+v", dt, want)
	}
	var back Time
	if !DateTimeToTime(&dt, &back) || back != Time(1_700_000_000)*1_000_000_000 {
		t.Errorf("DateTimeToTime = %d", back)
	}
	if GetDaysInMonth(2024, 2) != 29 || GetDaysInMonth(2023, 2) != 28 {
		t.Error("GetDaysInMonth wrong")
	}
	if GetDayOfYear(2024, 3, 1) != 60 || GetDayOfWeek(2023, 11, 14) != 2 {
		t.Errorf("day of year/week = %d %d", GetDayOfYear(2024, 3, 1), GetDayOfWeek(2023, 11, 14))
	}
	var lo, hi uint32
	TimeToWindows(Time(1_700_000_000)*1_000_000_000, &lo, &hi)
	if TimeFromWindows(lo, hi) != Time(1_700_000_000)*1_000_000_000 {
		t.Error("Windows time round trip failed")
	}
	var df DateFormat
	var tf TimeFormat
	if !GetDateTimeLocalePreferences(&df, &tf) {
		t.Error(GetError())
	}
}

func TestCoreTimerCallbackOnForeignThread(t *testing.T) {
	var calls atomic.Int32
	var gotID atomic.Uint32
	var gotInterval atomic.Uint32
	done := make(chan struct{}, 8)
	cb := NewTimerCallback(func(ud unsafe.Pointer, id TimerID, interval uint32) uint32 {
		gotID.Store(uint32(id))
		gotInterval.Store(interval)
		if calls.Add(1) >= 3 {
			done <- struct{}{}
			return 0 // cancel
		}
		return interval
	})
	id := AddTimer(10, cb, nil)
	if id == 0 {
		t.Fatal(GetError())
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timer fired %d times", calls.Load())
	}
	if gotID.Load() != uint32(id) || gotInterval.Load() != 10 {
		t.Errorf("callback saw id=%d interval=%d, want %d/10", gotID.Load(), gotInterval.Load(), id)
	}
	time.Sleep(50 * time.Millisecond)
	if n := calls.Load(); n != 3 {
		t.Errorf("timer kept running after returning 0: %d calls", n)
	}
	if RemoveTimer(id) {
		t.Log("RemoveTimer succeeded on a self-cancelled timer")
	}
}

func TestCoreTimerNSAndRemove(t *testing.T) {
	var calls atomic.Int32
	cb := NewNSTimerCallback(func(ud unsafe.Pointer, id TimerID, interval uint64) uint64 {
		calls.Add(1)
		return interval
	})
	id := AddTimerNS(5_000_000, cb, nil)
	if id == 0 {
		t.Fatal(GetError())
	}
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !RemoveTimer(id) {
		t.Error(GetError())
	}
	// A callback already in flight when RemoveTimer returns may still finish,
	// so take the baseline after a settling delay.
	time.Sleep(40 * time.Millisecond)
	n := calls.Load()
	time.Sleep(40 * time.Millisecond)
	if n < 2 || calls.Load() != n {
		t.Errorf("calls before=%d after remove=%d", n, calls.Load())
	}
}

func TestCoreThreads(t *testing.T) {
	var ran atomic.Int32
	var tid atomic.Uint64
	fn := NewThreadFunction(func(data unsafe.Pointer) int32 {
		ran.Add(1)
		tid.Store(uint64(GetCurrentThreadID()))
		return 42
	})
	th := CreateThreadRuntime(fn, "core-worker", nil, 0, 0)
	if th == nil {
		t.Fatal(GetError())
	}
	if GetThreadName(th) != "core-worker" {
		t.Errorf("thread name = %q", GetThreadName(th))
	}
	// SDL publishes the thread ID from the new thread, so read it only after
	// the worker has started.
	for deadline := time.Now().Add(5 * time.Second); tid.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	id := GetThreadID(th)
	var status int32
	WaitThread(th, &status)
	if status != 42 || ran.Load() != 1 {
		t.Errorf("status=%d ran=%d", status, ran.Load())
	}
	if uint64(id) != tid.Load() || id == GetCurrentThreadID() {
		t.Errorf("thread id %d vs inside %d vs main %d", id, tid.Load(), GetCurrentThreadID())
	}
}

func TestCoreMutexAtomicSemaphoreAcrossThreads(t *testing.T) {
	mu := CreateMutex()
	if mu == nil {
		t.Fatal(GetError())
	}
	defer DestroyMutex(mu)
	var counter int
	var atom AtomicInt
	var spin SpinLock
	sem := CreateSemaphore(0)
	defer DestroySemaphore(sem)

	const workers, iters = 4, 200
	fn := NewThreadFunction(func(unsafe.Pointer) int32 {
		for i := 0; i < iters; i++ {
			LockMutex(mu)
			counter++
			UnlockMutex(mu)
			AddAtomicInt(&atom, 1)
			LockSpinlock(&spin)
			UnlockSpinlock(&spin)
		}
		SignalSemaphore(sem)
		return 0
	})
	threads := make([]*Thread, workers)
	for i := range threads {
		if threads[i] = CreateThreadRuntime(fn, "core-w", nil, 0, 0); threads[i] == nil {
			t.Fatal(GetError())
		}
	}
	for range threads {
		if !WaitSemaphoreTimeout(sem, 5000) {
			t.Fatal("worker did not signal")
		}
	}
	for _, th := range threads {
		WaitThread(th, nil)
	}
	if counter != workers*iters || GetAtomicInt(&atom) != workers*iters {
		t.Errorf("counter=%d atomic=%d", counter, GetAtomicInt(&atom))
	}
	if TryWaitSemaphore(sem) || GetSemaphoreValue(sem) != 0 {
		t.Error("semaphore should be drained")
	}
}

func TestCoreSyncPrimitives(t *testing.T) {
	mu := CreateMutex()
	defer DestroyMutex(mu)
	if !TryLockMutex(mu) {
		t.Fatal("TryLockMutex failed")
	}
	UnlockMutex(mu)

	rw := CreateRWLock()
	if rw == nil {
		t.Fatal(GetError())
	}
	defer DestroyRWLock(rw)
	LockRWLockForReading(rw)
	if !TryLockRWLockForReading(rw) {
		t.Error("second reader refused")
	}
	UnlockRWLock(rw)
	UnlockRWLock(rw)
	LockRWLockForWriting(rw)
	UnlockRWLock(rw)

	cond := CreateCondition()
	defer DestroyCondition(cond)
	LockMutex(mu)
	start := time.Now()
	if WaitConditionTimeout(cond, mu, 20) {
		t.Error("condition reported signalled without a signal")
	}
	UnlockMutex(mu)
	if time.Since(start) < 15*time.Millisecond {
		t.Error("WaitConditionTimeout returned early")
	}
	SignalCondition(cond)
	BroadcastCondition(cond)

	sem := CreateSemaphore(2)
	defer DestroySemaphore(sem)
	if GetSemaphoreValue(sem) != 2 || !TryWaitSemaphore(sem) || GetSemaphoreValue(sem) != 1 {
		t.Error("semaphore counting wrong")
	}

	var is InitState
	if !ShouldInit(&is) {
		t.Fatal("ShouldInit false the first time")
	}
	SetInitialized(&is, true)
	if ShouldInit(&is) {
		t.Error("ShouldInit true after SetInitialized")
	}
	if !ShouldQuit(&is) {
		t.Error("ShouldQuit false when initialized")
	}
	SetInitialized(&is, false)
	if unsafe.Sizeof(is) != 24 {
		t.Errorf("sizeof(InitState) = %d, want 24", unsafe.Sizeof(is))
	}
}

func TestCoreAtomics(t *testing.T) {
	var a AtomicInt
	if SetAtomicInt(&a, 5) != 0 || GetAtomicInt(&a) != 5 {
		t.Error("SetAtomicInt")
	}
	if AddAtomicInt(&a, 3) != 5 || GetAtomicInt(&a) != 8 {
		t.Error("AddAtomicInt should return the previous value")
	}
	if !CompareAndSwapAtomicInt(&a, 8, 1) || CompareAndSwapAtomicInt(&a, 8, 2) || a.Value != 1 {
		t.Error("CompareAndSwapAtomicInt")
	}
	var u AtomicU32
	SetAtomicU32(&u, 0xFFFFFFFF)
	if GetAtomicU32(&u) != 0xFFFFFFFF || AddAtomicU32(&u, 1) != 0xFFFFFFFF || GetAtomicU32(&u) != 0 {
		t.Error("AtomicU32 wraparound")
	}
	if !CompareAndSwapAtomicU32(&u, 0, 9) || u.Value != 9 {
		t.Error("CompareAndSwapAtomicU32")
	}
	x, y := 1, 2
	var p unsafe.Pointer = unsafe.Pointer(&x)
	if GetAtomicPointer(&p) != unsafe.Pointer(&x) {
		t.Error("GetAtomicPointer")
	}
	if !CompareAndSwapAtomicPointer(&p, unsafe.Pointer(&x), unsafe.Pointer(&y)) || p != unsafe.Pointer(&y) {
		t.Error("CompareAndSwapAtomicPointer")
	}
	if SetAtomicPointer(&p, unsafe.Pointer(&x)) != unsafe.Pointer(&y) {
		t.Error("SetAtomicPointer should return the old value")
	}
	var sl SpinLock
	LockSpinlock(&sl)
	if TryLockSpinlock(&sl) {
		t.Error("spinlock locked twice")
	}
	UnlockSpinlock(&sl)
	if !TryLockSpinlock(&sl) {
		t.Error("spinlock not released")
	}
	UnlockSpinlock(&sl)
	MemoryBarrierReleaseFunction()
	MemoryBarrierAcquireFunction()
}

func TestCoreTLS(t *testing.T) {
	var id TLSID
	destroyed := make(chan unsafe.Pointer, 1)
	dtor := NewTLSDestructorCallback(func(v unsafe.Pointer) { destroyed <- v })
	val := new(int)
	fn := NewThreadFunction(func(unsafe.Pointer) int32 {
		if GetTLS(&id) != nil {
			return 1
		}
		if !SetTLS(&id, unsafe.Pointer(val), dtor) || GetTLS(&id) != unsafe.Pointer(val) {
			return 2
		}
		CleanupTLS()
		return 0
	})
	th := CreateThreadRuntime(fn, "core-tls", nil, 0, 0)
	if th == nil {
		t.Fatal(GetError())
	}
	var st int32
	WaitThread(th, &st)
	if st != 0 {
		t.Fatalf("tls thread status %d", st)
	}
	select {
	case v := <-destroyed:
		if v != unsafe.Pointer(val) {
			t.Error("destructor got the wrong value")
		}
	case <-time.After(2 * time.Second):
		t.Error("TLS destructor never ran")
	}
	if GetTLS(&id) != nil {
		t.Error("TLS leaked into another thread")
	}
}

func TestCoreEnvironment(t *testing.T) {
	env := CreateEnvironment(false)
	if env == nil {
		t.Fatal(GetError())
	}
	defer DestroyEnvironment(env)
	if GetEnvironmentVariable(env, "CORE_X") != "" {
		t.Error("empty environment has a variable")
	}
	if !SetEnvironmentVariable(env, "CORE_X", "one", true) || !SetEnvironmentVariable(env, "CORE_Y", "two", true) {
		t.Fatal(GetError())
	}
	if SetEnvironmentVariable(env, "CORE_X", "other", false); GetEnvironmentVariable(env, "CORE_X") != "one" {
		t.Error("overwrite=false replaced the value")
	}
	SetEnvironmentVariable(env, "CORE_X", "uno", true)
	if GetEnvironmentVariable(env, "CORE_X") != "uno" {
		t.Error("overwrite=true did not replace")
	}
	vars := GetEnvironmentVariables(env)
	sort.Strings(vars)
	if len(vars) != 2 || vars[0] != "CORE_X=uno" || vars[1] != "CORE_Y=two" {
		t.Errorf("variables = %v", vars)
	}
	if !UnsetEnvironmentVariable(env, "CORE_Y") || GetEnvironmentVariable(env, "CORE_Y") != "" {
		t.Error("unset failed")
	}

	t.Setenv("CORE_PROC", "val")
	if got := GetEnvironmentVariable(GetEnvironment(), "CORE_PROC"); got != "val" {
		// SDL snapshots the process environment on first use, so a later
		// Go-side Setenv may legitimately be invisible; set it through SDL.
		SetEnvironmentVariable(GetEnvironment(), "CORE_PROC", "val", true)
		if GetEnvironmentVariable(GetEnvironment(), "CORE_PROC") != "val" {
			t.Errorf("process environment lost value: %q", got)
		}
	}
	if SetenvUnsafe("CORE_UNSAFE", "u", 1) != 0 || GetenvUnsafe("CORE_UNSAFE") != "u" {
		t.Error("SetenvUnsafe/GetenvUnsafe")
	}
	if UnsetenvUnsafe("CORE_UNSAFE") != 0 || GetenvUnsafe("CORE_UNSAFE") != "" {
		t.Error("UnsetenvUnsafe")
	}
}

func TestCoreHints(t *testing.T) {
	const name = "SDL_CORE_TEST_HINT"
	if GetHint(name) != "" {
		t.Fatal("hint unexpectedly set")
	}
	if !GetHintBoolean(name, true) || GetHintBoolean(name, false) {
		t.Error("GetHintBoolean should return the default when unset")
	}
	var events []string
	cb := NewHintCallback(func(ud unsafe.Pointer, n, oldV, newV *byte) {
		events = append(events, GoString(n)+":"+GoString(oldV)+"->"+GoString(newV))
	})
	if !AddHintCallback(name, cb, nil) {
		t.Fatal(GetError())
	}
	if !SetHint(name, "1") || GetHint(name) != "1" || !GetHintBoolean(name, false) {
		t.Error("SetHint/GetHint")
	}
	// A lower priority must not override.
	if SetHintWithPriority(name, "0", HINT_DEFAULT) {
		t.Error("default priority overrode a normal-priority hint")
	}
	if !SetHintWithPriority(name, "2", HINT_OVERRIDE) || GetHint(name) != "2" {
		t.Error("override priority failed")
	}
	RemoveHintCallback(name, cb, nil)
	if !ResetHint(name) || GetHint(name) != "" {
		t.Error("ResetHint")
	}
	want := []string{name + ":->", name + ":->1", name + ":1->2"}
	if len(events) != len(want) {
		t.Fatalf("events = %q", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %q, want %q", i, events[i], want[i])
		}
	}
	if GetHint(HINT_APP_NAME) != "" {
		t.Logf("HINT_APP_NAME preset: %q", GetHint(HINT_APP_NAME))
	}
}

func TestCoreErrors(t *testing.T) {
	ClearError()
	if GetError() != "" {
		t.Errorf("GetError after ClearError = %q", GetError())
	}
	if OutOfMemory() {
		t.Error("OutOfMemory returned true")
	}
	if GetError() != "Out of memory" {
		t.Errorf("OOM message = %q", GetError())
	}
	// Errors are thread-local: another thread must not see ours.
	SetError("main-thread error")
	seen := make(chan string, 1)
	th := CreateThreadRuntime(NewThreadFunction(func(unsafe.Pointer) int32 {
		seen <- GetError()
		return 0
	}), "core-err", nil, 0, 0)
	WaitThread(th, nil)
	if s := <-seen; s != "" {
		t.Errorf("error leaked across threads: %q", s)
	}
	if GetError() != "main-thread error" {
		t.Errorf("error = %q", GetError())
	}
	SetError("unicode: 日本語 %s", "é")
	if GetError() != "unicode: 日本語 é" {
		t.Errorf("unicode error = %q", GetError())
	}
	ClearError()
}

func TestCoreMemory(t *testing.T) {
	p := Malloc(64)
	if p == nil {
		t.Fatal("Malloc nil")
	}
	b := unsafe.Slice((*byte)(p), 64)
	for i := range b {
		b[i] = byte(i)
	}
	p = Realloc(p, 4096)
	b = unsafe.Slice((*byte)(p), 4096)
	for i := 0; i < 64; i++ {
		if b[i] != byte(i) {
			t.Fatalf("Realloc lost byte %d", i)
		}
	}
	Free(p)

	c := Calloc(8, 8)
	for i, v := range unsafe.Slice((*byte)(c), 64) {
		if v != 0 {
			t.Fatalf("Calloc byte %d = %d", i, v)
		}
	}
	Free(c)

	a := AlignedAlloc(64, 100)
	if a == nil || uintptr(a)%64 != 0 {
		t.Errorf("AlignedAlloc = %p", a)
	}
	AlignedFree(a)
	Free(nil)

	var m MallocFunc
	var ca CallocFunc
	var re ReallocFunc
	var fr FreeFunc
	GetMemoryFunctions(&m, &ca, &re, &fr)
	if m == 0 || ca == 0 || re == 0 || fr == 0 {
		t.Errorf("memory functions = %#x %#x %#x %#x", m, ca, re, fr)
	}
	GetOriginalMemoryFunctions(&m, &ca, &re, &fr)
	if m == 0 {
		t.Error("original malloc is nil")
	}
	_ = GetNumAllocations()
}

func TestCoreHashAndRandom(t *testing.T) {
	data := []byte("123456789")
	if got := Crc32(0, unsafe.Pointer(&data[0]), uint(len(data))); got != 0xCBF43926 {
		t.Errorf("Crc32 = %#x, want 0xCBF43926", got)
	}
	if got := Crc16(0, unsafe.Pointer(&data[0]), uint(len(data))); got != 0xBB3D {
		t.Errorf("Crc16 = %#x", got)
	}
	if got := Murmur3_32(unsafe.Pointer(&data[0]), uint(len(data)), 0); got != 0xB4FEF382 {
		t.Errorf("Murmur3_32 = %#x", got)
	}
	if a, b := Murmur3_32(unsafe.Pointer(&data[0]), 9, 1), Murmur3_32(unsafe.Pointer(&data[0]), 9, 1); a != b {
		t.Error("Murmur3_32 not deterministic")
	}

	s1, s2 := uint64(1234), uint64(1234)
	for i := 0; i < 16; i++ {
		x, y := RandR(&s1, 100), RandR(&s2, 100)
		if x != y || x < 0 || x >= 100 {
			t.Fatalf("RandR = %d / %d", x, y)
		}
		if f := RandfR(&s1); f < 0 || f >= 1 {
			t.Fatalf("RandfR = %v", f)
		}
		RandfR(&s2)
		if RandBitsR(&s1) != RandBitsR(&s2) {
			t.Fatal("RandBitsR diverged")
		}
	}
	if s1 == 1234 {
		t.Error("RandR did not advance state through the pointer")
	}
	Srand(42)
	a := []int32{Rand(1000), Rand(1000), Rand(1000)}
	Srand(42)
	b := []int32{Rand(1000), Rand(1000), Rand(1000)}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("Srand(42) not reproducible: %v vs %v", a, b)
		}
	}
	if f := Randf(); f < 0 || f >= 1 {
		t.Errorf("Randf = %v", f)
	}
	_ = RandBits()
}

func TestCoreIconv(t *testing.T) {
	// inbytesleft includes the terminating NUL; the result is a C string
	// copied and freed by the wrapper.
	if got := IconvString("ISO-8859-1", "UTF-8", "\u00e9a", 4); got != "\xe9a" {
		t.Errorf("IconvString = %q", got)
	}
}

func TestCoreSystemInfo(t *testing.T) {
	if n := GetNumLogicalCPUCores(); n < 1 {
		t.Errorf("cores = %d", n)
	}
	if n := GetCPUCacheLineSize(); n < 16 || n > 256 {
		t.Errorf("cache line = %d", n)
	}
	if n := GetSystemRAM(); n < 64 {
		t.Errorf("RAM = %d MiB", n)
	}
	if n := GetSystemPageSize(); n != 4096 && n != 16384 {
		t.Errorf("page size = %d", n)
	}
	if n := GetSIMDAlignment(); n < 8 || n&(n-1) != 0 {
		t.Errorf("SIMD alignment = %d", n)
	}
	// Architecture sanity: exactly one family is reported.
	hasARM := HasNEON()
	hasX86 := HasSSE2()
	if hasARM == hasX86 {
		t.Errorf("NEON=%v SSE2=%v", hasARM, hasX86)
	}
	_ = []bool{HasAltiVec(), HasMMX(), HasSSE(), HasSSE3(), HasSSE41(), HasSSE42(), HasAVX(), HasAVX2(), HasAVX512F(), HasARMSIMD(), HasLSX(), HasLASX()}
	coreSkipUnless(t, "SDL_HasSVE2")
	_ = HasSVE2()
}

func TestCoreLocalePowerMisc(t *testing.T) {
	if locs := GetPreferredLocales(); len(locs) > 0 {
		if locs[0].Language == "" {
			t.Errorf("locales first=%+v", locs[0])
		}
		if unsafe.Sizeof(Locale{}) != 16 {
			t.Errorf("sizeof(Locale) = %d", unsafe.Sizeof(Locale{}))
		}
	} else {
		t.Logf("no preferred locales: %s", GetError())
	}

	var secs, pct int32
	st := GetPowerInfo(&secs, &pct)
	if st < POWERSTATE_ERROR || st > POWERSTATE_CHARGED {
		t.Errorf("power state = %d", st)
	}
	if pct < -1 || pct > 100 || secs < -1 {
		t.Errorf("power secs=%d pct=%d", secs, pct)
	}
}

func TestCoreVersionAndPlatform(t *testing.T) {
	if GetVersion() < 3_000_000 {
		t.Errorf("version = %d", GetVersion())
	}
	if p := GetPlatform(); p != "macOS" && p != "Linux" && p != "Windows" {
		t.Logf("platform = %q", p)
	}
}

func TestCoreSharedCallbackMutation(t *testing.T) {
	// Several goroutines hammer a pure-Go callback invoked synchronously by SDL
	// (property enumeration) to make sure purego callbacks are reentrant.
	p := CreateProperties()
	defer DestroyProperties(p)
	for _, k := range []string{"a", "b", "c"} {
		SetNumberProperty(p, k, 1)
	}
	var total atomic.Int32
	cb := NewEnumeratePropertiesCallback(func(ud unsafe.Pointer, props PropertiesID, name *byte) {
		total.Add(int32(GetNumberProperty(props, GoString(name), 0)))
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				EnumerateProperties(p, cb, nil)
			}
		}()
	}
	wg.Wait()
	if total.Load() != 8*50*3 {
		t.Errorf("total = %d, want %d", total.Load(), 8*50*3)
	}
}
