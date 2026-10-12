//go:build !windows

package sdl

import (
	"runtime"

	"github.com/ebitengine/purego"
)

func libraryCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"libSDL3.dylib",
			"libSDL3.0.dylib",
			"/opt/homebrew/lib/libSDL3.dylib",
			"/usr/local/lib/libSDL3.dylib",
			"/opt/local/lib/libSDL3.dylib",
			"SDL3.framework/SDL3",
			"/Library/Frameworks/SDL3.framework/SDL3",
		}
	default:
		return []string{"libSDL3.so.0", "libSDL3.so"}
	}
}

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func lookup(h uintptr, name string) (uintptr, error) {
	return purego.Dlsym(h, name)
}

func bindFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }
