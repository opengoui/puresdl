package sdl

import "unsafe"

// LocaleInfo is a preferred locale: a language and an optional country.
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
