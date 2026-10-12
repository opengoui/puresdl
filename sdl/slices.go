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
	src := unsafe.Slice(p, n)
	s := make([]string, n)
	for i, e := range src {
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
	var s []string
	for q := p; *q != nil; q = (**byte)(unsafe.Add(unsafe.Pointer(q), unsafe.Sizeof(q))) {
		s = append(s, GoString(*q))
	}
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
