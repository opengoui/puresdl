//go:build windows

package sdl

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/ebitengine/purego"
)

func libraryCandidates() []string {
	// LoadLibrary already searches the executable's directory and PATH; the
	// explicit executable-relative path makes that location win regardless of
	// the process working directory or any SetDllDirectory call.
	var c []string
	if exe, err := os.Executable(); err == nil {
		c = append(c, filepath.Join(filepath.Dir(exe), "SDL3.dll"))
	}
	return append(c, "SDL3.dll")
}

func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

func lookup(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}

func bindFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }
