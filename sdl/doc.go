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
// [Load] with a non-empty path also switches an already loaded library to
// that path (see [Load] for the rules) and [LoadedPath] reports which library
// is bound. The companion module github.com/opengoui/libsdl embeds SDL3
// builds and uses this to load them from a directory of the caller's choice.
//
// # Conventions
//
// The SDL_ prefix is dropped: SDL_CreateWindow is [CreateWindow], SDL_Window
// is [Window] and SDL_INIT_VIDEO is [INIT_VIDEO]. C bool, integer, float and
// enum types map to the Go types of the same size. Pointers to C data are Go
// pointers; opaque handles are pointers to empty structs.
//
//   - const char * parameters are Go strings. Where the documentation says
//     NULL is allowed and NULL only selects a default, the empty string is
//     passed as NULL. Where NULL means something different from "" (deleting
//     a property in [SetStringProperty], [SetAppMetadataProperty],
//     [SetGamepadMapping]; a separator in [InsertTrayEntryAt]; the default root in
//     [OpenTitleStorage]) the parameter
//     is a *string: nil is NULL and [Ptr]("") is an empty string.
//   - const char * results are copied into Go strings and NULL becomes "";
//     results the caller must release with SDL_free are copied and freed.
//     Where NULL means "not set" ([GetHint], [GetEnvironmentVariable],
//     [Getenv], [GetAppMetadataProperty]) a sibling with an OK suffix
//     ([GetHintOK] ...) returns (string, bool).
//   - functions that return an SDL_free-able array ([GetDisplays],
//     [GetWindows], [LoadFile] ...) return a Go slice and release the SDL
//     allocation; the count out-parameter is dropped from the signature.
//   - [GoStrings], [GoStringsN] and [Slice] help with arrays that SDL hands out
//     and that have no generated wrapper; the first two copy, [Slice] aliases
//     the C memory.
//   - Functions whose SDL documentation says that a pointer argument is
//     retained rather than copied carry a "Keep alive:" note: the caller must
//     keep that Go memory reachable while SDL uses it.
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
// The *_gen.go files are generated from the headers of the SDL release pinned
// in the SDL_VERSION file. cmd/sdlsync downloads those headers and runs the
// generator (cmd/gensdl, built on modernc.org/cc/v4 and github.com/dave/jennifer);
// see the repository README.
package sdl

//go:generate go run ../cmd/sdlsync -root .. -generate
