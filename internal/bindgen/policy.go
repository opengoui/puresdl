package bindgen

import (
	"math/big"
	"regexp"
	"strings"

	"github.com/opengoui/puresdl/internal/ir"
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

// constTypeOverrides pins the type of macro constants whose contiguous
// #define block does not name the right typedef. The value is the C typedef
// name; "" keeps the constant untyped. Entries were checked against the SDL
// 3.5 headers.
var constTypeOverrides = map[string]string{
	// Positions, not window flags: they are passed as int coordinates.
	"SDL_WINDOWPOS_UNDEFINED_MASK": "",
	"SDL_WINDOWPOS_UNDEFINED":      "",
	"SDL_WINDOWPOS_CENTERED_MASK":  "",
	"SDL_WINDOWPOS_CENTERED":       "",
	// Sentinel IDs follow the ID typedef they are cast to in the headers,
	// not the (SDL_PenID, SDL_Finger) declaration that precedes them.
	"SDL_PEN_MOUSEID":   "SDL_MouseID",
	"SDL_PEN_TOUCHID":   "SDL_TouchID",
	"SDL_TOUCH_MOUSEID": "SDL_MouseID",
	"SDL_MOUSE_TOUCHID": "SDL_TouchID",
	// SDL_HAPTIC_INFINITY is an effect length, not part of SDL_Haptic.
	"SDL_HAPTIC_INFINITY": "",
	// Button numbers and masks share SDL_MouseButtonFlags in the headers.
	"SDL_BUTTON_LEFT":   "SDL_MouseButtonFlags",
	"SDL_BUTTON_MIDDLE": "SDL_MouseButtonFlags",
	"SDL_BUTTON_RIGHT":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_X1":     "SDL_MouseButtonFlags",
	"SDL_BUTTON_X2":     "SDL_MouseButtonFlags",
	"SDL_BUTTON_LMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_MMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_RMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_X1MASK": "SDL_MouseButtonFlags",
	"SDL_BUTTON_X2MASK": "SDL_MouseButtonFlags",
}

// constType returns the Go type name for an integer macro, or "" when it
// stays untyped. A macro takes the integer typedef declared immediately
// before its #define block (ir.Const.After) unless overridden, and only
// when that typedef is emitted as a named Go type that can hold the value.
func (g *gen) constType(k *ir.Const) string {
	cName := k.After
	if o, ok := constTypeOverrides[k.Name]; ok {
		cName = o
	}
	if cName == "" {
		return ""
	}
	n, ok := g.tdGo[cName]
	if !ok {
		return ""
	}
	td := g.typedefs[cName]
	res := td.Type.Under.Resolve()
	if res == nil || res.Kind != ir.Int || res.Char {
		return ""
	}
	if !fitsInt(k.Int, res.Size, res.Signed) {
		return ""
	}
	return n
}

// fitsInt reports whether v is representable in an integer of size bytes.
func fitsInt(v *big.Int, size int64, signed bool) bool {
	bitsN := uint(size * 8)
	if signed {
		lim := new(big.Int).Lsh(big.NewInt(1), bitsN-1)
		return v.Cmp(new(big.Int).Neg(lim)) >= 0 && v.Cmp(lim) < 0
	}
	return v.Sign() >= 0 && v.BitLen() <= int(bitsN)
}
