// Package sdltest is a hermetic test harness for programs built on package
// sdl, in particular the self-contained examples under examples/. It lets a
// test drive real SDL code (keyboard, mouse, pen, gamepad, joystick,
// clipboard, audio queue, window resize, quit) with no hardware, no visible
// window and no sleeping.
//
// # Setup
//
// Every test package that uses sdltest needs this TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }
//
// [Main] selects the dummy video, audio and camera drivers and the software
// renderer (through SDL hints, so the caller's environment is irrelevant) and
// runs the tests on a goroutine while the process main thread serves
// [OnMain]. SDL video and event functions must run on the main thread; the
// examples lock it in init, so their run functions assert the same.
//
// A test then looks like:
//
//	func TestQuit(t *testing.T) {
//		sdltest.Skip(t)                 // Skip when libSDL3 is not loadable
//		in := sdltest.NewInput(t, 0)    // inject events, any goroutine
//		in.Quit()
//		sdltest.OnMain(t, func() { err = run(0) })
//	}
//
// # Facilities
//
//   - [Main], [OnMain], [Call]: main-thread execution.
//   - [Skip], [Setup]: environment checks and SDL init/quit with cleanup.
//   - [Input]: event injection through sdl.PushEvent; [Pump] and [Drain]
//     observe the queue; [Clock] is a deterministic time source.
//   - [AttachJoystick], [AttachGamepad]: virtual devices, including rumble capture.
//   - [NewCanvas], [Capture], [AssertImage]: software rendering, pixel
//     readback and PNG golden files (testdata/ of the calling package, -update
//     to rewrite, testdata/failed/ on mismatch).
//   - [NewAudioProbe]: unbound audio streams whose queued bytes can be read back.
//
// # Limits
//
// Injected events only enter the event queue. State that SDL derives from
// real devices (sdl.GetKeyboardState, sdl.GetMouseState, clipboard contents
// on a real pasteboard) is not changed by them; code under test should
// consume events. Virtual joysticks do update real joystick state. The dummy
// camera driver and dummy audio device exist but produce no real I/O.
//
// # Making examples testable
//
// Split the example into pure logic and a thin loop: a state struct with
// handleEvent(e *sdl.Event) (quit bool) and step/iterate methods that take the
// current time as an argument (or hold a now func() uint64 field that
// defaults to sdl.GetTicks and that tests replace with [Clock.Ticks]), and
// run(maxFrames int) that only owns init, window, renderer and the
// poll/step/draw loop. Draw into a function that takes the *sdl.Renderer so a
// test can pass the renderer from [NewCanvas].
package sdltest
