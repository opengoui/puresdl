package sdltest

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to the main OS thread.
func init() {
	// Package init runs on the process main goroutine; locking here makes
	// that goroutine, and so Main, stay on the main OS thread.
	runtime.LockOSThread()
}

// queue carries calls from test goroutines to the main goroutine; active is
// true while Main is serving it.
var (
	queue  = make(chan *mainCall)
	active atomic.Bool
)

// mainCall is one function submitted to the main thread, with the panic (if
// any) and its stack captured for re-raising on the caller.
type mainCall struct {
	fn    func()
	done  chan struct{}
	panic any
	stack []byte
}

// Main runs the tests of a package and returns the process exit code. Call it
// from TestMain on the main goroutine:
//
//	func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }
//
// The tests run on another goroutine; this goroutine stays on the main OS
// thread and executes the functions passed to [OnMain]. Main also applies
// [Env], so code that calls sdl.Init itself gets the dummy drivers.
func Main(m *testing.M) int {
	Env()
	active.Store(true)
	done := make(chan int, 1)
	go func() { done <- m.Run() }()
	for {
		select {
		case c := <-queue:
			c.run()
		case code := <-done:
			active.Store(false)
			return code
		}
	}
}

// run executes the call on the main thread, recording any panic and then
// signalling completion.
func (c *mainCall) run() {
	defer close(c.done)
	defer func() {
		if r := recover(); r != nil {
			c.panic, c.stack = r, stackTrace()
		}
	}()
	c.fn()
}

// stackTrace returns the current goroutine's stack, truncated to 16 KiB.
func stackTrace() []byte {
	buf := make([]byte, 16<<10)
	return buf[:runtime.Stack(buf, false)]
}

// OnMain runs fn on the process main thread and waits for it to return. It
// fails the test if the package's TestMain does not use [Main].
//
// fn must not call t.Fatal or any other method that ends the goroutine (it
// does not run on the test goroutine) and must not call OnMain itself. Return
// errors or values instead, see [Call]. A panic in fn is re-raised in the test
// goroutine with the original stack appended. Tests that use OnMain must not
// be parallel, because SDL state is process-global.
func OnMain(t testing.TB, fn func()) {
	t.Helper()
	if !active.Load() {
		t.Fatal("sdltest: OnMain needs TestMain to call sdltest.Main")
	}
	c := &mainCall{fn: fn, done: make(chan struct{})}
	queue <- c
	<-c.done
	if c.panic != nil {
		panic(fmt.Sprintf("%v\n\nmain-thread stack:\n%s", c.panic, c.stack))
	}
}

// Call runs fn on the main thread and returns its result; see [OnMain].
func Call[T any](t testing.TB, fn func() T) T {
	t.Helper()
	var v T
	OnMain(t, func() { v = fn() })
	return v
}

// Env selects the dummy video, audio and camera drivers and the software
// renderer, through SDL hints and the process environment. It must run before
// sdl.Init; [Main], [Skip] and [Setup] call it, so tests rarely need to. The
// SDL hints are set only when libSDL3 is loaded; the environment variables
// are always set.
func Env() {
	for _, kv := range [][2]string{
		{sdl.HINT_VIDEO_DRIVER, "dummy"},
		{sdl.HINT_AUDIO_DRIVER, "dummy"},
		{sdl.HINT_CAMERA_DRIVER, "dummy"},
		{sdl.HINT_RENDER_DRIVER, "software"},
	} {
		os.Setenv(kv[0], kv[1])
		if sdl.LoadError() == nil {
			sdl.SetHint(kv[0], kv[1])
		}
	}
}
