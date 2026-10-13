package sdl

// The expected values in this file were not computed by this package. They
// were produced by compiling and running small C programs with the system
// clang against the headers of the release pinned in SDL_VERSION (3.4.18, the
// same headers the bindings are generated from); the macros and static inline
// functions need no link step:
//
//	clang -I $SDL/include -Wno-everything -o gen gen.c   (only SDL_fabsf, a real
//	                                                      export, is stubbed)
//
// Each C program evaluated the SDL_* macro or inline function on the test
// inputs below and printed the result as Go source, which was pasted here.
// Byte-order macros were evaluated on little-endian arm64.

import (
	"fmt"
	"math"
	"math/bits"
	"os"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestMacroPixelFormats(t *testing.T) {
	tests := []struct {
		name                                                 string
		format                                               PixelFormat
		flag, typ, order, layout, bits, bytes                uint32
		indexed, packed, array, tenBit, float, alpha, fourcc bool
	}{
		{"UNKNOWN", PIXELFORMAT_UNKNOWN, 0, 0, 0, 0, 0, 0, false, false, false, false, false, false, false},
		{"INDEX1LSB", PIXELFORMAT_INDEX1LSB, 1, 1, 1, 0, 1, 0, true, false, false, false, false, false, false},
		{"INDEX2MSB", PIXELFORMAT_INDEX2MSB, 1, 12, 2, 0, 2, 0, true, false, false, false, false, false, false},
		{"INDEX4LSB", PIXELFORMAT_INDEX4LSB, 1, 2, 1, 0, 4, 0, true, false, false, false, false, false, false},
		{"INDEX8", PIXELFORMAT_INDEX8, 1, 3, 0, 0, 8, 1, true, false, false, false, false, false, false},
		{"RGB332", PIXELFORMAT_RGB332, 1, 4, 1, 1, 8, 1, false, true, false, false, false, false, false},
		{"XRGB4444", PIXELFORMAT_XRGB4444, 1, 5, 1, 2, 12, 2, false, true, false, false, false, false, false},
		{"RGB565", PIXELFORMAT_RGB565, 1, 5, 1, 5, 16, 2, false, true, false, false, false, false, false},
		{"ARGB1555", PIXELFORMAT_ARGB1555, 1, 5, 3, 3, 16, 2, false, true, false, false, false, true, false},
		{"ARGB8888", PIXELFORMAT_ARGB8888, 1, 6, 3, 6, 32, 4, false, true, false, false, false, true, false},
		{"RGBA8888", PIXELFORMAT_RGBA8888, 1, 6, 4, 6, 32, 4, false, true, false, false, false, true, false},
		{"XBGR8888", PIXELFORMAT_XBGR8888, 1, 6, 5, 6, 24, 4, false, true, false, false, false, false, false},
		{"ABGR2101010", PIXELFORMAT_ABGR2101010, 1, 6, 7, 7, 32, 4, false, true, false, true, false, true, false},
		{"ARGB2101010", PIXELFORMAT_ARGB2101010, 1, 6, 3, 7, 32, 4, false, true, false, true, false, true, false},
		{"XRGB2101010", PIXELFORMAT_XRGB2101010, 1, 6, 1, 7, 32, 4, false, true, false, true, false, false, false},
		{"RGB24", PIXELFORMAT_RGB24, 1, 7, 1, 0, 24, 3, false, false, true, false, false, false, false},
		{"BGR24", PIXELFORMAT_BGR24, 1, 7, 4, 0, 24, 3, false, false, true, false, false, false, false},
		{"RGBA32", PIXELFORMAT_RGBA32, 1, 6, 7, 6, 32, 4, false, true, false, false, false, true, false},
		{"BGRA32", PIXELFORMAT_BGRA32, 1, 6, 3, 6, 32, 4, false, true, false, false, false, true, false},
		{"RGB48", PIXELFORMAT_RGB48, 1, 8, 1, 0, 48, 6, false, false, true, false, false, false, false},
		{"RGBA64", PIXELFORMAT_RGBA64, 1, 8, 2, 0, 64, 8, false, false, true, false, false, true, false},
		{"RGBA64_FLOAT", PIXELFORMAT_RGBA64_FLOAT, 1, 10, 2, 0, 64, 8, false, false, true, false, true, true, false},
		{"RGBA128_FLOAT", PIXELFORMAT_RGBA128_FLOAT, 1, 11, 2, 0, 128, 16, false, false, true, false, true, true, false},
		{"RGB96_FLOAT", PIXELFORMAT_RGB96_FLOAT, 1, 11, 1, 0, 96, 12, false, false, true, false, true, false, false},
		{"YV12", PIXELFORMAT_YV12, 3, 2, 3, 1, 0, 1, false, false, false, false, false, false, true},
		{"IYUV", PIXELFORMAT_IYUV, 5, 6, 5, 5, 0, 1, false, false, false, false, false, false, true},
		{"YUY2", PIXELFORMAT_YUY2, 3, 2, 5, 9, 0, 2, false, false, false, false, false, false, true},
		{"UYVY", PIXELFORMAT_UYVY, 5, 9, 5, 6, 0, 2, false, false, false, false, false, false, true},
		{"YVYU", PIXELFORMAT_YVYU, 5, 5, 5, 9, 0, 2, false, false, false, false, false, false, true},
		{"NV12", PIXELFORMAT_NV12, 3, 2, 3, 1, 0, 1, false, false, false, false, false, false, true},
		{"NV21", PIXELFORMAT_NV21, 3, 1, 3, 2, 0, 1, false, false, false, false, false, false, true},
		{"P010", PIXELFORMAT_P010, 3, 0, 3, 1, 0, 2, false, false, false, false, false, false, true},
		{"EXTERNAL_OES", PIXELFORMAT_EXTERNAL_OES, 2, 0, 5, 3, 0, 1, false, false, false, false, false, false, true},
		{"MJPG", PIXELFORMAT_MJPG, 4, 7, 5, 0, 0, 1, false, false, false, false, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.format
			if got := PIXELFLAG(f); got != tc.flag {
				t.Errorf("PIXELFLAG = %d, want %d", got, tc.flag)
			}
			if got := uint32(PIXELTYPE(f)); got != tc.typ {
				t.Errorf("PIXELTYPE = %d, want %d", got, tc.typ)
			}
			if got := PIXELORDER(f); got != tc.order {
				t.Errorf("PIXELORDER = %d, want %d", got, tc.order)
			}
			if got := PIXELLAYOUT(f); got != tc.layout {
				t.Errorf("PIXELLAYOUT = %d, want %d", got, tc.layout)
			}
			if got := BITSPERPIXEL(f); got != tc.bits {
				t.Errorf("BITSPERPIXEL = %d, want %d", got, tc.bits)
			}
			if got := BYTESPERPIXEL(f); got != tc.bytes {
				t.Errorf("BYTESPERPIXEL = %d, want %d", got, tc.bytes)
			}
			for _, b := range []struct {
				name      string
				got, want bool
			}{
				{"INDEXED", ISPIXELFORMAT_INDEXED(f), tc.indexed},
				{"PACKED", ISPIXELFORMAT_PACKED(f), tc.packed},
				{"ARRAY", ISPIXELFORMAT_ARRAY(f), tc.array},
				{"10BIT", ISPIXELFORMAT_10BIT(f), tc.tenBit},
				{"FLOAT", ISPIXELFORMAT_FLOAT(f), tc.float},
				{"ALPHA", ISPIXELFORMAT_ALPHA(f), tc.alpha},
				{"FOURCC", ISPIXELFORMAT_FOURCC(f), tc.fourcc},
			} {
				if b.got != b.want {
					t.Errorf("ISPIXELFORMAT_%s = %v, want %v", b.name, b.got, b.want)
				}
			}
		})
	}
}

func TestMacroColorspaces(t *testing.T) {
	tests := []struct {
		name                                          string
		cs                                            Colorspace
		typ, rng, chroma, primaries, transfer, matrix uint32
		bt601, bt709, bt2020, limited, full           bool
	}{
		{"UNKNOWN", COLORSPACE_UNKNOWN, 0, 0, 0, 0, 0, 0, false, false, false, true, false},
		{"SRGB", COLORSPACE_SRGB, 1, 2, 0, 1, 13, 0, false, false, false, false, true},
		{"SRGB_LINEAR", COLORSPACE_SRGB_LINEAR, 1, 2, 0, 1, 8, 0, false, false, false, false, true},
		{"HDR10", COLORSPACE_HDR10, 1, 2, 0, 9, 16, 0, false, false, false, false, true},
		{"JPEG", COLORSPACE_JPEG, 2, 2, 0, 1, 6, 6, true, false, false, false, true},
		{"BT601_LIMITED", COLORSPACE_BT601_LIMITED, 2, 1, 1, 6, 6, 6, true, false, false, true, false},
		{"BT601_FULL", COLORSPACE_BT601_FULL, 2, 2, 1, 6, 6, 6, true, false, false, false, true},
		{"BT709_LIMITED", COLORSPACE_BT709_LIMITED, 2, 1, 1, 1, 1, 1, false, true, false, true, false},
		{"BT709_FULL", COLORSPACE_BT709_FULL, 2, 2, 1, 1, 1, 1, false, true, false, false, true},
		{"BT2020_LIMITED", COLORSPACE_BT2020_LIMITED, 2, 1, 1, 9, 16, 9, false, false, true, true, false},
		{"BT2020_FULL", COLORSPACE_BT2020_FULL, 2, 2, 1, 9, 16, 9, false, false, true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.cs
			if got := uint32(COLORSPACETYPE(c)); got != tc.typ {
				t.Errorf("COLORSPACETYPE = %d, want %d", got, tc.typ)
			}
			if got := uint32(COLORSPACERANGE(c)); got != tc.rng {
				t.Errorf("COLORSPACERANGE = %d, want %d", got, tc.rng)
			}
			if got := uint32(COLORSPACECHROMA(c)); got != tc.chroma {
				t.Errorf("COLORSPACECHROMA = %d, want %d", got, tc.chroma)
			}
			if got := uint32(COLORSPACEPRIMARIES(c)); got != tc.primaries {
				t.Errorf("COLORSPACEPRIMARIES = %d, want %d", got, tc.primaries)
			}
			if got := uint32(COLORSPACETRANSFER(c)); got != tc.transfer {
				t.Errorf("COLORSPACETRANSFER = %d, want %d", got, tc.transfer)
			}
			if got := uint32(COLORSPACEMATRIX(c)); got != tc.matrix {
				t.Errorf("COLORSPACEMATRIX = %d, want %d", got, tc.matrix)
			}
			if got := ISCOLORSPACE_MATRIX_BT601(c); got != tc.bt601 {
				t.Errorf("ISCOLORSPACE_MATRIX_BT601 = %v, want %v", got, tc.bt601)
			}
			if got := ISCOLORSPACE_MATRIX_BT709(c); got != tc.bt709 {
				t.Errorf("ISCOLORSPACE_MATRIX_BT709 = %v, want %v", got, tc.bt709)
			}
			if got := ISCOLORSPACE_MATRIX_BT2020_NCL(c); got != tc.bt2020 {
				t.Errorf("ISCOLORSPACE_MATRIX_BT2020_NCL = %v, want %v", got, tc.bt2020)
			}
			if got := ISCOLORSPACE_LIMITED_RANGE(c); got != tc.limited {
				t.Errorf("ISCOLORSPACE_LIMITED_RANGE = %v, want %v", got, tc.limited)
			}
			if got := ISCOLORSPACE_FULL_RANGE(c); got != tc.full {
				t.Errorf("ISCOLORSPACE_FULL_RANGE = %v, want %v", got, tc.full)
			}
		})
	}
}

func TestMacroAudioFormats(t *testing.T) {
	tests := []struct {
		name                                        string
		format                                      AudioFormat
		bits, bytes                                 uint16
		float, big, little, signed, isInt, unsigned bool
	}{
		{"UNKNOWN", AUDIO_UNKNOWN, 0, 0, false, false, true, false, true, true},
		{"U8", AUDIO_U8, 8, 1, false, false, true, false, true, true},
		{"S8", AUDIO_S8, 8, 1, false, false, true, true, true, false},
		{"S16LE", AUDIO_S16LE, 16, 2, false, false, true, true, true, false},
		{"S16BE", AUDIO_S16BE, 16, 2, false, true, false, true, true, false},
		{"S32LE", AUDIO_S32LE, 32, 4, false, false, true, true, true, false},
		{"S32BE", AUDIO_S32BE, 32, 4, false, true, false, true, true, false},
		{"F32LE", AUDIO_F32LE, 32, 4, true, false, true, true, false, false},
		{"F32BE", AUDIO_F32BE, 32, 4, true, true, false, true, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.format
			if got := AUDIO_BITSIZE(f); got != tc.bits {
				t.Errorf("AUDIO_BITSIZE = %d, want %d", got, tc.bits)
			}
			if got := AUDIO_BYTESIZE(f); got != tc.bytes {
				t.Errorf("AUDIO_BYTESIZE = %d, want %d", got, tc.bytes)
			}
			for _, b := range []struct {
				name      string
				got, want bool
			}{
				{"ISFLOAT", AUDIO_ISFLOAT(f), tc.float},
				{"ISBIGENDIAN", AUDIO_ISBIGENDIAN(f), tc.big},
				{"ISLITTLEENDIAN", AUDIO_ISLITTLEENDIAN(f), tc.little},
				{"ISSIGNED", AUDIO_ISSIGNED(f), tc.signed},
				{"ISINT", AUDIO_ISINT(f), tc.isInt},
				{"ISUNSIGNED", AUDIO_ISUNSIGNED(f), tc.unsigned},
			} {
				if b.got != b.want {
					t.Errorf("AUDIO_%s = %v, want %v", b.name, b.got, b.want)
				}
			}
		})
	}
}

func TestMacroAudioFrameSize(t *testing.T) {
	// C: SDL_AUDIO_FRAMESIZE of each spec.
	for _, tc := range []struct {
		spec AudioSpec
		want int32
	}{
		{AudioSpec{Format: AUDIO_S16LE, Channels: 2, Freq: 44100}, 4},
		{AudioSpec{Format: AUDIO_F32LE, Channels: 6, Freq: 48000}, 24},
		{AudioSpec{Format: AUDIO_U8, Channels: 1, Freq: 8000}, 1},
	} {
		if got := AUDIO_FRAMESIZE(tc.spec); got != tc.want {
			t.Errorf("AUDIO_FRAMESIZE(%+v) = %d, want %d", tc.spec, got, tc.want)
		}
	}
}

func TestMacroDefine(t *testing.T) {
	// DEFINE_* builders and FOURCC against the C macros.
	tests := []struct {
		src       string
		got, want uint32
	}{
		{"DEFINE_PIXELFORMAT(PIXELTYPE_PACKED32, uint32(PACKEDORDER_ARGB), uint32(PACKEDLAYOUT_8888), 32, 4)", uint32(DEFINE_PIXELFORMAT(PIXELTYPE_PACKED32, uint32(PACKEDORDER_ARGB), uint32(PACKEDLAYOUT_8888), 32, 4)), 0x16362004},
		{"DEFINE_PIXELFORMAT(PIXELTYPE_ARRAYF16, uint32(ARRAYORDER_RGBA), 0, 64, 8)", uint32(DEFINE_PIXELFORMAT(PIXELTYPE_ARRAYF16, uint32(ARRAYORDER_RGBA), 0, 64, 8)), 0x1a204008},
		{"DEFINE_PIXELFORMAT(PIXELTYPE_INDEX8, 0, 0, 8, 1)", uint32(DEFINE_PIXELFORMAT(PIXELTYPE_INDEX8, 0, 0, 8, 1)), 0x13000801},
		{"DEFINE_PIXELFOURCC('Y', 'U', 'Y', '2')", uint32(DEFINE_PIXELFOURCC('Y', 'U', 'Y', '2')), 0x32595559},
		{"FOURCC('N', 'V', '1', '2')", uint32(FOURCC('N', 'V', '1', '2')), 0x3231564e},
		{"FOURCC(0xff, 0, 0x80, 0x7f)", uint32(FOURCC(0xff, 0, 0x80, 0x7f)), 0x7f8000ff},
		{"DEFINE_COLORSPACE(COLOR_TYPE_RGB, COLOR_RANGE_FULL, COLOR_PRIMARIES_BT709, TRANSFER_CHARACTERISTICS_SRGB, MATRIX_COEFFICIENTS_IDENTITY, CHROMA_LOCATION_NONE)", uint32(DEFINE_COLORSPACE(COLOR_TYPE_RGB, COLOR_RANGE_FULL, COLOR_PRIMARIES_BT709, TRANSFER_CHARACTERISTICS_SRGB, MATRIX_COEFFICIENTS_IDENTITY, CHROMA_LOCATION_NONE)), 0x120005a0},
		{"DEFINE_COLORSPACE(COLOR_TYPE_YCBCR, COLOR_RANGE_LIMITED, COLOR_PRIMARIES_BT601, TRANSFER_CHARACTERISTICS_BT601, MATRIX_COEFFICIENTS_BT601, CHROMA_LOCATION_LEFT)", uint32(DEFINE_COLORSPACE(COLOR_TYPE_YCBCR, COLOR_RANGE_LIMITED, COLOR_PRIMARIES_BT601, TRANSFER_CHARACTERISTICS_BT601, MATRIX_COEFFICIENTS_BT601, CHROMA_LOCATION_LEFT)), 0x211018c6},
		{"DEFINE_AUDIO_FORMAT(true, false, false, 16)", uint32(DEFINE_AUDIO_FORMAT(true, false, false, 16)), 0x8010},
		{"DEFINE_AUDIO_FORMAT(true, true, true, 32)", uint32(DEFINE_AUDIO_FORMAT(true, true, true, 32)), 0x9120},
		{"DEFINE_AUDIO_FORMAT(false, false, false, 8)", uint32(DEFINE_AUDIO_FORMAT(false, false, false, 8)), 0x8},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %#x, want %#x", tc.src, tc.got, tc.want)
		}
	}
	// The builders must also reproduce the generated constants.
	if got := DEFINE_PIXELFORMAT(PIXELTYPE_PACKED32, uint32(PACKEDORDER_ARGB), uint32(PACKEDLAYOUT_8888), 32, 4); got != PIXELFORMAT_ARGB8888 {
		t.Errorf("DEFINE_PIXELFORMAT(ARGB8888) = %#x, want %#x", uint32(got), uint32(PIXELFORMAT_ARGB8888))
	}
	if got := DEFINE_PIXELFOURCC('Y', 'U', 'Y', '2'); got != PIXELFORMAT_YUY2 {
		t.Errorf("DEFINE_PIXELFOURCC(YUY2) = %#x", uint32(got))
	}
	if got := DEFINE_AUDIO_FORMAT(true, false, false, 16); got != AUDIO_S16LE {
		t.Errorf("DEFINE_AUDIO_FORMAT(S16LE) = %#x", uint32(got))
	}
	if got := DEFINE_COLORSPACE(COLOR_TYPE_RGB, COLOR_RANGE_FULL, COLOR_PRIMARIES_BT709, TRANSFER_CHARACTERISTICS_SRGB, MATRIX_COEFFICIENTS_IDENTITY, CHROMA_LOCATION_NONE); got != COLORSPACE_SRGB {
		t.Errorf("DEFINE_COLORSPACE(SRGB) = %#x", uint32(got))
	}
}

func TestMacroVersion(t *testing.T) {
	// C: SDL_VERSIONNUM(3,4,18)=3004018, (0,0,0)=0, (2,30,999)=2030999.
	for _, tc := range []struct{ major, minor, patch, want int }{
		{3, 4, 18, 3004018}, {0, 0, 0, 0}, {2, 30, 999, 2030999},
	} {
		if got := VERSIONNUM(tc.major, tc.minor, tc.patch); got != tc.want {
			t.Errorf("VERSIONNUM(%d,%d,%d) = %d, want %d", tc.major, tc.minor, tc.patch, got, tc.want)
		}
	}
	for _, tc := range []struct{ v, major, minor, micro int }{
		{3004018, 3, 4, 18},
		{3005000, 3, 5, 0},
		{2030999, 2, 30, 999},
		{0, 0, 0, 0},
		{1002003, 1, 2, 3},
		{3005000, 3, 5, 0},
	} {
		if got := VERSIONNUM_MAJOR(tc.v); got != tc.major {
			t.Errorf("VERSIONNUM_MAJOR(%d) = %d, want %d", tc.v, got, tc.major)
		}
		if got := VERSIONNUM_MINOR(tc.v); got != tc.minor {
			t.Errorf("VERSIONNUM_MINOR(%d) = %d, want %d", tc.v, got, tc.minor)
		}
		if got := VERSIONNUM_MICRO(tc.v); got != tc.micro {
			t.Errorf("VERSIONNUM_MICRO(%d) = %d, want %d", tc.v, got, tc.micro)
		}
	}
	// SDL_VERSION is 3004018 in the release 3.4.18 headers. After bumping
	// SDL_VERSION, recompute the table below with clang and update this value.
	if VERSION != 3004018 {
		t.Fatalf("VERSION = %d; the expectations below assume 3004018", VERSION)
	}
	if b, err := os.ReadFile("../SDL_VERSION"); err != nil {
		t.Fatal(err)
	} else if want := fmt.Sprintf("release-%d.%d.%d\n", VERSIONNUM_MAJOR(VERSION), VERSIONNUM_MINOR(VERSION), VERSIONNUM_MICRO(VERSION)); string(b) != want {
		t.Errorf("SDL_VERSION file = %q, generated bindings are for %q", b, want)
	}
	for _, tc := range []struct {
		x, y, z int
		want    bool
	}{
		{3, 5, 0, false},
		{3, 5, 1, false},
		{3, 4, 99, false},
		{4, 0, 0, false},
		{3, 2, 0, true},
		{2, 99, 99, true},
		{3, 6, 0, false},
		{3, 4, 18, true},
		{3, 4, 19, false},
		{3, 4, 17, true},
	} {
		if got := VERSION_ATLEAST(tc.x, tc.y, tc.z); got != tc.want {
			t.Errorf("VERSION_ATLEAST(%d,%d,%d) = %v, want %v", tc.x, tc.y, tc.z, got, tc.want)
		}
	}
}

func TestMacroWindowPos(t *testing.T) {
	for _, tc := range []struct {
		pos                 int32
		undefined, centered bool
	}{
		{0, false, false},
		{1, false, false},
		{100, false, false},
		{-5, false, false},
		{536805376, true, false},
		{805240832, false, true},
		{536805378, true, false},
		{805240835, false, true},
		{536805376, true, false},
		{805240832, false, true},
	} {
		if got := WINDOWPOS_ISUNDEFINED(tc.pos); got != tc.undefined {
			t.Errorf("WINDOWPOS_ISUNDEFINED(%d) = %v, want %v", tc.pos, got, tc.undefined)
		}
		if got := WINDOWPOS_ISCENTERED(tc.pos); got != tc.centered {
			t.Errorf("WINDOWPOS_ISCENTERED(%d) = %v, want %v", tc.pos, got, tc.centered)
		}
	}
	for _, tc := range []struct {
		name      string
		got, want uint32
	}{
		{"UNDEFINED_DISPLAY(0)", WINDOWPOS_UNDEFINED_DISPLAY(0), 536805376},
		{"UNDEFINED_DISPLAY(2)", WINDOWPOS_UNDEFINED_DISPLAY(2), 536805378},
		{"CENTERED_DISPLAY(0)", WINDOWPOS_CENTERED_DISPLAY(0), 805240832},
		{"CENTERED_DISPLAY(3)", WINDOWPOS_CENTERED_DISPLAY(3), 805240835},
	} {
		if tc.got != tc.want {
			t.Errorf("WINDOWPOS_%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// The button indices compare with the uint8 Button field of the mouse events.
func TestButtonIndexMatchesEventField(t *testing.T) {
	ev := MouseButtonEvent{Button: BUTTON_RIGHT}
	if ev.Button != BUTTON_RIGHT || ev.Button == BUTTON_LEFT {
		t.Errorf("Button = %d", ev.Button)
	}
}

func TestMacroButtonMask(t *testing.T) {
	for _, tc := range []struct {
		button uint8
		want   MouseButtonFlags
	}{
		{1, 0x1},
		{2, 0x2},
		{3, 0x4},
		{4, 0x8},
		{5, 0x10},
	} {
		if got := BUTTON_MASK(tc.button); got != tc.want {
			t.Errorf("BUTTON_MASK(%d) = %#x, want %#x", tc.button, uint32(got), uint32(tc.want))
		}
	}
	if BUTTON_MASK(BUTTON_LEFT) != BUTTON_LMASK || BUTTON_MASK(BUTTON_RIGHT) != BUTTON_RMASK {
		t.Error("BUTTON_MASK disagrees with BUTTON_LMASK/BUTTON_RMASK")
	}
}

func TestMacroScancodeToKeycode(t *testing.T) {
	for _, tc := range []struct {
		scancode Scancode
		want     Keycode
	}{
		{SCANCODE_A, 0x40000004},
		{SCANCODE_RETURN, 0x40000028},
		{SCANCODE_F1, 0x4000003a},
		{SCANCODE_LEFT, 0x40000050},
		{SCANCODE_LCTRL, 0x400000e0},
		{SCANCODE_KP_ENTER, 0x40000058},
	} {
		if got := SCANCODE_TO_KEYCODE(tc.scancode); got != tc.want {
			t.Errorf("SCANCODE_TO_KEYCODE(%d) = %#x, want %#x", tc.scancode, uint32(got), uint32(tc.want))
		}
	}
}

func TestMacroTime(t *testing.T) {
	// Columns: v, SECONDS_TO_NS(v), NS_TO_SECONDS(v*1e9+7), MS_TO_NS(v),
	// NS_TO_MS(v*1e6+999999), US_TO_NS(v), NS_TO_US(v*1e3+999), NS_TO_MS(v),
	// NS_TO_US(v).
	for _, tc := range []struct {
		v, s2ns, ns2s, ms2ns, ns2ms, us2ns, ns2us, rawMS, rawUS uint64
	}{
		{0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1000000000, 1, 1000000, 1, 1000, 1, 0, 0},
		{999, 999000000000, 999, 999000000, 999, 999000, 999, 0, 0},
		{1000, 1000000000000, 1000, 1000000000, 1000, 1000000, 1000, 0, 1},
		{1500, 1500000000000, 1500, 1500000000, 1500, 1500000, 1500, 0, 1},
		{123456789, 123456789000000000, 123456789, 123456789000000, 123456789, 123456789000, 123456789, 123, 123456},
		{3600, 3600000000000, 3600, 3600000000, 3600, 3600000, 3600, 0, 3},
	} {
		v := tc.v
		got := []uint64{
			SECONDS_TO_NS(v), NS_TO_SECONDS(v*1000000000 + 7), MS_TO_NS(v),
			NS_TO_MS(v*1000000 + 999999), US_TO_NS(v), NS_TO_US(v*1000 + 999),
			NS_TO_MS(v), NS_TO_US(v),
		}
		want := []uint64{tc.s2ns, tc.ns2s, tc.ms2ns, tc.ns2ms, tc.us2ns, tc.ns2us, tc.rawMS, tc.rawUS}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("v=%d column %d = %d, want %d", v, i+1, got[i], want[i])
			}
		}
	}
}

func TestMacroBits(t *testing.T) {
	for _, tc := range []struct {
		x      uint32
		msb    int
		oneBit bool
	}{
		{0x0, -1, false},
		{0x1, 0, true},
		{0x2, 1, true},
		{0x3, 1, false},
		{0x80, 7, true},
		{0xffff, 15, false},
		{0x10000, 16, true},
		{0x80000000, 31, true},
		{0xffffffff, 31, false},
		{0x7fffffff, 30, false},
	} {
		if got := MostSignificantBitIndex32(tc.x); got != tc.msb {
			t.Errorf("MostSignificantBitIndex32(%#x) = %d, want %d", tc.x, got, tc.msb)
		}
		if got := HasExactlyOneBitSet32(tc.x); got != tc.oneBit {
			t.Errorf("HasExactlyOneBitSet32(%#x) = %v, want %v", tc.x, got, tc.oneBit)
		}
	}
}

func TestMacroSizeOverflow(t *testing.T) {
	if bits.UintSize != 64 {
		t.Skip("expectations were computed for a 64-bit size_t")
	}
	for _, tc := range []struct {
		a, b  uint64
		mulOK bool
		mul   uint64
		addOK bool
		add   uint64
	}{
		{0, 0, true, 0, true, 0},
		{3, 4, true, 12, true, 7},
		{18446744073709551615, 1, true, 18446744073709551615, false, 0},
		{18446744073709551615, 2, false, 0, false, 0},
		{4294967296, 4294967296, false, 0, true, 8589934592},
		{4294967296, 2147483648, true, 9223372036854775808, true, 6442450944},
		{9223372036854775807, 2, true, 18446744073709551614, true, 9223372036854775809},
		{0, 18446744073709551615, true, 0, true, 18446744073709551615},
	} {
		mulU, ok := SizeMulCheckOverflow(uint(tc.a), uint(tc.b))
		mul := uint64(mulU)
		if ok != tc.mulOK || (ok && mul != tc.mul) {
			t.Errorf("SizeMulCheckOverflow(%d, %d) = %d, %v; want %d, %v", tc.a, tc.b, mul, ok, tc.mul, tc.mulOK)
		}
		addU, ok := SizeAddCheckOverflow(uint(tc.a), uint(tc.b))
		add := uint64(addU)
		if ok != tc.addOK || (ok && add != tc.add) {
			t.Errorf("SizeAddCheckOverflow(%d, %d) = %d, %v; want %d, %v", tc.a, tc.b, add, ok, tc.add, tc.addOK)
		}
	}
}

func TestMacroSwap(t *testing.T) {
	// Values printed by the C program for SDL_Swap* and SDL_SwapFloat*.
	if got := Swap16(0x1234); got != 0x3412 {
		t.Errorf("Swap16(0x1234) = %#x", got)
	}
	if got := Swap16(0xABCD); got != 0xcdab {
		t.Errorf("Swap16(0xABCD) = %#x", got)
	}
	if got := Swap32(0x12345678); got != 0x78563412 {
		t.Errorf("Swap32(0x12345678) = %#x", got)
	}
	if got := Swap32(0xDEADBEEF); got != 0xefbeadde {
		t.Errorf("Swap32(0xDEADBEEF) = %#x", got)
	}
	if got := Swap64(0x0123456789ABCDEF); got != 0xefcdab8967452301 {
		t.Errorf("Swap64 = %#x", got)
	}
	if got := math.Float32bits(SwapFloat(1.0)); got != 0x803f {
		t.Errorf("SwapFloat(1) bits = %#x, want 0x803f", got)
	}
	if got := math.Float32bits(SwapFloat(-2.5)); got != 0x20c0 {
		t.Errorf("SwapFloat(-2.5) bits = %#x, want 0x20c0", got)
	}
	// The C results for LE/BE were taken on a little-endian host.
	if hostBigEndian {
		t.Skip("LE/BE expectations are for a little-endian host")
	}
	if got := Swap16LE(0x1234); got != 0x1234 {
		t.Errorf("Swap16LE = %#x", got)
	}
	if got := Swap16BE(0x1234); got != 0x3412 {
		t.Errorf("Swap16BE = %#x", got)
	}
	if got := Swap32LE(0x12345678); got != 0x12345678 {
		t.Errorf("Swap32LE = %#x", got)
	}
	if got := Swap32BE(0x12345678); got != 0x78563412 {
		t.Errorf("Swap32BE = %#x", got)
	}
	if got := Swap64LE(0x0123456789ABCDEF); got != 0x0123456789abcdef {
		t.Errorf("Swap64LE = %#x", got)
	}
	if got := Swap64BE(0x0123456789ABCDEF); got != 0xefcdab8967452301 {
		t.Errorf("Swap64BE = %#x", got)
	}
	if got := math.Float32bits(SwapFloatLE(1.0)); got != 0x3f800000 {
		t.Errorf("SwapFloatLE bits = %#x", got)
	}
	if got := math.Float32bits(SwapFloatBE(1.0)); got != 0x803f {
		t.Errorf("SwapFloatBE bits = %#x", got)
	}
}

// macroMatrix checks a row-major boolean matrix encoded as one string of '0'
// and '1' per row, as printed by the C program.
func macroMatrix(t *testing.T, name string, rows []string, n, m int, f func(i, j int) bool) {
	t.Helper()
	if len(rows) != n {
		t.Fatalf("%s: %d rows, want %d", name, len(rows), n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if want := rows[i][j] == '1'; f(i, j) != want {
				t.Errorf("%s[%d][%d] = %v, want %v", name, i, j, !want, want)
			}
		}
	}
}

func TestMacroRects(t *testing.T) {
	rs := []Rect{{0, 0, 0, 0}, {1, 2, 3, 4}, {1, 2, 3, 4}, {-5, -5, 10, 10}, {0, 0, -1, 5}, {0, 0, 5, 0}, {10, 20, 30, 40}}
	ps := []Point{{1, 2}, {3, 4}, {4, 6}, {5, 5}, {-5, -5}, {0, 0}, {4, 5}, {3, 5}, {39, 59}, {40, 60}, {9, 19}}

	empty := "1000110"
	for i := range rs {
		if got := RectEmpty(&rs[i]); got != (empty[i] == '1') {
			t.Errorf("RectEmpty(%v) = %v", rs[i], got)
		}
	}
	macroMatrix(t, "RectsEqual", []string{"1000000", "0110000", "0110000", "0001000", "0000100", "0000010", "0000001"}, 7, 7,
		func(i, j int) bool { return RectsEqual(&rs[i], &rs[j]) })
	macroMatrix(t, "PointInRect", []string{"00000000000", "11000001000", "11000001000", "11001100000", "00000000000", "00000000000", "00000000100"}, 7, 11,
		func(i, j int) bool { return PointInRect(&ps[j], &rs[i]) })

	// RectToFRect: the C results are the exact float conversions.
	for _, r := range rs {
		want := FRect{float32(r.X), float32(r.Y), float32(r.W), float32(r.H)}
		if got := RectToFRect(&r); got != want {
			t.Errorf("RectToFRect(%v) = %v, want %v", r, got, want)
		}
	}
	if got := RectToFRect(&Rect{-5, -5, 10, 10}); got != (FRect{-5, -5, 10, 10}) {
		t.Errorf("RectToFRect = %v", got)
	}

	// C: SDL_RectEmpty(NULL)=true, SDL_RectsEqual(NULL,r)=false,
	// SDL_PointInRect(p,NULL)=false.
	if !RectEmpty(nil) || RectsEqual(nil, &rs[0]) || RectsEqual(&rs[0], nil) || PointInRect(&ps[0], nil) || PointInRect(nil, &rs[1]) {
		t.Error("nil handling differs from C")
	}
}

func TestMacroFloatRects(t *testing.T) {
	fr := []FRect{{0, 0, 0, 0}, {1, 2, 3, 4}, {1, 2, 3, 4}, {1, 2, 3, 4.0000002}, {1.00001, 2, 3, 4}, {0, 0, -1, 5}, {0, 0, 5, -0.5}, {10, 20, 30, 40}}
	fp := []FPoint{{1, 2}, {4, 6}, {4.0001, 6}, {0, 0}, {40, 60}, {40.5, 60}, {10, 20}, {-1, 2}}

	empty := "00000110"
	for i := range fr {
		if got := RectEmptyFloat(&fr[i]); got != (empty[i] == '1') {
			t.Errorf("RectEmptyFloat(%v) = %v", fr[i], got)
		}
	}
	macroMatrix(t, "RectsEqualFloat", []string{"10000000", "01110000", "01110000", "01110000", "00001000", "00000100", "00000010", "00000001"}, 8, 8,
		func(i, j int) bool { return RectsEqualFloat(&fr[i], &fr[j]) })
	macroMatrix(t, "RectsEqualEpsilon(0.001)", []string{"10000000", "01111000", "01111000", "01111000", "01111000", "00000100", "00000010", "00000001"}, 8, 8,
		func(i, j int) bool { return RectsEqualEpsilon(&fr[i], &fr[j], 0.001) })
	macroMatrix(t, "PointInRectFloat", []string{"00010000", "11000000", "11000000", "11000000", "01000000", "00000000", "00000000", "00001010"}, 8, 8,
		func(i, j int) bool { return PointInRectFloat(&fp[j], &fr[i]) })

	// C: SDL_RectEmptyFloat(NULL)=true, RectsEqualFloat(NULL,r)=false,
	// RectsEqualEpsilon(r,NULL,1)=false, RectsEqualEpsilon(r,r,-1)=true
	// (identical pointers short-circuit).
	if !RectEmptyFloat(nil) || RectsEqualFloat(nil, &fr[0]) || RectsEqualEpsilon(&fr[0], nil, 1) || !RectsEqualEpsilon(&fr[1], &fr[1], -1) || PointInRectFloat(nil, &fr[0]) {
		t.Error("nil handling differs from C")
	}
}

func TestMacroCreateThread(t *testing.T) {
	if !Available("SDL_CreateThreadRuntime") || !Available("SDL_CreateThreadWithPropertiesRuntime") {
		t.Skip("thread runtime entry points not exported by the loaded SDL3")
	}
	var calls atomic.Int32
	var gotData atomic.Uintptr
	datum := new(int32)
	fn := NewThreadFunction(func(data unsafe.Pointer) int32 {
		calls.Add(1)
		gotData.Store(uintptr(data))
		return 42
	})

	th := CreateThread(fn, "macro-thread", unsafe.Pointer(datum))
	if th == nil {
		t.Fatalf("CreateThread returned nil: %s", GetError())
	}
	if name := GetThreadName(th); name != "macro-thread" {
		t.Errorf("GetThreadName = %q", name)
	}
	var status int32
	WaitThread(th, &status)
	if status != 42 || calls.Load() != 1 || gotData.Load() != uintptr(unsafe.Pointer(datum)) {
		t.Errorf("thread status=%d calls=%d data match=%v", status, calls.Load(), gotData.Load() == uintptr(unsafe.Pointer(datum)))
	}

	props := CreateProperties()
	defer DestroyProperties(props)
	SetPointerProperty(props, PROP_THREAD_CREATE_ENTRY_FUNCTION_POINTER, *(*unsafe.Pointer)(unsafe.Pointer(&fn)))
	SetStringProperty(props, PROP_THREAD_CREATE_NAME_STRING, Ptr("macro-props"))
	th = CreateThreadWithProperties(props)
	if th == nil {
		t.Fatalf("CreateThreadWithProperties returned nil: %s", GetError())
	}
	if name := GetThreadName(th); name != "macro-props" {
		t.Errorf("GetThreadName = %q", name)
	}
	status = 0
	WaitThread(th, &status)
	if status != 42 || calls.Load() != 2 {
		t.Errorf("thread status=%d calls=%d", status, calls.Load())
	}
}
