//go:build (darwin || linux || windows) && (amd64 || arm64)

package sdl

import (
	"sync"
	"unsafe"
)

// floatCalls reports whether callF0 ... callF8 can be used: the trampoline
// loads the floating-point argument registers too.
const floatCalls = true

// registerFallback is a no-op: generated floating-point functions call C
// through callFN and never use their purego.RegisterFunc fallback here.
func registerFallback(fptr any, symbol string) {}

// callFrame is the argument block of the assembly trampoline in
// call_trampoline_*.s, which hard-codes its offsets: fn at 0, the sixteen
// argument words at 8, r at 136 and fr at 144. How the trampoline maps the
// argument words to registers depends on the ABI (see [fillFrame]); it then
// calls fn and stores the integer and floating-point result registers into r
// and fr.
type callFrame struct {
	fn uintptr
	a  [2 * maxDirectArgs]uintptr
	r  uintptr
	fr uint64
}

// Fail the build if callFrame drifts from the offsets in the assembly.
var _ [1]struct{} = [unsafe.Offsetof(callFrame{}.fr) - 143]struct{}{}

// trampolineABI0 is the address of the trampoline, set in assembly.
var trampolineABI0 uintptr

// cgocall runs fn(arg) on the system stack with the C calling convention,
// after telling the scheduler that the goroutine is in a foreign call. The
// runtime exports it to purego with a push linkname; purego's fake cgo
// runtime makes it usable in CGO_ENABLED=0 builds on darwin and linux, and
// Windows needs no cgo runtime for it.
//
//go:linkname cgocall runtime.cgocall
//go:noescape
func cgocall(fn uintptr, arg unsafe.Pointer) int32

// frames recycles call frames. A frame must not live on the goroutine stack:
// when the C function calls back into Go on the same thread, the callback
// runs on this goroutine's stack and may grow it, which moves the stack while
// the trampoline still holds the frame address. (cgo avoids this by
// recomputing the address with _cgo_topofstack in its C stubs.) A frame
// holds only integers, so the garbage collector never scans it.
var frames = sync.Pool{New: func() any { return new(callFrame) }}

// noFloats is the floatArgs of an integer-only call.
var noFloats floatArgs

// call fills a pooled frame with fn and the arguments, calls fn and returns
// the integer and floating-point result registers. a holds the integer words
// and f the floating-point words, each in parameter order; bit i of fmask is
// set when parameter i is floating-point.
func call(fn uintptr, a *[maxDirectArgs]uintptr, f *floatArgs, fmask uint16) (uintptr, uint64) {
	fr := frames.Get().(*callFrame)
	fr.fn = fn
	fillFrame(fr, a, f, fmask)
	cgocall(trampolineABI0, unsafe.Pointer(fr))
	r, rf := fr.r, fr.fr
	frames.Put(fr)
	return r, rf
}

func call0(fn uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call1(fn, a0 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call2(fn, a0, a1 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call3(fn, a0, a1, a2 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call4(fn, a0, a1, a2, a3 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call5(fn, a0, a1, a2, a3, a4 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call6(fn, a0, a1, a2, a3, a4, a5 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call7(fn, a0, a1, a2, a3, a4, a5, a6 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5, a6}, &noFloats, 0)
	return r
}

//go:uintptrescapes
func call8(fn, a0, a1, a2, a3, a4, a5, a6, a7 uintptr) uintptr {
	r, _ := call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5, a6, a7}, &noFloats, 0)
	return r
}

func callF0(fn uintptr, fmask uint16, f *floatArgs) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{}, f, fmask)
}

//go:uintptrescapes
func callF1(fn uintptr, fmask uint16, f *floatArgs, a0 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0}, f, fmask)
}

//go:uintptrescapes
func callF2(fn uintptr, fmask uint16, f *floatArgs, a0, a1 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1}, f, fmask)
}

//go:uintptrescapes
func callF3(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2}, f, fmask)
}

//go:uintptrescapes
func callF4(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3}, f, fmask)
}

//go:uintptrescapes
func callF5(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4}, f, fmask)
}

//go:uintptrescapes
func callF6(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5}, f, fmask)
}

//go:uintptrescapes
func callF7(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5, a6}, f, fmask)
}

//go:uintptrescapes
func callF8(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5, a6, a7 uintptr) (uintptr, uint64) {
	return call(fn, &[maxDirectArgs]uintptr{a0, a1, a2, a3, a4, a5, a6, a7}, f, fmask)
}
