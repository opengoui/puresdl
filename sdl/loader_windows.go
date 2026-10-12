//go:build windows

package sdl

import (
	"syscall"

	"github.com/ebitengine/purego"
)

func libraryCandidates() []string { return []string{"SDL3.dll"} }

func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

func lookup(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}

func bindFunc(fptr any, addr uintptr) { purego.RegisterFunc(fptr, addr) }
