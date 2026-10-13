package sdl

import "unsafe"

// guidABI has the C layout of [GUID] but is made of scalar fields, which
// purego passes and returns by value correctly on every supported
// architecture (it mis-classifies a struct whose only field is an array on
// amd64). Generated bindings use it at the C boundary only.
type guidABI struct{ lo, hi uint64 }

// The conversions copy bytes because GUID has alignment 1 while guidABI has
// alignment 8, so a pointer cast could be misaligned.

// guidToABI converts g to its scalar-field C-boundary form.
func guidToABI(g GUID) (a guidABI) {
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&a)), len(g.Data)), g.Data[:])
	return a
}

// guid converts a back to a [GUID].
func (a guidABI) guid() (g GUID) {
	copy(g.Data[:], unsafe.Slice((*byte)(unsafe.Pointer(&a)), len(g.Data)))
	return g
}
