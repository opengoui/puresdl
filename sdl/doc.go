// Package sdl is a pure Go binding for SDL 3 built on purego: it loads the
// SDL3 shared library at run time, needs no cgo and no C toolchain, and
// cross-compiles like any other Go package.
//
// # Loading
//
// The library is located when the package is imported (SDL3_LIBRARY, then the
// platform defaults). A missing library does not panic at import time; check
// [LoadError], or call [Load] with an explicit path. Functions that the
// installed SDL3 does not export panic with a descriptive message when
// called; [Available] tests for a symbol.
//
// # Conventions
//
// The SDL_ prefix is dropped: SDL_CreateWindow is [CreateWindow], SDL_Window
// is [Window] and SDL_INIT_VIDEO is [INIT_VIDEO]. C bool, integer, float and
// enum types map to the Go types of the same size. Pointers to C data are Go
// pointers; opaque handles are pointers to empty structs.
//
//   - const char * parameters are Go strings. Where the documentation says
//     NULL is allowed, the empty string is passed as NULL.
//   - const char * results are copied into Go strings; results the caller
//     must release with SDL_free are copied and freed.
//   - printf-style functions ([SetError], [Log] ...) take a Go format string
//     and arguments, formatted with fmt.Sprintf before SDL sees them.
//   - C unions such as [Event] expose their members through accessor methods.
//   - Callback typedefs are uintptr-based types created from Go functions with
//     the matching New... constructor ([NewEventFilter]). purego allows about
//     2000 callbacks per process and never frees them, so create each once.
//
// SDL requires most video and event functions to run on the main thread. On
// macOS call runtime.LockOSThread in an init function so that main.main runs
// on the process main thread.
//
// # Generation
//
// The *_gen.go files are generated from the SDL headers by cmd/gensdl using
// modernc.org/cc/v4 and github.com/dave/jennifer; see the repository README.
package sdl

//go:generate go run ../cmd/gensdl -include ../../SDL/include -out . -skipped ../docs/skipped.txt
