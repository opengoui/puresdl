//go:build !windows

package sdl

// Wchar is the Go type of the C wchar_t: a 32-bit UTF-32 code unit on every
// platform except Windows, where it is a 16-bit UTF-16 code unit.
type Wchar = int32

// wcharDecode converts NUL-free wchar_t units to a string.
func wcharDecode(u []Wchar) string {
	r := make([]rune, len(u))
	for i, c := range u {
		r[i] = rune(c)
	}
	return string(r)
}

// wcharEncode converts s to wchar_t units without a terminator.
func wcharEncode(s string) []Wchar {
	rs := []rune(s)
	u := make([]Wchar, len(rs))
	for i, r := range rs {
		u[i] = Wchar(r)
	}
	return u
}
