package bindgen

import (
	"regexp"
	"strings"
)

// keptStdinc are the SDL_stdinc.h functions that have no Go standard library
// equivalent and are therefore bound. Every other function declared there is
// a libc replacement (string, math, memory, wide-char helpers) that Go
// programs should not call through SDL.
var keptStdinc = regexp.MustCompile(`^SDL_(` +
	`malloc|calloc|realloc|free|aligned_alloc|aligned_free|GetOriginalMemoryFunctions|GetMemoryFunctions|SetMemoryFunctions|GetNumAllocations|` +
	`CreateEnvironment|GetEnvironment|GetEnvironmentVariable|GetEnvironmentVariables|SetEnvironmentVariable|UnsetEnvironmentVariable|DestroyEnvironment|` +
	`getenv|getenv_unsafe|setenv_unsafe|unsetenv_unsafe|` +
	`srand|rand|randf|rand_bits|rand_r|randf_r|rand_bits_r|murmur3_32|crc16|crc32|` +
	`iconv_open|iconv_close|iconv|iconv_string|` +
	`GetNumAllocations|` +
	`ReportAssertion|GetAssertionReport|ResetAssertionReport|SetAssertionHandler|GetDefaultAssertionHandler|GetAssertionHandler` +
	`)$`)

// nullableRe recognises a \param description that allows NULL.
var nullableRe = regexp.MustCompile(`\bNULL\b`)

// skipReason reports why a function is not bound, or "" to bind it.
func skipReason(name, file string, hasVaList bool, variadicOK bool) string {
	switch {
	case hasVaList:
		return "takes a va_list"
	case file == "SDL_stdinc.h" && !keptStdinc.MatchString(name):
		return "libc replacement"
	case strings.HasPrefix(name, "SDL_hid_") && false:
		return ""
	}
	if !variadicOK {
		return "C variadic function"
	}
	return ""
}

// freedStringRe recognises documentation that tells the caller to release a
// returned string with SDL_free.
var freedStringRe = regexp.MustCompile(`SDL_free\b`)
