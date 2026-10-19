package sdl

import (
	"math"
	"unsafe"
)

// The generated direct-call functions (fn<Name> declared as functions, not
// variables) reach C through call0 ... call8 and callF0 ... callF8 instead of
// the reflective wrappers that purego.RegisterFunc installs. The generator
// uses them for signatures whose parameters and result are each one integer
// word (integers, enums, bool, pointers) or one floating-point value, with at
// most eight integer and eight floating-point parameters, so that every
// argument is passed in a register (Apple arm64 packs stack arguments by
// their natural size, which a uniform word array cannot express). Structs
// passed by value and variadic functions stay on purego.RegisterFunc.
//
// callN takes N integer words. callFN additionally takes the floating-point
// arguments as a *floatArgs, in parameter order, and a mask whose bit i is
// set when parameter i is floating-point (the Microsoft x64 ABI assigns
// registers by position and needs it); it returns the raw integer and
// floating-point result registers, and the generated code keeps whichever the
// C result type uses.
//
// Two implementations exist. On darwin, linux and windows for amd64 and arm64
// (call_trampoline.go) a small assembly trampoline loads the integer and
// floating-point argument registers from a pooled frame and is entered
// through runtime.cgocall, as cgo does; it needs neither cgo nor a
// C toolchain because purego's fake cgo runtime enables runtime.cgocall in
// CGO_ENABLED=0 builds (Windows needs no cgo runtime for it). The frame
// layout is ABI-specific: call_regclass.go for System V and AAPCS64,
// call_windows_amd64.go for Microsoft x64. Elsewhere (call_syscall.go) callN uses
// purego.SyscallN and floating-point signatures fall back to
// purego.RegisterFunc: [floatCalls] is false there, and every generated
// floating-point function then calls its reg<Name> variable, which
// [registerFallback] binds.
//
// Each callN and callFN is marked go:uintptrescapes: an argument written as
// uintptr(unsafe.Pointer(p)) in the call expression moves the object p points
// to onto the heap and keeps it alive until the call returns, which is the
// rule purego.SyscallN itself relies on. No implementation allocates.

// maxDirectArgs is the largest number of integer parameters, and separately
// of floating-point parameters, of a direct-call function.
const maxDirectArgs = 8

// floatArgs holds the floating-point arguments of a callFN call in parameter
// order. A float32 occupies the low 32 bits of its slot (see [f32]).
type floatArgs [maxDirectArgs]uint64

// f32 returns the register image of a float32 argument.
func f32(x float32) uint64 { return uint64(math.Float32bits(x)) }

// f64 returns the register image of a float64 argument.
func f64(x float64) uint64 { return math.Float64bits(x) }

// retF32 extracts a float32 result from the floating-point result register.
func retF32(r uint64) float32 { return math.Float32frombits(uint32(r)) }

// retF64 extracts a float64 result from the floating-point result register.
func retF64(r uint64) float64 { return math.Float64frombits(r) }

// positionalArgs interleaves the integer words a and the floating-point words
// f into parameter order, as the Microsoft x64 ABI assigns argument slots:
// slot i takes the next floating-point word when bit i of fmask is set and
// the next integer word otherwise. Slots past the last parameter are zero.
func positionalArgs(a *[maxDirectArgs]uintptr, f *floatArgs, fmask uint16) [2 * maxDirectArgs]uintptr {
	var s [2 * maxDirectArgs]uintptr
	ai, fi := 0, 0
	for i := range s {
		if fmask&(1<<i) != 0 {
			if fi < len(f) {
				s[i] = uintptr(f[fi])
				fi++
			}
		} else if ai < len(a) {
			s[i] = a[ai]
			ai++
		}
	}
	return s
}

// scratchWords is the size of a call frame's scratch area in 8-byte words
// (512 bytes, matching scratchBytes in internal/bindgen).
const scratchWords = 64

// A direct call whose parameters point to single plain values (FRect, Event
// ...) copies them into the scratch area of its call frame and passes that
// address instead: passing the caller's address would move the value to the
// heap (go:uintptrescapes), so RenderFillRect(r, &FRect{...}) or a local
// Event given to PollEvent would allocate on every call. The generator lists
// the eligible types, all free of Go pointers, and assigns each parameter a
// fixed offset; the generated code gets the frame with getFrame, calls
// through callSN or callFSN, copies outputs back with scratchOut and returns
// the frame with putFrame.

// scratchIn copies *p to offset off of fr's scratch area and returns the
// address of the copy, or 0 when p is nil. T must contain no Go pointers.
func scratchIn[T any](fr *callFrame, off uintptr, p *T) uintptr {
	if p == nil {
		return 0
	}
	q := (*T)(unsafe.Add(unsafe.Pointer(&fr.scratch), off))
	*q = *p
	return uintptr(unsafe.Pointer(q))
}

// scratchOut copies the value at offset off of fr's scratch area back to *p
// after the call; it does nothing when p is nil.
func scratchOut[T any](fr *callFrame, off uintptr, p *T) {
	if p != nil {
		*p = *(*T)(unsafe.Add(unsafe.Pointer(&fr.scratch), off))
	}
}

// cstrScratch passes the string arguments of one call. A string without a
// trailing NUL needs a NUL-terminated copy; a short one goes into the scratch
// area of a call frame acquired on first use instead of a fresh allocation,
// so SDL constants such as PROP_* and HINT_* names do not allocate. The
// generated wrapper declares one on its stack and defers release; SDL must
// not keep the pointer after the call (strings that SDL retains are passed
// as *byte instead).
type cstrScratch struct {
	fr *callFrame
	n  uintptr // bytes of fr.scratch in use
}

// cstr is like the package-level cstr, copying into the scratch area when
// the copy fits.
func (c *cstrScratch) cstr(s string) *byte {
	if len(s) > 0 && s[len(s)-1] == 0 {
		return unsafe.StringData(s)
	}
	need := uintptr(len(s)) + 1
	if need > uintptr(len(callFrame{}.scratch))*8-c.n {
		return CString(s)
	}
	if c.fr == nil {
		c.fr = getFrame()
	}
	b := unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(&c.fr.scratch), c.n)), need)
	copy(b, s)
	b[len(s)] = 0
	c.n += need
	return &b[0]
}

// cstrNull is like the package-level cstrNull.
func (c *cstrScratch) cstrNull(s string) *byte {
	if s == "" {
		return nil
	}
	return c.cstr(s)
}

// release returns the call frame, if one was acquired, after the call.
func (c *cstrScratch) release() {
	if c.fr != nil {
		putFrame(c.fr)
	}
}

// cbool converts a Go bool to the 0 or 1 that C expects.
func cbool(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// cptr converts a pointer returned by C to unsafe.Pointer. The value is C
// memory, never Go memory, so the conversion does not hide a Go pointer from
// the garbage collector.
func cptr(r uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&r)) }

// missingSymbol panics for a direct-call function whose C symbol is absent
// from the loaded library, like the stub [register] installs.
func missingSymbol(symbol string) {
	panic("sdl: " + symbol + " is not available in the loaded SDL3 library")
}
