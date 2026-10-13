// Command read-and-draw reads frames from a camera and draws them to the
// screen. It is a port of SDL/examples/camera/01-read-and-draw/read-and-draw.c.
//
// It simply opens the first camera it finds in whatever format the camera
// offers. If no camera is available it logs that and exits cleanly. Press Esc
// or close the window to quit.
//
// Run it with:
//
//	go run ./examples/camera/01-read-and-draw [-frames N]
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

// init locks the main goroutine to its OS thread, as SDL's video and event
// functions must be called from the thread that initialised them.
func init() {
	runtime.LockOSThread()
}

// main parses the -frames flag and runs the example, exiting with status 1
// and a message on stderr if run fails.
func main() {
	frames := flag.Int("frames", 0, "exit after this many frames (0 = run until quit)")
	flag.Parse()
	if err := run(*frames); err != nil {
		fmt.Fprintln(os.Stderr, "read-and-draw:", err)
		os.Exit(1)
	}
}

// run initialises SDL, creates the window and renderer, and drives the event
// and draw loop until the user quits or maxFrames frames have been presented
// (0 means no limit). It releases every SDL resource it created before
// returning.
func run(maxFrames int) error {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	sdl.SetAppMetadata("Example Camera Read and Draw", "1.0", "com.example.camera-read-and-draw")

	if !sdl.Init(sdl.INIT_VIDEO | sdl.INIT_CAMERA) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("examples/camera/read-and-draw", 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY, &window, &renderer) {
		return fmt.Errorf("couldn't create window/renderer: %s", sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	devices := sdl.GetCameras()
	if devices == nil {
		return fmt.Errorf("couldn't enumerate camera devices: %s", sdl.GetError())
	}
	if len(devices) == 0 {
		// The C program fails here; a missing camera is an expected
		// condition for this port, so report it and exit cleanly.
		fmt.Fprintln(os.Stderr, "read-and-draw: couldn't find any camera devices; please connect a camera and try again")
		return nil
	}

	// Just take the first thing we see, in any format it wants.
	camera := sdl.OpenCamera(devices[0], nil)
	if camera == nil {
		return fmt.Errorf("couldn't open camera: %s", sdl.GetError())
	}
	a := newApp(window, renderer, camera)
	defer a.close()

	var event sdl.Event
	for frame := 0; maxFrames == 0 || frame < maxFrames; frame++ {
		for sdl.PollEvent(&event) {
			quit, err := a.handleEvent(&event)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
		}
		a.iterate()
		sdl.RenderPresent(renderer)
		if maxFrames != 0 {
			sdl.Delay(1)
		}
	}
	return nil
}

// app is the state shared by the event handler and the per-frame iteration,
// mirroring the globals of the C program.
type app struct {
	window   *sdl.Window
	renderer *sdl.Renderer
	camera   *sdl.Camera
	texture  *sdl.Texture

	// acquire and release fetch and return the newest camera frame. They
	// default to the camera and exist so a test can supply frames.
	acquire func(timestampNS *uint64) *sdl.Surface
	release func(*sdl.Surface)
}

// newApp returns an app whose frame acquire and release hooks use camera.
func newApp(window *sdl.Window, renderer *sdl.Renderer, camera *sdl.Camera) *app {
	return &app{
		window:   window,
		renderer: renderer,
		camera:   camera,
		acquire:  func(ts *uint64) *sdl.Surface { return sdl.AcquireCameraFrame(camera, ts) },
		release:  func(f *sdl.Surface) { sdl.ReleaseCameraFrame(camera, f) },
	}
}

// handleEvent reports whether the program should end successfully, or an
// error when it should fail (camera use denied).
func (a *app) handleEvent(event *sdl.Event) (quit bool, err error) {
	switch sdl.EventType(event.Type()) {
	case sdl.EVENT_QUIT:
		return true, nil
	case sdl.EVENT_KEY_DOWN:
		if event.Key().Key == sdl.SDLK_ESCAPE {
			return true, nil
		}
	case sdl.EVENT_CAMERA_DEVICE_APPROVED:
		fmt.Println("Camera use approved by user!")
	case sdl.EVENT_CAMERA_DEVICE_DENIED:
		return false, fmt.Errorf("camera use denied by user")
	}
	return false, nil
}

// iterate uploads the newest camera frame, if any, and draws the scene. It
// does not present.
func (a *app) iterate() {
	var timestampNS uint64
	if surf := a.acquire(&timestampNS); surf != nil {
		// Some platforms don't know what the camera offers until the user
		// gives permission, so build the texture and resize the window
		// when the first frame arrives.
		if a.texture == nil {
			sdl.SetWindowSize(a.window, surf.W, surf.H)
			sdl.SetRenderLogicalPresentation(a.renderer, surf.W, surf.H, sdl.LOGICAL_PRESENTATION_LETTERBOX)
			a.texture = sdl.CreateTexture(a.renderer, surf.Format, sdl.TEXTUREACCESS_STREAMING, surf.W, surf.H)
		}
		if a.texture != nil {
			sdl.UpdateTextureBytes(a.texture, nil, surf.PixelBytes(), surf.Pitch)
		}
		a.release(surf)
	}

	sdl.SetRenderDrawColor(a.renderer, 0x99, 0x99, 0x99, sdl.ALPHA_OPAQUE)
	sdl.RenderClear(a.renderer)
	if a.texture != nil { // draw the latest camera frame, if available.
		sdl.RenderTexture(a.renderer, a.texture, nil, nil)
	}
}

// close releases the camera and the texture; the window and renderer belong
// to run.
func (a *app) close() {
	sdl.CloseCamera(a.camera)
	if a.texture != nil {
		sdl.DestroyTexture(a.texture)
		a.texture = nil
	}
}
