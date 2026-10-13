//go:build windows

package sdl

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/ebitengine/purego"
)

// libraryCandidates returns the SDL3.dll locations tried, in order: next to
// the executable, then the default Windows DLL search.
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

// openLibrary loads the DLL at path and returns its module handle.
func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

// lookup returns the address of the exported symbol name in the module h.
func lookup(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}

// bindFunc binds the C function at addr to the Go function variable fptr.
func bindFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }
