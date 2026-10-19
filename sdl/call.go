package sdl

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The generated direct-call functions (fn<Name> declared as functions, not
// variables) reach C through call0 ... call8 instead of the reflective
// wrappers that purego.RegisterFunc installs. The generator uses them only for
// signatures whose parameters and result each fit in one integer register:
// integers, enums, bool and pointers, with at most eight parameters so that
// every argument is passed in a register on all supported ABIs (Apple arm64
// packs stack arguments by their natural size, which SyscallN does not).
// Floating-point parameters or results and structs passed by value stay on
// purego.RegisterFunc.
//
// Each callN is marked go:uintptrescapes: an argument written as
// uintptr(unsafe.Pointer(p)) in the call expression moves the object p points
// to onto the heap and keeps it alive until callN returns, which is the rule
// purego.SyscallN itself relies on. The arguments are then copied into a
// pooled array, so a call does not allocate.

// maxDirectArgs is the largest parameter count of a direct-call function.
const maxDirectArgs = 8

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
