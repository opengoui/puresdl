package sdl

// This file holds hand-written Go versions of the SDL function-like macros
// and static inline functions, which cannot be bound through the shared
// library. Each one mirrors the definition in the SDL 3.5.0 headers.

import (
	"math"
	"math/bits"
	"unsafe"
)

// Version macros (SDL_version.h).

// VERSIONNUM packs major, minor and patch into a single comparable number, as
// returned by [GetVersion].
func VERSIONNUM(major, minor, patch int) int {
	return major*1000000 + minor*1000 + patch
}

// VERSIONNUM_MAJOR extracts the major component of a version number.
func VERSIONNUM_MAJOR(version int) int { return version / 1000000 }

// VERSIONNUM_MINOR extracts the minor component of a version number.
func VERSIONNUM_MINOR(version int) int { return (version / 1000) % 1000 }

// VERSIONNUM_MICRO extracts the micro (patch) component of a version number.
func VERSIONNUM_MICRO(version int) int { return version % 1000 }

// VERSION_ATLEAST reports whether the SDL headers this binding was generated
// from are at least version x.y.z. It does not query the loaded library; use
// [GetVersion] for that.
func VERSION_ATLEAST(x, y, z int) bool {
	return VERSION >= VERSIONNUM(x, y, z)
}

// FOURCC builds a four-character code from its bytes (SDL_FOURCC).
func FOURCC(a, b, c, d byte) uint32 {
	return uint32(a) | uint32(b)<<8 | uint32(c)<<16 | uint32(d)<<24
}

// Pixel format macros (SDL_pixels.h).

// DEFINE_PIXELFOURCC builds a [PixelFormat] from a four-character code.
func DEFINE_PIXELFOURCC(a, b, c, d byte) PixelFormat {
	return PixelFormat(FOURCC(a, b, c, d))
}

// DEFINE_PIXELFORMAT builds a [PixelFormat] from its components. The order
// and layout arguments are the numeric value of a [PackedOrder], [ArrayOrder]
// or [BitmapOrder], and of a [PackedLayout].
func DEFINE_PIXELFORMAT(typ PixelType, order, layout, bitsPerPixel, bytesPerPixel uint32) PixelFormat {
	return PixelFormat(1<<28 | uint32(typ)<<24 | order<<20 | layout<<16 | bitsPerPixel<<8 | bytesPerPixel)
}

// PIXELFLAG returns the flag nibble of a pixel format.
func PIXELFLAG(format PixelFormat) uint32 { return uint32(format>>28) & 0x0F }

// PIXELTYPE returns the [PixelType] of a pixel format.
func PIXELTYPE(format PixelFormat) PixelType { return PixelType(format>>24) & 0x0F }

// PIXELORDER returns the channel order of a pixel format, as the numeric value
// of a [PackedOrder], [ArrayOrder] or [BitmapOrder].
func PIXELORDER(format PixelFormat) uint32 { return uint32(format>>20) & 0x0F }

// PIXELLAYOUT returns the [PackedLayout] of a pixel format as a number.
func PIXELLAYOUT(format PixelFormat) uint32 { return uint32(format>>16) & 0x0F }

// ISPIXELFORMAT_FOURCC reports whether format is a FourCC (non-enumerated)
// format.
func ISPIXELFORMAT_FOURCC(format PixelFormat) bool {
	return format != 0 && PIXELFLAG(format) != 1
}

// BITSPERPIXEL returns the bits per pixel of format, or 0 for FourCC formats.
func BITSPERPIXEL(format PixelFormat) uint32 {
	if ISPIXELFORMAT_FOURCC(format) {
		return 0
	}
	return uint32(format>>8) & 0xFF
}

// BYTESPERPIXEL returns the bytes per pixel of format. FourCC formats get a
// best-effort value.
func BYTESPERPIXEL(format PixelFormat) uint32 {
	if ISPIXELFORMAT_FOURCC(format) {
		switch format {
		case PIXELFORMAT_YUY2, PIXELFORMAT_UYVY, PIXELFORMAT_YVYU,
			PIXELFORMAT_P010, PIXELFORMAT_I0FL, PIXELFORMAT_I4FL:
			return 2
		}
		return 1
	}
	return uint32(format) & 0xFF
}

// ISPIXELFORMAT_INDEXED reports whether format is an indexed (palette) format.
func ISPIXELFORMAT_INDEXED(format PixelFormat) bool {
	if ISPIXELFORMAT_FOURCC(format) {
		return false
	}
	switch PIXELTYPE(format) {
	case PIXELTYPE_INDEX1, PIXELTYPE_INDEX2, PIXELTYPE_INDEX4, PIXELTYPE_INDEX8:
		return true
	}
	return false
}

// ISPIXELFORMAT_PACKED reports whether format is a packed format.
func ISPIXELFORMAT_PACKED(format PixelFormat) bool {
	if ISPIXELFORMAT_FOURCC(format) {
		return false
	}
	switch PIXELTYPE(format) {
	case PIXELTYPE_PACKED8, PIXELTYPE_PACKED16, PIXELTYPE_PACKED32:
		return true
	}
	return false
}

// ISPIXELFORMAT_ARRAY reports whether format is an array format.
func ISPIXELFORMAT_ARRAY(format PixelFormat) bool {
	if ISPIXELFORMAT_FOURCC(format) {
		return false
	}
	switch PIXELTYPE(format) {
	case PIXELTYPE_ARRAYU8, PIXELTYPE_ARRAYU16, PIXELTYPE_ARRAYU32,
		PIXELTYPE_ARRAYF16, PIXELTYPE_ARRAYF32:
		return true
	}
	return false
}

// ISPIXELFORMAT_10BIT reports whether format is a packed 10-bit format.
func ISPIXELFORMAT_10BIT(format PixelFormat) bool {
	return !ISPIXELFORMAT_FOURCC(format) &&
		PIXELTYPE(format) == PIXELTYPE_PACKED32 &&
		PIXELLAYOUT(format) == uint32(PACKEDLAYOUT_2101010)
}

// ISPIXELFORMAT_FLOAT reports whether format is a floating point format.
func ISPIXELFORMAT_FLOAT(format PixelFormat) bool {
	if ISPIXELFORMAT_FOURCC(format) {
		return false
	}
	t := PIXELTYPE(format)
	return t == PIXELTYPE_ARRAYF16 || t == PIXELTYPE_ARRAYF32
}

// ISPIXELFORMAT_ALPHA reports whether format has an alpha channel.
func ISPIXELFORMAT_ALPHA(format PixelFormat) bool {
	order := PIXELORDER(format)
	if ISPIXELFORMAT_PACKED(format) {
		switch PackedOrder(order) {
		case PACKEDORDER_ARGB, PACKEDORDER_RGBA, PACKEDORDER_ABGR, PACKEDORDER_BGRA:
			return true
		}
	}
	if ISPIXELFORMAT_ARRAY(format) {
		switch ArrayOrder(order) {
		case ARRAYORDER_ARGB, ARRAYORDER_RGBA, ARRAYORDER_ABGR, ARRAYORDER_BGRA:
			return true
		}
	}
	return false
}

// Colorspace macros (SDL_pixels.h).

// DEFINE_COLORSPACE builds a [Colorspace] from its components.
func DEFINE_COLORSPACE(typ ColorType, rng ColorRange, primaries ColorPrimaries, transfer TransferCharacteristics, matrix MatrixCoefficients, chroma ChromaLocation) Colorspace {
	return Colorspace(uint32(typ)<<28 | uint32(rng)<<24 | uint32(chroma)<<20 |
		uint32(primaries)<<10 | uint32(transfer)<<5 | uint32(matrix))
}

// COLORSPACETYPE returns the [ColorType] of a colorspace.
func COLORSPACETYPE(c Colorspace) ColorType { return ColorType(c>>28) & 0x0F }

// COLORSPACERANGE returns the [ColorRange] of a colorspace.
func COLORSPACERANGE(c Colorspace) ColorRange { return ColorRange(c>>24) & 0x0F }

// COLORSPACECHROMA returns the [ChromaLocation] of a colorspace.
func COLORSPACECHROMA(c Colorspace) ChromaLocation { return ChromaLocation(c>>20) & 0x0F }

// COLORSPACEPRIMARIES returns the [ColorPrimaries] of a colorspace.
func COLORSPACEPRIMARIES(c Colorspace) ColorPrimaries { return ColorPrimaries(c>>10) & 0x1F }

// COLORSPACETRANSFER returns the [TransferCharacteristics] of a colorspace.
func COLORSPACETRANSFER(c Colorspace) TransferCharacteristics {
	return TransferCharacteristics(c>>5) & 0x1F
}

// COLORSPACEMATRIX returns the [MatrixCoefficients] of a colorspace.
func COLORSPACEMATRIX(c Colorspace) MatrixCoefficients { return MatrixCoefficients(c) & 0x1F }

// ISCOLORSPACE_MATRIX_BT601 reports whether c uses BT.601 (or BT.470BG)
// matrix coefficients.
func ISCOLORSPACE_MATRIX_BT601(c Colorspace) bool {
	m := COLORSPACEMATRIX(c)
	return m == MATRIX_COEFFICIENTS_BT601 || m == MATRIX_COEFFICIENTS_BT470BG
}

// ISCOLORSPACE_MATRIX_BT709 reports whether c uses BT.709 matrix coefficients.
func ISCOLORSPACE_MATRIX_BT709(c Colorspace) bool {
	return COLORSPACEMATRIX(c) == MATRIX_COEFFICIENTS_BT709
}

// ISCOLORSPACE_MATRIX_BT2020_NCL reports whether c uses BT.2020 non-constant
// luminance matrix coefficients.
func ISCOLORSPACE_MATRIX_BT2020_NCL(c Colorspace) bool {
	return COLORSPACEMATRIX(c) == MATRIX_COEFFICIENTS_BT2020_NCL
}

// ISCOLORSPACE_LIMITED_RANGE reports whether c is not full range.
func ISCOLORSPACE_LIMITED_RANGE(c Colorspace) bool {
	return COLORSPACERANGE(c) != COLOR_RANGE_FULL
}

// ISCOLORSPACE_FULL_RANGE reports whether c is full range.
func ISCOLORSPACE_FULL_RANGE(c Colorspace) bool {
	return COLORSPACERANGE(c) == COLOR_RANGE_FULL
}

// Audio format macros (SDL_audio.h).

// DEFINE_AUDIO_FORMAT builds an [AudioFormat] from its components; size is the
// sample size in bits.
func DEFINE_AUDIO_FORMAT(signed, bigEndian, float bool, size uint16) AudioFormat {
	var v uint16
	if signed {
		v |= 1 << 15
	}
	if bigEndian {
		v |= 1 << 12
	}
	if float {
		v |= 1 << 8
	}
	return AudioFormat(v | size&AUDIO_MASK_BITSIZE)
}

// AUDIO_BITSIZE returns the sample size of f in bits.
func AUDIO_BITSIZE(f AudioFormat) uint16 { return uint16(f & AUDIO_MASK_BITSIZE) }

// AUDIO_BYTESIZE returns the sample size of f in bytes.
func AUDIO_BYTESIZE(f AudioFormat) uint16 { return AUDIO_BITSIZE(f) / 8 }

// AUDIO_ISFLOAT reports whether f is a floating point format.
func AUDIO_ISFLOAT(f AudioFormat) bool { return f&AUDIO_MASK_FLOAT != 0 }

// AUDIO_ISBIGENDIAN reports whether f is big-endian.
func AUDIO_ISBIGENDIAN(f AudioFormat) bool { return f&AUDIO_MASK_BIG_ENDIAN != 0 }

// AUDIO_ISLITTLEENDIAN reports whether f is little-endian.
func AUDIO_ISLITTLEENDIAN(f AudioFormat) bool { return !AUDIO_ISBIGENDIAN(f) }

// AUDIO_ISSIGNED reports whether f is signed.
func AUDIO_ISSIGNED(f AudioFormat) bool { return f&AUDIO_MASK_SIGNED != 0 }

// AUDIO_ISINT reports whether f is an integer format.
func AUDIO_ISINT(f AudioFormat) bool { return !AUDIO_ISFLOAT(f) }

// AUDIO_ISUNSIGNED reports whether f is unsigned.
func AUDIO_ISUNSIGNED(f AudioFormat) bool { return !AUDIO_ISSIGNED(f) }

// AUDIO_FRAMESIZE returns the size in bytes of one sample frame of spec.
func AUDIO_FRAMESIZE(spec AudioSpec) int32 {
	return int32(AUDIO_BYTESIZE(spec.Format)) * spec.Channels
}

// Window position macros (SDL_video.h).

// WINDOWPOS_UNDEFINED_DISPLAY returns the "undefined position" value for the
// given display index.
func WINDOWPOS_UNDEFINED_DISPLAY(display uint32) uint32 {
	return WINDOWPOS_UNDEFINED_MASK | display
}

// WINDOWPOS_ISUNDEFINED reports whether x is an "undefined position" value.
func WINDOWPOS_ISUNDEFINED(x int32) bool {
	return uint32(x)&0xFFFF0000 == WINDOWPOS_UNDEFINED_MASK
}

// WINDOWPOS_CENTERED_DISPLAY returns the "centered" value for the given
// display index.
func WINDOWPOS_CENTERED_DISPLAY(display uint32) uint32 {
	return WINDOWPOS_CENTERED_MASK | display
}

// WINDOWPOS_ISCENTERED reports whether x is a "centered" value.
func WINDOWPOS_ISCENTERED(x int32) bool {
	return uint32(x)&0xFFFF0000 == WINDOWPOS_CENTERED_MASK
}

// Input macros.

// BUTTON_MASK returns the [MouseButtonFlags] mask for mouse button x (1-based,
// such as [BUTTON_LEFT]).
func BUTTON_MASK(x int) MouseButtonFlags { return 1 << (x - 1) }

// SCANCODE_TO_KEYCODE converts a [Scancode] to the matching extended
// [Keycode].
func SCANCODE_TO_KEYCODE(s Scancode) Keycode {
	return Keycode(uint32(s) | SDLK_SCANCODE_MASK)
}

// Time conversion macros (SDL_timer.h).

// SECONDS_TO_NS converts seconds to nanoseconds.
func SECONDS_TO_NS(s uint64) uint64 { return s * NS_PER_SECOND }

// NS_TO_SECONDS converts nanoseconds to whole seconds.
func NS_TO_SECONDS(ns uint64) uint64 { return ns / NS_PER_SECOND }

// MS_TO_NS converts milliseconds to nanoseconds.
func MS_TO_NS(ms uint64) uint64 { return ms * NS_PER_MS }

// NS_TO_MS converts nanoseconds to whole milliseconds.
func NS_TO_MS(ns uint64) uint64 { return ns / NS_PER_MS }

// US_TO_NS converts microseconds to nanoseconds.
func US_TO_NS(us uint64) uint64 { return us * NS_PER_US }

// NS_TO_US converts nanoseconds to whole microseconds.
func NS_TO_US(ns uint64) uint64 { return ns / NS_PER_US }

// Rect inline functions (SDL_rect.h). Like the C versions they accept nil
// pointers and treat them as "no rectangle".

// RectToFRect converts r to a floating point rectangle. It returns the zero
// value for a nil r.
func RectToFRect(r *Rect) FRect {
	if r == nil {
		return FRect{}
	}
	return FRect{X: float32(r.X), Y: float32(r.Y), W: float32(r.W), H: float32(r.H)}
}

// PointInRect reports whether p lies inside r. The right and bottom edges are
// exclusive.
func PointInRect(p *Point, r *Rect) bool {
	return p != nil && r != nil &&
		p.X >= r.X && p.X < r.X+r.W &&
		p.Y >= r.Y && p.Y < r.Y+r.H
}

// RectEmpty reports whether r is nil or has a non-positive width or height.
func RectEmpty(r *Rect) bool { return r == nil || r.W <= 0 || r.H <= 0 }

// RectsEqual reports whether a and b are both non-nil and identical.
func RectsEqual(a, b *Rect) bool { return a != nil && b != nil && *a == *b }

// PointInRectFloat reports whether p lies inside r, edges inclusive.
func PointInRectFloat(p *FPoint, r *FRect) bool {
	return p != nil && r != nil &&
		p.X >= r.X && p.X <= r.X+r.W &&
		p.Y >= r.Y && p.Y <= r.Y+r.H
}

// RectEmptyFloat reports whether r is nil or has a negative width or height.
func RectEmptyFloat(r *FRect) bool { return r == nil || r.W < 0 || r.H < 0 }

// RectsEqualEpsilon reports whether a and b are both non-nil and every
// component differs by at most epsilon.
func RectsEqualEpsilon(a, b *FRect, epsilon float32) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	abs := func(x float32) float32 { return float32(math.Abs(float64(x))) }
	return abs(a.X-b.X) <= epsilon && abs(a.Y-b.Y) <= epsilon &&
		abs(a.W-b.W) <= epsilon && abs(a.H-b.H) <= epsilon
}

// RectsEqualFloat is [RectsEqualEpsilon] with [FLT_EPSILON].
func RectsEqualFloat(a, b *FRect) bool { return RectsEqualEpsilon(a, b, FLT_EPSILON) }

// Bit and overflow helpers (SDL_bits.h, SDL_stdinc.h).

// MostSignificantBitIndex32 returns the index of the highest set bit of x, or
// -1 if x is zero.
func MostSignificantBitIndex32(x uint32) int { return bits.Len32(x) - 1 }

// HasExactlyOneBitSet32 reports whether x is a power of two.
func HasExactlyOneBitSet32(x uint32) bool { return x != 0 && x&(x-1) == 0 }

// SizeMulCheckOverflow multiplies a and b. ok is false, and the product
// meaningless, if the result does not fit in a uint.
func SizeMulCheckOverflow(a, b uint) (product uint, ok bool) {
	hi, lo := bits.Mul(a, b)
	return lo, hi == 0
}

// SizeAddCheckOverflow adds a and b. ok is false, and the sum meaningless, if
// the result does not fit in a uint.
func SizeAddCheckOverflow(a, b uint) (sum uint, ok bool) {
	sum, carry := bits.Add(a, b, 0)
	return sum, carry == 0
}

// Endian helpers (SDL_endian.h).

// Swap16 reverses the byte order of x.
func Swap16(x uint16) uint16 { return bits.ReverseBytes16(x) }

// Swap32 reverses the byte order of x.
func Swap32(x uint32) uint32 { return bits.ReverseBytes32(x) }

// Swap64 reverses the byte order of x.
func Swap64(x uint64) uint64 { return bits.ReverseBytes64(x) }

// SwapFloat reverses the byte order of the IEEE 754 representation of x.
func SwapFloat(x float32) float32 {
	return math.Float32frombits(bits.ReverseBytes32(math.Float32bits(x)))
}

// hostBigEndian reports whether the running CPU is big-endian.
var hostBigEndian = func() bool {
	v := uint16(1)
	return *(*byte)(unsafe.Pointer(&v)) == 0
}()

// Swap16LE converts x between little-endian and host byte order.
func Swap16LE(x uint16) uint16 {
	if hostBigEndian {
		return Swap16(x)
	}
	return x
}

// Swap32LE converts x between little-endian and host byte order.
func Swap32LE(x uint32) uint32 {
	if hostBigEndian {
		return Swap32(x)
	}
	return x
}

// Swap64LE converts x between little-endian and host byte order.
func Swap64LE(x uint64) uint64 {
	if hostBigEndian {
		return Swap64(x)
	}
	return x
}

// SwapFloatLE converts x between little-endian and host byte order.
func SwapFloatLE(x float32) float32 {
	if hostBigEndian {
		return SwapFloat(x)
	}
	return x
}

// Swap16BE converts x between big-endian and host byte order.
func Swap16BE(x uint16) uint16 {
	if hostBigEndian {
		return x
	}
	return Swap16(x)
}

// Swap32BE converts x between big-endian and host byte order.
func Swap32BE(x uint32) uint32 {
	if hostBigEndian {
		return x
	}
	return Swap32(x)
}

// Swap64BE converts x between big-endian and host byte order.
func Swap64BE(x uint64) uint64 {
	if hostBigEndian {
		return x
	}
	return Swap64(x)
}

// SwapFloatBE converts x between big-endian and host byte order.
func SwapFloatBE(x float32) float32 {
	if hostBigEndian {
		return x
	}
	return SwapFloat(x)
}

// Thread creation macros (SDL_thread.h).

// CreateThread starts a new thread running fn(data). It is the Go form of the
// SDL_CreateThread macro, which passes the C runtime's thread begin/end
// functions only on Windows; here both are NULL.
func CreateThread(fn ThreadFunction, name string, data unsafe.Pointer) *Thread {
	return CreateThreadRuntime(fn, name, data, FunctionPointer(BeginThreadFunction), FunctionPointer(EndThreadFunction))
}

// CreateThreadWithProperties starts a new thread configured by props. It is
// the Go form of the SDL_CreateThreadWithProperties macro.
func CreateThreadWithProperties(props PropertiesID) *Thread {
	return CreateThreadWithPropertiesRuntime(props, FunctionPointer(BeginThreadFunction), FunctionPointer(EndThreadFunction))
}
