package sdltest

import "sync/atomic"

// Clock is a manually advanced millisecond time source that replaces
// sdl.GetTicks in code under test, so that frame stepping needs no sleeping.
// The zero value starts at 0 and is ready to use; it is safe for concurrent use.
type Clock struct{ ms atomic.Uint64 }

// Ticks returns the current time in milliseconds. Its method value matches the
// func() uint64 signature of sdl.GetTicks.
func (c *Clock) Ticks() uint64 { return c.ms.Load() }

// Advance moves the clock forward by ms milliseconds and returns the new time.
func (c *Clock) Advance(ms uint64) uint64 { return c.ms.Add(ms) }
