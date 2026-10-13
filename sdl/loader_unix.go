//go:build !windows

package sdl

import (
	"runtime"

	"github.com/ebitengine/purego"
)

// libraryCandidates returns the library names and paths tried, in order, when
// no explicit path is given: bare names first so the dynamic linker's search
// rules apply, then well-known install locations.
func libraryCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"libSDL3.dylib",
			"libSDL3.0.dylib",
			// Homebrew prefixes: /opt/homebrew on arm64, /usr/local on amd64.
			"/opt/homebrew/lib/libSDL3.dylib",
			"/usr/local/lib/libSDL3.dylib",
			"/opt/local/lib/libSDL3.dylib",
			"SDL3.framework/SDL3",
			"/Library/Frameworks/SDL3.framework/SDL3",
		}
	default:
		// Bare names go through the dynamic linker's cache and RPATH; the
		// absolute paths cover systems whose cache does not list the
		// distribution's multiarch directory.
		c := []string{"libSDL3.so.0", "libSDL3.so"}
		for _, dir := range linuxLibDirs() {
			c = append(c, dir+"/libSDL3.so.0", dir+"/libSDL3.so")
		}
		return c
	}
}

// linuxLibDirs lists the usual library directories for the target
// architecture, Debian/Ubuntu multiarch first.
func linuxLibDirs() []string {
	var dirs []string
	switch runtime.GOARCH {
	case "amd64":
		dirs = append(dirs, "/usr/lib/x86_64-linux-gnu", "/usr/lib64")
	case "arm64":
		dirs = append(dirs, "/usr/lib/aarch64-linux-gnu", "/usr/lib64")
	case "arm":
		dirs = append(dirs, "/usr/lib/arm-linux-gnueabihf")
	}
	return append(dirs, "/usr/local/lib", "/usr/local/lib64", "/usr/lib")
}

// openLibrary dlopens path with RTLD_NOW|RTLD_GLOBAL and returns the handle.
func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

// lookup returns the address of the symbol name in the library h.
func lookup(h uintptr, name string) (uintptr, error) {
	return purego.Dlsym(h, name)
}

// bindFunc binds the C function at addr to the Go function variable fptr.
func bindFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }
