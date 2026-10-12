package sdl

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

// LibraryEnv is the environment variable that overrides the SDL3 shared
// library path searched by [Load].
const LibraryEnv = "SDL3_LIBRARY"

var (
	loadMu  sync.Mutex
	loadErr error
	loaded  bool
	handle  uintptr
	missing = map[string]struct{}{}
)

// ErrNotLoaded is returned by [LoadError] when the shared library has not
// been loaded yet.
var ErrNotLoaded = errors.New("sdl: library not loaded")

// ErrUnsupportedArch is reported by [LoadError] on 32-bit targets, where
// purego cannot pass structs by value. The package still compiles there.
var ErrUnsupportedArch = errors.New("sdl: 32-bit targets are not supported")

func init() {
	// A failure here is not fatal: it is recorded and reported by LoadError
	// so that importing the package never panics on machines without SDL3.
	_ = Load("")
}

// Load locates and binds the SDL3 shared library. An empty path searches
// the SDL3_LIBRARY environment variable followed by the platform defaults.
// It runs once automatically when the package is imported; call it again
// with an explicit path only if the automatic load failed.
func Load(path string) error {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loaded {
		return nil
	}
	if unsafe.Sizeof(uintptr(0)) != 8 {
		// purego cannot pass structs by value on 32-bit targets, which many
		// SDL functions need, so binding would panic. Fail cleanly instead.
		loadErr = ErrUnsupportedArch
		return loadErr
	}
	candidates := libraryCandidates()
	if path != "" {
		candidates = []string{path}
	} else if env := os.Getenv(LibraryEnv); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	var errs []error
	for _, c := range candidates {
		h, err := openLibrary(c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		handle = h
		clear(missing)
		bindAll()
		loaded, loadErr = true, nil
		return nil
	}
	loadErr = fmt.Errorf("sdl: cannot load SDL3: %w", errors.Join(errs...))
	return loadErr
}

// LoadError reports why the shared library could not be loaded, or nil if
// it was loaded successfully.
func LoadError() error {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loaded {
		return nil
	}
	if loadErr == nil {
		return ErrNotLoaded
	}
	return loadErr
}

// Available reports whether the named C symbol (for example "SDL_Init")
// exists in the loaded library. Functions introduced by newer SDL3 releases
// than the one installed are absent and panic when called.
func Available(symbol string) bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	if !loaded {
		return false
	}
	if _, gone := missing[symbol]; gone {
		return false
	}
	_, err := lookup(handle, symbol)
	return err == nil
}

// register binds the C function symbol to the Go function variable fptr.
// A missing symbol installs a stub that panics with a descriptive message
// when called, so older SDL3 releases still work for the APIs they have.
func register(fptr any, symbol string) {
	addr, err := lookup(handle, symbol)
	if err != nil || addr == 0 {
		missing[symbol] = struct{}{}
		fn := reflect.ValueOf(fptr).Elem()
		fn.Set(reflect.MakeFunc(fn.Type(), func([]reflect.Value) []reflect.Value {
			panic("sdl: " + symbol + " is not available in the loaded SDL3 library")
		}))
		return
	}
	bindFunc(fptr, addr)
}

// CString returns a NUL-terminated copy of s suitable for parameters that
// accept NULL (declared as *byte). The result is only valid while the
// returned pointer is reachable from Go.
func CString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// GoString copies the NUL-terminated C string at p into a Go string. It
// returns "" for a nil pointer.
func GoString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// cstr returns a NUL-terminated copy of s.
func cstr(s string) *byte { return CString(s) }

// cstrNull is like cstr but maps the empty string to NULL.
func cstrNull(s string) *byte {
	if s == "" {
		return nil
	}
	return CString(s)
}

// cstrPtr returns a NUL-terminated copy of *s, or NULL when s is nil. Unlike
// cstrNull it keeps the empty string distinct from NULL.
func cstrPtr(s *string) *byte {
	if s == nil {
		return nil
	}
	return CString(*s)
}

// cprintf formats a printf-style message and escapes it so that SDL's own
// format processing prints it verbatim. No C varargs are involved.
func cprintf(format string, args []any) *byte {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return CString(strings.ReplaceAll(msg, "%", "%%"))
}
