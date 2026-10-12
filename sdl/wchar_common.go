package sdl

import "unsafe"

// WcharString decodes the NUL-terminated wide string at p (UTF-16 on Windows,
// UTF-32 elsewhere) into a Go string. A nil p yields "".
func WcharString(p *Wchar) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*Wchar)(unsafe.Add(unsafe.Pointer(p), uintptr(n)*unsafe.Sizeof(*p))) != 0 {
		n++
	}
	return wcharDecode(unsafe.Slice(p, n))
}

// WcharCString returns s as a NUL-terminated wide string. Pass &result[0] to
// C; the slice must stay reachable for the duration of the call.
func WcharCString(s string) []Wchar { return append(wcharEncode(s), 0) }

// wcstrNull is like WcharCString but returns a pointer, and NULL for "". It
// is used by generated wrappers; the backing array is kept alive by the
// returned pointer.
func wcstrNull(s string) *Wchar {
	if s == "" {
		return nil
	}
	return &WcharCString(s)[0]
}
