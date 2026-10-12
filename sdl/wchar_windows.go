//go:build windows

package sdl

import "unicode/utf16"

// Wchar is the Go type of the C wchar_t: a 16-bit UTF-16 code unit on
// Windows, and a 32-bit UTF-32 code unit on every other platform.
type Wchar = uint16

func wcharDecode(u []Wchar) string { return string(utf16.Decode(u)) }

func wcharEncode(s string) []Wchar { return utf16.Encode([]rune(s)) }
