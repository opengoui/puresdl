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
	`iconv_open|iconv_close|iconv|iconv_string` +
	`)$`)

// skipTypes are helper declarations of the headers that are not API.
var skipTypes = map[string]bool{"SDL_alignment_test": true, "SDL_DUMMY_ENUM": true}

var nullRe = regexp.MustCompile(`\bNULL\b`)

// nullable reports whether a \param description allows NULL, ignoring
// "must not be NULL" and "NULL-terminated".
func nullable(doc string) bool {
	for _, m := range nullRe.FindAllStringIndex(doc, -1) {
		before := strings.ToLower(doc[max(0, m[0]-16):m[0]])
		after := doc[m[1]:min(len(doc), m[1]+12)]
		if strings.Contains(before, "not ") || strings.Contains(before, "non-") || strings.HasPrefix(after, "-terminated") {
			continue
		}
		return true
	}
	return false
}

// freedStringRe recognises documentation that tells the caller to release a
// returned allocation with SDL_free.
var freedStringRe = regexp.MustCompile(`SDL_free\b`)

// paramTypeOverrides replace the Go type of C parameters that are declared as
// plain integers although they carry an enum value.
var paramTypeOverrides = map[string]string{
	"EventEnabled.type": "EventType", "HasEvent.type": "EventType", "HasEvents.minType": "EventType", "HasEvents.maxType": "EventType", "SetEventEnabled.type": "EventType", "FlushEvent.type": "EventType",
	"FlushEvents.minType": "EventType", "FlushEvents.maxType": "EventType",
	"PeepEvents.minType": "EventType", "PeepEvents.maxType": "EventType",
}

// fieldTypeOverrides do the same for struct and union members.
var fieldTypeOverrides = map[string]string{"Event.type": "EventType", "CommonEvent.type": "EventType"}

// initialisms are spelled in capitals when they start a lower-case C name.
var initialisms = map[string]string{"hid": "HID", "id": "ID", "io": "IO", "utf": "UTF", "crc": "CRC"}

// handWritten functions get only their raw function variable (fn<Name>); the
// exported wrapper lives in a hand-written file because a generic rule would
// be wrong for them.
var handWritten = map[string]bool{"SDL_GetPreferredLocales": true}
