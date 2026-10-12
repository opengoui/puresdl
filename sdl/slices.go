package sdl

import "unsafe"

// The helpers below back the generated wrappers of functions that return an
// SDL_free-able array: they copy the array into Go memory and release the
// SDL allocation.

// copyFree copies n elements at p into a new slice and frees p.
func copyFree[E any](p *E, n int) []E {
	if p == nil {
		return nil
	}
	s := append([]E(nil), unsafe.Slice(p, n)...)
	Free(unsafe.Pointer(p))
	return s
}

// copyFreeDeref copies the n structs that the pointers at p refer to. SDL
// allocates the array and the structs in one block, so everything is copied
// before the block is freed.
func copyFreeDeref[E any](p **E, n int) []E {
	if p == nil {
		return nil
	}
	src := unsafe.Slice(p, n)
	s := make([]E, n)
	for i, e := range src {
		s[i] = *e
	}
	Free(unsafe.Pointer(p))
	return s
}

// copyFreeStrings copies n C strings at p and frees the array.
func copyFreeStrings(p **byte, n int) []string {
	if p == nil {
		return nil
	}
	s := make([]string, n)
	for i, e := range unsafe.Slice(p, n) {
		s[i] = GoString(e)
	}
	Free(unsafe.Pointer(p))
	return s
}

// copyFreeStrings0 is copyFreeStrings for a NULL-terminated array.
func copyFreeStrings0(p **byte) []string {
	if p == nil {
		return nil
	}
	s := GoStrings(p)
	Free(unsafe.Pointer(p))
	return s
}

// copyFreeBytes copies n bytes at p and frees them.
func copyFreeBytes(p unsafe.Pointer, n int) []byte {
	if p == nil {
		return nil
	}
	s := append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
	Free(p)
	return s
}

// GoStrings copies the C strings of the NULL-terminated array at p into Go
// strings. It returns nil for a nil p and does not free the array, which
// stays owned by whoever allocated it.
func GoStrings(p **byte) []string {
	if p == nil {
		return nil
	}
	var s []string
	for q := p; *q != nil; q = (**byte)(unsafe.Add(unsafe.Pointer(q), unsafe.Sizeof(q))) {
		s = append(s, GoString(*q))
	}
	return s
}

// GoStringsN copies the n C strings of the array at p into Go strings. It
// returns nil for a nil p or n <= 0 and does not free the array. A nil
// element becomes the empty string.
func GoStringsN(p **byte, n int) []string {
	if p == nil || n <= 0 {
		return nil
	}
	s := make([]string, n)
	for i, e := range unsafe.Slice(p, n) {
		s[i] = GoString(e)
	}
	return s
}

// Slice views the n elements at p as a Go slice without copying them. It
// returns nil for a nil p or n <= 0. The slice aliases C memory, so it must
// not be used after SDL frees or reuses that memory; copy it first
// (for example with slices.Clone) when it has to outlive the call that
// produced it.
func Slice[T any](p *T, n int) []T {
	if p == nil || n <= 0 {
		return nil
	}
	return unsafe.Slice(p, n)
}
