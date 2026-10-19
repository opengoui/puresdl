//go:build !((darwin || linux || windows) && (amd64 || arm64))

package sdl

import (
	"sync"

	"github.com/ebitengine/purego"
)

// floatCalls reports whether callF0 ... callF8 can be used. Here they cannot:
// purego.SyscallN does not load floating-point registers, so generated
// floating-point functions call their purego.RegisterFunc fallback instead.
const floatCalls = false

// registerFallback binds the purego.RegisterFunc fallback of a generated
// floating-point function, which is the only way to call it here.
func registerFallback(fptr any, symbol string) { register(fptr, symbol) }

// The variadic argument of a go:uintptrescapes function escapes, so the
// arguments are copied into a pooled array instead of a fresh slice.
var callArgs = sync.Pool{New: func() any { return new([maxDirectArgs]uintptr) }}

// callPooled calls fn with the first n words of a and clears them for reuse.
func callPooled(fn uintptr, a *[maxDirectArgs]uintptr, n int) uintptr {
	r, _, _ := purego.SyscallN(fn, a[:n]...)
	clear(a[:n])
	callArgs.Put(a)
	return r
}

func call0(fn uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn)
	return r
}

//go:uintptrescapes
func call1(fn, a0 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0] = a0
	return callPooled(fn, a, 1)
}

//go:uintptrescapes
func call2(fn, a0, a1 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1] = a0, a1
	return callPooled(fn, a, 2)
}

//go:uintptrescapes
func call3(fn, a0, a1, a2 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2] = a0, a1, a2
	return callPooled(fn, a, 3)
}

//go:uintptrescapes
func call4(fn, a0, a1, a2, a3 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2], a[3] = a0, a1, a2, a3
	return callPooled(fn, a, 4)
}

//go:uintptrescapes
func call5(fn, a0, a1, a2, a3, a4 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2], a[3], a[4] = a0, a1, a2, a3, a4
	return callPooled(fn, a, 5)
}

//go:uintptrescapes
func call6(fn, a0, a1, a2, a3, a4, a5 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2], a[3], a[4], a[5] = a0, a1, a2, a3, a4, a5
	return callPooled(fn, a, 6)
}

//go:uintptrescapes
func call7(fn, a0, a1, a2, a3, a4, a5, a6 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2], a[3], a[4], a[5], a[6] = a0, a1, a2, a3, a4, a5, a6
	return callPooled(fn, a, 7)
}

//go:uintptrescapes
func call8(fn, a0, a1, a2, a3, a4, a5, a6, a7 uintptr) uintptr {
	a := callArgs.Get().(*[maxDirectArgs]uintptr)
	a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7] = a0, a1, a2, a3, a4, a5, a6, a7
	return callPooled(fn, a, 8)
}

// noFloatCalls panics in a callFN that a correct caller never reaches,
// because floatCalls is false.
func noFloatCalls() (uintptr, uint64) {
	panic("sdl: floating-point direct calls are not supported on this platform")
}

func callF0(fn uintptr, fmask uint16, f *floatArgs) (uintptr, uint64) { return noFloatCalls() }

func callF1(fn uintptr, fmask uint16, f *floatArgs, a0 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF2(fn uintptr, fmask uint16, f *floatArgs, a0, a1 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF3(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF4(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF5(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF6(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF7(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}

func callF8(fn uintptr, fmask uint16, f *floatArgs, a0, a1, a2, a3, a4, a5, a6, a7 uintptr) (uintptr, uint64) {
	return noFloatCalls()
}
