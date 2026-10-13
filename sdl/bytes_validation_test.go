package sdl

import (
	"errors"
	"math"
	"strings"
	"testing"
	"unsafe"
)

func TestFitsInt32(t *testing.T) {
	ClearError()
	if !fitsInt32(math.MaxInt32, "x") || !fitsInt32(0, "x") {
		t.Fatal("in-range lengths must fit")
	}
	if GetError() != "" {
		t.Fatalf("unexpected error %q", GetError())
	}
	if fitsInt32(math.MaxInt32+1, "myFunc") {
		t.Fatal("MaxInt32+1 must not fit")
	}
	if got := GetError(); !strings.Contains(got, "myFunc") {
		t.Fatalf("error = %q, want it to name the function", got)
	}
}

func TestPutAudioStreamBytesTooLong(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	if testing.Short() {
		t.Skip("allocates 2 GiB of address space")
	}
	spec := AudioSpec{Format: AUDIO_U8, Channels: 1, Freq: 8000}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)
	big := make([]byte, math.MaxInt32+1) // untouched pages stay virtual
	ClearError()
	if PutAudioStreamBytes(st, big) {
		t.Fatal("2 GiB slice must be rejected")
	}
	if GetError() == "" {
		t.Error("no SDL error set")
	}
	if got := GetAudioStreamAvailable(st); got != 0 {
		t.Errorf("available = %d, want 0", got)
	}
}

func TestPutAudioStreamPlanarDataSlicesValidation(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	// 16-bit samples: a plane of numSamples bytes is too short.
	spec := AudioSpec{Format: AUDIO_S16, Channels: 2, Freq: 22050}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)

	short := make([]byte, 4)
	full := make([]byte, 8)
	for _, tc := range []struct {
		name   string
		planes [][]byte
		n      int32
	}{
		{"short plane", [][]byte{full, short}, 4},
		{"short plane after nil", [][]byte{nil, short}, 4},
		{"negative samples", [][]byte{full, full}, -1},
	} {
		ClearError()
		if PutAudioStreamPlanarDataSlices(st, tc.planes, tc.n) {
			t.Errorf("%s: want false", tc.name)
		}
		if GetError() == "" {
			t.Errorf("%s: no SDL error set", tc.name)
		}
		if got := GetAudioStreamQueued(st); got != 0 {
			t.Errorf("%s: queued = %d, want 0 (nothing handed to SDL)", tc.name, got)
		}
	}
	if PutAudioStreamPlanarDataSlices(nil, [][]byte{full}, 1) {
		t.Error("nil stream must fail")
	}

	if !PutAudioStreamPlanarDataSlices(st, [][]byte{full, full}, 4) {
		t.Fatal(GetError())
	}
	FlushAudioStream(st)
	if got := GetAudioStreamAvailable(st); got != 16 {
		t.Errorf("available = %d, want 16", got)
	}
	// A plane longer than needed is fine.
	if !PutAudioStreamPlanarDataSlices(st, [][]byte{full, full}, 2) {
		t.Fatal(GetError())
	}
}

func TestPutAudioStreamPlanarDataSlicesNilPlaneIsSilence(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	spec := AudioSpec{Format: AUDIO_F32, Channels: 2, Freq: 22050}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)

	const n = 5
	left := make([]byte, n*4)
	for i := range left {
		left[i] = 0x11
	}
	for _, planes := range [][][]byte{{left, nil}, {nil, left}, {left, {}}} {
		if !PutAudioStreamPlanarDataSlices(st, planes, n) {
			t.Fatal(GetError())
		}
	}
	FlushAudioStream(st)
	// 3 calls x n frames x 2 channels x 4 bytes.
	if got := GetAudioStreamQueued(st); got != 3*n*2*4 {
		t.Errorf("queued = %d, want %d", got, 3*n*2*4)
	}
	out := make([]byte, n*2*4)
	if got := GetAudioStreamData(st, unsafe.Pointer(&out[0]), int32(len(out))); got != int32(len(out)) {
		t.Fatalf("read %d bytes", got)
	}
	for f := 0; f < n; f++ {
		l, r := out[f*8:f*8+4], out[f*8+4:f*8+8]
		for k := 0; k < 4; k++ {
			if l[k] != 0x11 || r[k] != 0 {
				t.Fatalf("frame %d: left %v right %v, want left data and silent right", f, l, r)
			}
		}
	}
}

func TestUpdateTextureBytesValidation(t *testing.T) {
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

	// Whole texture: 4 rows at pitch 16, last row 4*4 bytes: need 16*3+16.
	const whole = 64
	rect := &Rect{X: 1, Y: 1, W: 2, H: 3} // need 16*2 + 2*4 = 40
	for _, tc := range []struct {
		name  string
		rect  *Rect
		n     int
		pitch int32
		ok    bool
	}{
		{"whole exact", nil, whole, 16, true},
		{"whole short by one", nil, whole - 1, 16, false},
		{"rect exact", rect, 40, 16, true},
		{"rect short by one", rect, 39, 16, false},
		{"zero pitch", nil, whole, 0, false},
		{"negative pitch", nil, whole, -16, false},
		{"negative rect", &Rect{W: -1, H: 2}, whole, 16, false},
		{"last row narrower than pitch", &Rect{W: 1, H: 2}, 16 + 4, 16, true},
	} {
		ClearError()
		got := UpdateTextureBytes(tex, tc.rect, make([]byte, tc.n), tc.pitch)
		if got != tc.ok {
			t.Errorf("%s: got %v, want %v (%s)", tc.name, got, tc.ok, GetError())
		}
		if !tc.ok && GetError() == "" {
			t.Errorf("%s: no SDL error set", tc.name)
		}
	}
	if UpdateTextureBytes(nil, nil, make([]byte, whole), 16) {
		t.Error("nil texture must fail")
	}
	if !UpdateTextureBytes(tex, nil, nil, 0) {
		t.Error("empty pixels remain a successful no-op")
	}
}

func TestLoadWAVBytesEmptyIsFixedError(t *testing.T) {
	SetError("stale error from elsewhere")
	for _, in := range [][]byte{nil, {}} {
		_, _, err := LoadWAVBytes(in)
		if !errors.Is(err, errLoadWAVEmpty) {
			t.Fatalf("err = %v, want errLoadWAVEmpty", err)
		}
		if strings.Contains(err.Error(), "stale") || !strings.Contains(err.Error(), "empty data") {
			t.Fatalf("err = %q", err)
		}
	}
}

func TestIOFromBytesWritable(t *testing.T) {
	src := []byte{1, 2, 3}
	io := IOFromBytes(src)
	if io == nil {
		t.Fatal(GetError())
	}
	defer CloseIO(io)
	if SeekIO(io, 0, IO_SEEK_END) != 3 {
		t.Fatalf("seek end: %s", GetError())
	}
	extra := []byte{9}
	if n := WriteIO(io, unsafe.Pointer(&extra[0]), 1); n != 1 {
		t.Fatalf("write = %d (%s)", n, GetError())
	}
	if !slicesEqual(src, []byte{1, 2, 3}) {
		t.Fatalf("source modified: %v", src)
	}
	if got := SeekIO(io, 0, IO_SEEK_END); got != 4 {
		t.Fatalf("size after write = %d, want 4", got)
	}
}

func slicesEqual(a, b []byte) bool { return string(a) == string(b) }

func planarTexture(t *testing.T, format PixelFormat, w, h int32) *Texture {
	t.Helper()
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(16, 16, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { DestroySurface(dst) })
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { DestroyRenderer(r) })
	tex := CreateTexture(r, format, TEXTUREACCESS_STATIC, w, h)
	if tex == nil {
		t.Skipf("format %#x unsupported: %s", format, GetError())
	}
	t.Cleanup(func() { DestroyTexture(tex) })
	return tex
}

func TestUpdateYUVTextureBytesValidation(t *testing.T) {
	tex := planarTexture(t, PIXELFORMAT_IYUV, 5, 3) // chroma planes are 3x2
	y, u := make([]byte, 5*3), make([]byte, 3*2)
	for _, tc := range []struct {
		name       string
		y, u, v    []byte
		pitch      int32
		upitch     int32
		rect       *Rect
		wantResult bool
	}{
		{"exact", y, u, u, 5, 3, nil, true},
		{"short luma", y[:14], u, u, 5, 3, nil, false},
		{"short u", y, u[:5], u, 5, 3, nil, false},
		{"short v", y, u, u[:5], 5, 3, nil, false},
		{"zero pitch", y, u, u, 0, 3, nil, false},
		{"negative chroma pitch", y, u, u, 5, -3, nil, false},
		{"negative rect", y, u, u, 5, 3, &Rect{W: -2, H: 2}, false},
		{"empty luma", nil, u, u, 5, 3, nil, false},
	} {
		ClearError()
		got := UpdateYUVTextureBytes(tex, tc.rect, tc.y, tc.pitch, tc.u, tc.upitch, tc.v, tc.upitch)
		if got != tc.wantResult {
			t.Errorf("%s: got %v, want %v (%s)", tc.name, got, tc.wantResult, GetError())
		}
		if !tc.wantResult && GetError() == "" {
			t.Errorf("%s: no SDL error set", tc.name)
		}
	}
	if UpdateYUVTextureBytes(nil, nil, y, 5, u, 3, u, 3) {
		t.Error("nil texture must fail")
	}
	if !UpdateYUVTextureBytes(tex, nil, nil, 0, nil, 0, nil, 0) {
		t.Error("all-empty planes remain a successful no-op")
	}
	rgb := planarTexture(t, PIXELFORMAT_RGBA32, 4, 4)
	ClearError()
	if UpdateYUVTextureBytes(rgb, nil, y, 5, u, 3, u, 3) || GetError() == "" {
		t.Error("a non-YUV texture must be rejected with an error")
	}
}

func TestUpdateNVTextureBytesValidation(t *testing.T) {
	tex := planarTexture(t, PIXELFORMAT_NV12, 5, 3) // UV plane: 3 pairs x 2 rows
	y, uv := make([]byte, 5*3), make([]byte, 6*2)
	for _, tc := range []struct {
		name string
		y    []byte
		uv   []byte
		ok   bool
	}{
		{"exact", y, uv, true},
		{"short luma", y[:14], uv, false},
		{"short uv", y, uv[:11], false},
	} {
		ClearError()
		got := UpdateNVTextureBytes(tex, nil, tc.y, 5, tc.uv, 6)
		if got != tc.ok {
			t.Errorf("%s: got %v, want %v (%s)", tc.name, got, tc.ok, GetError())
		}
		if !tc.ok && GetError() == "" {
			t.Errorf("%s: no SDL error set", tc.name)
		}
	}
	if UpdateNVTextureBytes(tex, nil, y, 0, uv, 6) || UpdateNVTextureBytes(nil, nil, y, 5, uv, 6) {
		t.Error("zero pitch and nil texture must fail")
	}
}
