//go:build ((darwin || linux) && (amd64 || arm64)) || (windows && arm64)

package sdl

// fillFrame lays out the arguments for ABIs that assign integer and
// floating-point arguments to separate register files (System V AMD64 and
// AAPCS64, including Windows on Arm for non-variadic functions): the first
// eight words are the integer arguments and the next eight the
// floating-point arguments, each in parameter order, so fmask is not needed.
func fillFrame(fr *callFrame, a *[maxDirectArgs]uintptr, f *floatArgs, fmask uint16) {
	copy(fr.a[:maxDirectArgs], a[:])
	for i, v := range f {
		fr.a[maxDirectArgs+i] = uintptr(v)
	}
}
