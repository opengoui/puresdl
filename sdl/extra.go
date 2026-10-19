package sdl

import (
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"
)

// Ptr returns a pointer to a copy of v. It is the way to pass a non-nil value
// for the *string parameters that distinguish nil (C NULL) from an empty
// string, for example SetStringProperty(props, name, Ptr("value")).
func Ptr[T any](v T) *T { return &v }

// LocaleInfo is a preferred locale: a language and an optional country.
// Country is empty when the locale is generic (SDL NULL).
type LocaleInfo struct {
	Language string
	Country  string
}

// GetPreferredLocales reports the user's preferred locales in order of
// preference, or nil on failure.
//
// SDL returns the locale structs and their strings in one allocation, so the
// strings are copied before that allocation is released.
//
// Since: SDL 3.2.0.
func GetPreferredLocales() []LocaleInfo {
	var n int32
	p := fnGetPreferredLocales(&n)
	if p == nil {
		return nil
	}
	defer Free(unsafe.Pointer(p))
	src := unsafe.Slice(p, int(n))
	out := make([]LocaleInfo, 0, n)
	for _, l := range src {
		if l == nil {
			break
		}
		out = append(out, LocaleInfo{GoString(l.Language), GoString(l.Country)})
	}
	return out
}

// scancodeNames holds the SDL-allocated name strings installed by
// [SetScancodeName], keyed by scancode, guarded by scancodeNamesMu.
var (
	scancodeNamesMu sync.Mutex
	scancodeNames   = map[Scancode]*byte{}
)

// SetScancodeName sets a human-readable name for a scancode. An empty name
// restores the default name.
//
// SDL does not copy the name, so the wrapper stores it in SDL-allocated
// memory that stays valid until the name is replaced; nothing needs to be
// kept alive by the caller.
//
// Thread safety: This function is not thread safe.
//
// Since: SDL 3.2.0.
//
// See also: GetScancodeName
func SetScancodeName(scancode Scancode, name string) bool {
	scancodeNamesMu.Lock()
	defer scancodeNamesMu.Unlock()
	var p *byte
	if name != "" {
		m := (*byte)(Malloc(uint(len(name) + 1)))
		if m == nil {
			return false
		}
		b := unsafe.Slice(m, len(name)+1)
		copy(b, name)
		b[len(name)] = 0
		p = m
	}
	if !fnSetScancodeName(scancode, p) {
		if p != nil {
			Free(unsafe.Pointer(p))
		}
		return false
	}
	if old := scancodeNames[scancode]; old != nil {
		Free(unsafe.Pointer(old))
	}
	if p == nil {
		delete(scancodeNames, scancode)
	} else {
		scancodeNames[scancode] = p
	}
	return true
}

// IconvString converts the bytes of inbuf from the character encoding
// fromcode to tocode and returns the converted bytes, or nil on error.
//
// SDL reports no output length, so the result ends before the first
// all-zero code unit of the target encoding (1 byte, or 2 for UTF-16 and
// UCS-2, or 4 for UTF-32 and UCS-4); the terminator is not included. Only
// len(inbuf) bytes are read; include the terminating NUL in inbuf to stop the
// conversion there.
//
// Since: SDL 3.2.0.
//
// See also: IconvOpen, IconvClose, Iconv
func IconvString(tocode string, fromcode string, inbuf []byte) []byte {
	in := make([]byte, len(inbuf)+1)
	copy(in, inbuf)
	p := fnIconvString(cstr(tocode), cstr(fromcode), &in[0], uint(len(inbuf)))
	if p == nil {
		return nil
	}
	defer Free(unsafe.Pointer(p))
	w := 1
	switch up := strings.ToUpper(tocode); {
	case strings.Contains(up, "UCS-4") || strings.Contains(up, "UCS4") || strings.Contains(up, "UTF-32") || strings.Contains(up, "UTF32"):
		w = 4
	case strings.Contains(up, "UCS-2") || strings.Contains(up, "UCS2") || strings.Contains(up, "UTF-16") || strings.Contains(up, "UTF16"):
		w = 2
	}
	n := 0
	for {
		unit := unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(p), n)), w)
		zero := true
		for _, c := range unit {
			if c != 0 {
				zero = false
			}
		}
		if zero {
			break
		}
		n += w
	}
	return append([]byte{}, unsafe.Slice(p, n)...)
}

// userdataHandles maps a token address (a 1-byte SDL allocation) to its value.
var userdataHandles sync.Map // uintptr -> any

// NewUserdataHandle stores v and returns an opaque non-nil token that can be
// passed as the userdata argument of ReadAsyncIO, WriteAsyncIO, CloseAsyncIO,
// VirtualJoystickDesc.Userdata and similar. SDL keeps userdata past the call,
// so passing a pointer to Go-managed memory is unsafe.
//
// The token is a unique address in the C heap (a 1-byte SDL allocation). It is
// never dereferenced, and being a real non-Go address it is safe to hold in Go
// stack frames and struct fields. Retrieve v with UserdataHandleValue and
// release the token with FreeUserdataHandle once SDL no longer reports it.
// It requires libSDL3 to be loaded and panics if the allocation fails.
func NewUserdataHandle(v any) unsafe.Pointer {
	m := Malloc(1)
	if m == nil {
		panic("sdl: NewUserdataHandle: SDL_malloc failed")
	}
	userdataHandles.Store(uintptr(m), v)
	return m
}

// UserdataHandleValue returns the value stored for a token from
// NewUserdataHandle, or nil when the token is unknown or already freed.
func UserdataHandleValue(p unsafe.Pointer) any {
	v, _ := userdataHandles.Load(uintptr(p))
	return v
}

// FreeUserdataHandle releases a token from NewUserdataHandle and its C
// allocation. Freeing an unknown or already freed token is a no-op.
func FreeUserdataHandle(p unsafe.Pointer) {
	if _, ok := userdataHandles.LoadAndDelete(uintptr(p)); ok {
		Free(p)
	}
}

// StepUTF8 decodes the first UTF-8 sequence of s and returns the code point
// and the number of bytes consumed. An invalid sequence yields
// INVALID_UNICODE_CODEPOINT and advances by one byte; an empty string yields
// (0, 0). It is a pure-Go counterpart of SDL_StepUTF8.
func StepUTF8(s string) (r rune, size int) {
	if s == "" {
		return 0, 0
	}
	r, size = utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		return INVALID_UNICODE_CODEPOINT, 1
	}
	return r, size
}

// StepBackUTF8 decodes the last UTF-8 sequence of s and returns the code point
// and the number of bytes it occupies, so s[:len(s)-size] is the remainder. An
// invalid trailing sequence yields INVALID_UNICODE_CODEPOINT with size 1; an
// empty string yields (0, 0). It is a pure-Go counterpart of SDL_StepBackUTF8.
func StepBackUTF8(s string) (r rune, size int) {
	if s == "" {
		return 0, 0
	}
	r, size = utf8.DecodeLastRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		return INVALID_UNICODE_CODEPOINT, 1
	}
	return r, size
}

// UCS4ToUTF8 returns the UTF-8 encoding of the code point r. A negative value,
// a surrogate or a value above U+10FFFF encodes INVALID_UNICODE_CODEPOINT. It
// is a pure-Go counterpart of SDL_UCS4ToUTF8.
func UCS4ToUTF8(r rune) string {
	if !utf8.ValidRune(r) {
		r = INVALID_UNICODE_CODEPOINT
	}
	return string(r)
}
