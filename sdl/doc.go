// Package sdl is a pure Go binding for SDL 3 built on purego: it loads the
// SDL3 shared library at run time, needs no cgo and no C toolchain, and
// cross-compiles like any other Go package.
//
// # Loading
//
// The library is located when the package is imported (SDL3_LIBRARY, then the
// platform defaults). Set [AutoLoadDisabledEnv] to "1" to skip this search and
// call [Load] explicitly. A missing library does not panic at import time;
// check [LoadError], or call [Load] with an explicit path. Functions that the
// installed SDL3 does not export panic with a descriptive message when called;
// [Available] tests for a symbol.
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
//   - const char * parameters are Go strings. A string with a trailing NUL is
//     passed without a temporary copy; cache NUL-terminated names used in a
//     hot loop to avoid an allocation on each call. Where the documentation says
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
// on the process main thread. SDL error state belongs to the current OS
// thread. If a goroutine may move between OS threads, keep it locked with
// runtime.LockOSThread across both a failing SDL call and the following
// [GetError] call.
//
// # Calls
//
// Functions whose parameters and result are each one integer word (integers,
// enums, bool, pointers) or one float or double, with at most eight integer
// and eight floating-point parameters, are direct calls that do not allocate.
// On darwin, linux and windows for amd64 and arm64 they enter C through a small
// assembly trampoline and runtime.cgocall, without cgo, at roughly a third of
// the cost of the reflective wrappers of purego.RegisterFunc. Elsewhere
// integer signatures go through purego.SyscallN and floating-point ones use
// purego.RegisterFunc. The remaining functions (structs passed by value,
// C variadic functions, more than eight integer parameters) use
// purego.RegisterFunc everywhere. Go memory passed by pointer is moved to the
// heap and kept alive for the duration of the call in every case.
//
// # Generation
//
// The *_gen.go files are generated from the headers of the SDL release pinned
// in the SDL_VERSION file. cmd/sdlsync downloads those headers and runs the
// generator (cmd/gensdl, built on modernc.org/cc/v4 and github.com/dave/jennifer);
// see the repository README.
//
// Never edit a *_gen.go file by hand: the next generation overwrites it and
// `sdlsync -check` (run in CI) fails on any difference. To change generated
// code, change the rules in internal/bindgen (preferred), or, when a rule
// cannot express the change, register a Patch in internal/gensdl, which is
// applied to the generated files at the end of every run.
//
// The files without the _gen suffix are hand-written and are the only place
// for code that cannot be derived from the headers:
//
//   - doc.go: this package documentation.
//   - loader.go, loader_unix.go, loader_windows.go: locating and loading
//     libSDL3 and binding its symbols.
//   - call.go, call_trampoline.go, call_regclass.go, call_windows_amd64.go,
//     call_trampoline_*.s, call_syscall.go: the allocation-free
//     call0 ... call8 and callF0 ... callF8 bridge that most generated
//     functions use instead of purego.RegisterFunc (see below).
//   - macros.go: Go versions of SDL function-like macros and static inline
//     functions, which the shared library does not export.
//   - slices.go: helpers behind the generated wrappers that return
//     SDL_free-able arrays, plus [GoStrings], [GoStringsN] and [Slice].
//   - wchar.go, wchar_windows.go, wchar_common.go: the platform-dependent
//     [Wchar] type and wide-string conversion.
//   - guid.go: the scalar-field ABI form of [GUID] used at the C boundary.
//   - extra.go: pure Go helpers such as [Ptr], the UTF-8 stepping functions
//     and the user-data handle table.
package sdl

//go:generate go run ../cmd/sdlsync -root .. -generate
