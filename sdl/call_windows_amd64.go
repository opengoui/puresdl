package sdl

// fillFrame lays out the arguments for the Microsoft x64 ABI, which assigns
// argument slots by position: slot i is RCX, RDX, R8 or R9 (or XMM0 to XMM3
// for a floating-point argument) for i < 4 and a stack slot otherwise. The
// trampoline loads each of the first four slots into both the integer and
// the floating-point register, so the callee finds the value in whichever it
// reads.
func fillFrame(fr *callFrame, a *[maxDirectArgs]uintptr, f *floatArgs, fmask uint16) {
	fr.a = positionalArgs(a, f, fmask)
}
