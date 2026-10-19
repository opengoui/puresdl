// Command 01-multi-window drives several windows from one SDL GPU device and
// moves the per-frame CPU work onto goroutines.
//
// It is not a port of an SDL example. It shows the threading model that SDL
// requires and how Go can still use every core:
//
//   - The main goroutine is locked to the main OS thread. It alone pumps
//     events, acquires swapchain textures, records command buffers and
//     submits them, because SDL ties windows, swapchains and command buffers
//     to the thread that created them.
//   - Every window has a worker goroutine that builds the next frame's
//     vertices (an animated particle field) in parallel with the other
//     workers and with the GPU. Frames move between the two sides through a
//     three-buffer mailbox, so the hot loop does not allocate.
//   - One window paces the loop with WaitForGPUSwapchain; every window then
//     acquires its swapchain texture with AcquireGPUSwapchainTexture, which
//     skips the window for this iteration when its frames in flight are not
//     done instead of waiting for them. On Metal the acquire still waits in
//     CAMetalLayer.nextDrawable until the layer has a free drawable, which is
//     the actual VSync wait there; -stats reports it separately from the work
//     of the main goroutine.
//
// The window titles show the presented frame rate. Close a window to drop
// it; the program exits when the last one closes, on Escape, or after -frames
// N loop iterations (0 runs until quit).
//
// Run it with:
//
//	go run ./examples/gpu/01-multi-window -windows 3 -particles 20000
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/opengoui/puresdl/sdl"
)

// shaders holds PositionColorTransform.vert and SolidColor.frag from
// TheSpydog/SDL_gpu_examples, precompiled for each backend (see
// shaders/LICENSE).
//
//go:embed shaders/*.spv shaders/*.msl shaders/*.dxil
var shaders embed.FS

// init pins the main goroutine to the main OS thread, which SDL requires for
// windows, events and swapchains.
func init() {
	runtime.LockOSThread()
}

// main parses the flags, runs the example and exits with status 1 on error.
func main() {
	frames := flag.Int("frames", 0, "exit after this many loop iterations (0 = run until quit)")
	windows := flag.Int("windows", 3, "number of windows")
	particles := flag.Int("particles", 20000, "particles per window")
	present := flag.String("present", "vsync", "present mode: vsync, mailbox or immediate")
	stats := flag.Bool("stats", false, "print the frame rate of every window once per second")
	flag.Parse()
	cfg := config{frames: *frames, windows: *windows, particles: *particles, present: *present, stats: *stats}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "01-multi-window:", err)
		os.Exit(1)
	}
}

// config is the parsed command line.
type config struct {
	frames, windows, particles int
	present                    string
	stats                      bool
}

// errNoGPU reports that the SDL GPU API cannot drive a window here, for
// example under the dummy video driver.
var errNoGPU = errors.New("no usable SDL GPU device")

// vertex matches the PositionColorTransform.vert input: a float3 position
// at location 0 and a float4 color at location 1.
type vertex struct {
	X, Y, Z    float32
	R, G, B, A float32
}

// vertsPerParticle is one triangle per particle.
const vertsPerParticle = 3

// frame is the CPU-side data of one frame of one window, built by a worker
// and consumed by the main goroutine.
type frame struct {
	verts     []vertex
	transform [16]float32 // column-major float4x4 for the vertex uniform
}

// mailbox hands frames from a worker to the main goroutine. Three frames
// circulate: one being filled, one published as latest, one being uploaded.
// A frame the main goroutine never took is recycled, so the newest frame
// always wins and nothing is allocated after start-up.
type mailbox struct {
	latest atomic.Pointer[frame]
	free   chan *frame
}

// newMailbox returns a mailbox with three frames of n vertices.
func newMailbox(n int) *mailbox {
	m := &mailbox{free: make(chan *frame, 3)}
	for range 3 {
		m.free <- &frame{verts: make([]vertex, n)}
	}
	return m
}

// publish makes f the latest frame and recycles the frame it replaces.
func (m *mailbox) publish(f *frame) {
	if old := m.latest.Swap(f); old != nil {
		m.free <- old
	}
}

// take returns the latest frame, or nil when none was published since the
// last take. The caller returns it with release.
func (m *mailbox) take() *frame { return m.latest.Swap(nil) }

// release returns a frame taken with take.
func (m *mailbox) release(f *frame) { m.free <- f }

// scene is the per-window animation state shared by a worker and the main
// goroutine. The fields written by the main goroutine are atomics.
type scene struct {
	index     int
	particles int
	size      atomic.Uint64 // pixel width<<32 | height
	mouse     atomic.Uint64 // float32 bits of x<<32 | y in [-1, 1], NaN when outside
	tick      chan float64  // time in seconds of the frame to build next
	box       *mailbox
}

// newScene returns the scene of window index with n particles.
func newScene(index, n int) *scene {
	s := &scene{index: index, particles: n, tick: make(chan float64, 1), box: newMailbox(n * vertsPerParticle)}
	s.setSize(1, 1)
	s.setMouse(float32(math.NaN()), 0)
	return s
}

func (s *scene) setSize(w, h int32) { s.size.Store(uint64(uint32(w))<<32 | uint64(uint32(h))) }

func (s *scene) setMouse(x, y float32) {
	s.mouse.Store(uint64(math.Float32bits(x))<<32 | uint64(math.Float32bits(y)))
}

// request asks the worker to build the frame for time t. A request the worker
// has not picked up yet is replaced, so a slow worker skips frames instead of
// falling behind.
func (s *scene) request(t float64) {
	select {
	case <-s.tick:
	default:
	}
	s.tick <- t
}

// work builds frames until done is closed. It runs on its own goroutine and
// never calls SDL.
func (s *scene) work(done <-chan struct{}) {
	for {
		var t float64
		select {
		case <-done:
			return
		case t = <-s.tick:
		}
		var f *frame
		select {
		case <-done:
			return
		case f = <-s.box.free:
		}
		size, mouse := s.size.Load(), s.mouse.Load()
		fill(f, s.index, t, uint32(size>>32), uint32(size),
			math.Float32frombits(uint32(mouse>>32)), math.Float32frombits(uint32(mouse)))
		s.box.publish(f)
	}
}

// fill writes the particle field of window index at time t into f. The field
// orbits the centre, or the mouse position (mx, my in [-1, 1]) when it is not
// NaN. Positions are in a space whose x axis is stretched by the aspect ratio
// of the w x h target; the transform maps it back to clip space.
func fill(f *frame, index int, t float64, w, h uint32, mx, my float32) {
	cx, cy := float32(0), float32(0)
	if !math.IsNaN(float64(mx)) {
		cx, cy = mx, my
	}
	aspect := float32(1)
	if w > 0 && h > 0 {
		aspect = float32(w) / float32(h)
	}
	n := len(f.verts) / vertsPerParticle
	hue0 := float64(index) * 0.37
	const golden = 2.399963229728653 // radians between successive particles
	for i := range n {
		fi := float64(i)
		// The speed sequence uses the plastic ratio, which is independent of
		// the golden angle; a golden-ratio speed would line particles up in
		// one spiral arm.
		speed := 0.2 + 0.8*math.Mod(fi*0.7548776662466927, 1)
		angle := fi*golden + t*speed
		radius := 0.05 + 0.9*math.Sqrt((fi+0.5)/float64(n))*(0.85+0.15*math.Sin(t*1.7+fi*0.01))
		sin, cos := math.Sincos(angle)
		x := cx*aspect + float32(cos*radius)
		y := cy + float32(sin*radius)
		r, g, b := hsv(math.Mod(hue0+fi/float64(n)+t*0.05, 1))
		const s = 0.012
		v := f.verts[i*vertsPerParticle : i*vertsPerParticle+vertsPerParticle]
		v[0] = vertex{x, y + s, 0, r, g, b, 0.8}
		v[1] = vertex{x - s*0.866, y - s*0.5, 0, r, g, b, 0.8}
		v[2] = vertex{x + s*0.866, y - s*0.5, 0, r, g, b, 0.8}
	}
	f.transform = [16]float32{1 / aspect, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// hsv converts a hue in [0, 1) at full saturation and value to RGB.
func hsv(h float64) (r, g, b float32) {
	i, f := math.Modf(h * 6)
	q, p := float32(1-f), float32(f)
	switch int(i) % 6 {
	case 0:
		return 1, p, 0
	case 1:
		return q, 1, 0
	case 2:
		return 0, 1, p
	case 3:
		return 0, q, 1
	case 4:
		return p, 0, 1
	}
	return 1, 0, q
}

// shaderCode returns the vertex and fragment shader bytecode and entry point
// for the first format in formats that the example ships.
func shaderCode(formats sdl.GPUShaderFormat) (vert, frag []byte, entry string, format sdl.GPUShaderFormat, err error) {
	var ext string
	switch {
	case formats&sdl.GPU_SHADERFORMAT_MSL != 0:
		ext, entry, format = "msl", "main0\x00", sdl.GPU_SHADERFORMAT_MSL
	case formats&sdl.GPU_SHADERFORMAT_SPIRV != 0:
		ext, entry, format = "spv", "main\x00", sdl.GPU_SHADERFORMAT_SPIRV
	case formats&sdl.GPU_SHADERFORMAT_DXIL != 0:
		ext, entry, format = "dxil", "main\x00", sdl.GPU_SHADERFORMAT_DXIL
	default:
		return nil, nil, "", 0, fmt.Errorf("no shipped shader format in %#x", formats)
	}
	if vert, err = shaders.ReadFile("shaders/PositionColorTransform.vert." + ext); err != nil {
		return nil, nil, "", 0, err
	}
	if frag, err = shaders.ReadFile("shaders/SolidColor.frag." + ext); err != nil {
		return nil, nil, "", 0, err
	}
	return vert, frag, entry, format, nil
}

// presentMode maps the -present flag to an SDL present mode.
func presentMode(name string) (sdl.GPUPresentMode, error) {
	switch name {
	case "vsync":
		return sdl.GPU_PRESENTMODE_VSYNC, nil
	case "mailbox":
		return sdl.GPU_PRESENTMODE_MAILBOX, nil
	case "immediate":
		return sdl.GPU_PRESENTMODE_IMMEDIATE, nil
	}
	return 0, fmt.Errorf("unknown present mode %q", name)
}

// window is the GPU state of one window, owned by the main goroutine.
type window struct {
	w        *sdl.Window
	id       sdl.WindowID
	scene    *scene
	vbuf     *sdl.GPUBuffer
	tbuf     *sdl.GPUTransferBuffer
	verts    uint32      // vertices in vbuf
	uniform  [16]float32 // transform of the uploaded frame
	target   sdl.GPUColorTargetInfo
	presents int // frames presented since the title was updated
	total    int
}

// app is the state of the main goroutine.
type app struct {
	dev      *sdl.GPUDevice
	pipeline map[sdl.GPUTextureFormat]*sdl.GPUGraphicsPipeline
	vert     *sdl.GPUShader
	frag     *sdl.GPUShader
	windows  []*window
	done     chan struct{}
	workers  sync.WaitGroup
	start    uint64 // GetTicksNS when the frame loop started
	// acquireNS and workNS split the main goroutine's time since the last
	// title update into waiting for swapchain textures and everything else.
	acquireNS, workNS uint64
}

// run initializes SDL and the GPU device, opens the windows, starts the
// workers and drives the frame loop. SDL resources are released on return.
func run(cfg config) (err error) {
	if err := sdl.LoadError(); err != nil {
		return err
	}
	if cfg.windows < 1 || cfg.particles < 1 {
		return fmt.Errorf("need at least one window and one particle")
	}
	mode, err := presentMode(cfg.present)
	if err != nil {
		return err
	}
	sdl.SetAppMetadata("puresdl GPU multi-window", "1.0", "com.example.gpu-multi-window")
	if !sdl.Init(sdl.INIT_VIDEO) {
		return fmt.Errorf("couldn't initialize SDL: %s", sdl.GetError())
	}
	defer sdl.Quit()

	dev := sdl.CreateGPUDevice(sdl.GPU_SHADERFORMAT_MSL|sdl.GPU_SHADERFORMAT_SPIRV|sdl.GPU_SHADERFORMAT_DXIL, false, "")
	if dev == nil {
		return fmt.Errorf("%w: %s", errNoGPU, sdl.GetError())
	}
	a := &app{dev: dev, pipeline: map[sdl.GPUTextureFormat]*sdl.GPUGraphicsPipeline{}, done: make(chan struct{})}
	defer a.close()
	// Two frames in flight let the CPU record frame N+1 while the GPU
	// renders frame N without queueing more latency.
	sdl.SetGPUAllowedFramesInFlight(dev, 2)
	if err := a.loadShaders(); err != nil {
		return err
	}
	for i := range cfg.windows {
		if err := a.openWindow(i, cfg.particles, mode); err != nil {
			return err
		}
	}
	fmt.Printf("driver %s, %d windows x %d particles, present %s\n", sdl.GetGPUDeviceDriver(dev), cfg.windows, cfg.particles, cfg.present)
	return a.loop(cfg.frames, cfg.stats)
}

// loadShaders creates the vertex and fragment shaders for the device.
func (a *app) loadShaders() error {
	vert, frag, entry, format, err := shaderCode(sdl.GetGPUShaderFormats(a.dev))
	if err != nil {
		return err
	}
	create := func(code []byte, stage sdl.GPUShaderStage, uniforms uint32) *sdl.GPUShader {
		info := sdl.GPUShaderCreateInfo{
			CodeSize:          uint(len(code)),
			Code:              &code[0],
			Entrypoint:        unsafe.StringData(entry),
			Format:            format,
			Stage:             stage,
			NumUniformBuffers: uniforms,
		}
		return sdl.CreateGPUShader(a.dev, &info)
	}
	if a.vert = create(vert, sdl.GPU_SHADERSTAGE_VERTEX, 1); a.vert == nil {
		return fmt.Errorf("couldn't create vertex shader: %s", sdl.GetError())
	}
	if a.frag = create(frag, sdl.GPU_SHADERSTAGE_FRAGMENT, 0); a.frag == nil {
		return fmt.Errorf("couldn't create fragment shader: %s", sdl.GetError())
	}
	return nil
}

// pipelineFor returns the additive-blend particle pipeline for a swapchain
// format, creating it on first use. Windows usually share one format.
func (a *app) pipelineFor(format sdl.GPUTextureFormat) (*sdl.GPUGraphicsPipeline, error) {
	if p := a.pipeline[format]; p != nil {
		return p, nil
	}
	buffers := []sdl.GPUVertexBufferDescription{{
		Slot: 0, Pitch: uint32(unsafe.Sizeof(vertex{})), InputRate: sdl.GPU_VERTEXINPUTRATE_VERTEX,
	}}
	attrs := []sdl.GPUVertexAttribute{
		{Location: 0, BufferSlot: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT3, Offset: 0},
		{Location: 1, BufferSlot: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT4, Offset: uint32(unsafe.Offsetof(vertex{}.R))},
	}
	targets := []sdl.GPUColorTargetDescription{{
		Format: format,
		BlendState: sdl.GPUColorTargetBlendState{
			EnableBlend:         true,
			SrcColorBlendfactor: sdl.GPU_BLENDFACTOR_SRC_ALPHA,
			DstColorBlendfactor: sdl.GPU_BLENDFACTOR_ONE,
			ColorBlendOp:        sdl.GPU_BLENDOP_ADD,
			SrcAlphaBlendfactor: sdl.GPU_BLENDFACTOR_ONE,
			DstAlphaBlendfactor: sdl.GPU_BLENDFACTOR_ONE,
			AlphaBlendOp:        sdl.GPU_BLENDOP_ADD,
		},
	}}
	info := sdl.GPUGraphicsPipelineCreateInfo{
		VertexShader:   a.vert,
		FragmentShader: a.frag,
		VertexInputState: sdl.GPUVertexInputState{
			VertexBufferDescriptions: &buffers[0], NumVertexBuffers: uint32(len(buffers)),
			VertexAttributes: &attrs[0], NumVertexAttributes: uint32(len(attrs)),
		},
		PrimitiveType:   sdl.GPU_PRIMITIVETYPE_TRIANGLELIST,
		RasterizerState: sdl.GPURasterizerState{FillMode: sdl.GPU_FILLMODE_FILL, CullMode: sdl.GPU_CULLMODE_NONE},
		TargetInfo:      sdl.GPUGraphicsPipelineTargetInfo{ColorTargetDescriptions: &targets[0], NumColorTargets: 1},
	}
	p := sdl.CreateGPUGraphicsPipeline(a.dev, &info)
	runtime.KeepAlive(buffers)
	runtime.KeepAlive(attrs)
	runtime.KeepAlive(targets)
	if p == nil {
		return nil, fmt.Errorf("couldn't create pipeline: %s", sdl.GetError())
	}
	a.pipeline[format] = p
	return p, nil
}

// openWindow creates window i, claims it for the device, allocates its
// buffers and starts its worker.
func (a *app) openWindow(i, particles int, mode sdl.GPUPresentMode) error {
	title := fmt.Sprintf("puresdl GPU window %d", i+1)
	w := sdl.CreateWindow(title, 640, 480, sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY)
	if w == nil {
		return fmt.Errorf("couldn't create window: %s", sdl.GetError())
	}
	sdl.SetWindowPosition(w, int32(80+i*60), int32(80+i*60))
	// Bring the window forward: macOS throttles presentation of occluded
	// windows, which a program started from a terminal often opens behind.
	sdl.RaiseWindow(w)
	if !sdl.ClaimWindowForGPUDevice(a.dev, w) {
		err := fmt.Errorf("%w: couldn't claim window: %s", errNoGPU, sdl.GetError())
		sdl.DestroyWindow(w)
		return err
	}
	win := &window{w: w, id: sdl.GetWindowID(w), scene: newScene(i, particles)}
	a.windows = append(a.windows, win)
	if dm := sdl.GetCurrentDisplayMode(sdl.GetDisplayForWindow(w)); dm != nil {
		fmt.Printf("window %d: display refresh %.0f Hz\n", i+1, dm.RefreshRate)
	}
	if !sdl.WindowSupportsGPUPresentMode(a.dev, w, mode) {
		fmt.Printf("window %d: present mode unsupported, using vsync\n", i+1)
		mode = sdl.GPU_PRESENTMODE_VSYNC
	}
	if !sdl.SetGPUSwapchainParameters(a.dev, w, sdl.GPU_SWAPCHAINCOMPOSITION_SDR, mode) {
		return fmt.Errorf("couldn't set swapchain parameters: %s", sdl.GetError())
	}
	if _, err := a.pipelineFor(sdl.GetGPUSwapchainTextureFormat(a.dev, w)); err != nil {
		return err
	}
	size := uint32(particles * vertsPerParticle * int(unsafe.Sizeof(vertex{})))
	win.vbuf = sdl.CreateGPUBuffer(a.dev, &sdl.GPUBufferCreateInfo{Usage: sdl.GPU_BUFFERUSAGE_VERTEX, Size: size})
	win.tbuf = sdl.CreateGPUTransferBuffer(a.dev, &sdl.GPUTransferBufferCreateInfo{Usage: sdl.GPU_TRANSFERBUFFERUSAGE_UPLOAD, Size: size})
	if win.vbuf == nil || win.tbuf == nil {
		return fmt.Errorf("couldn't create buffers: %s", sdl.GetError())
	}
	var pw, ph int32
	sdl.GetWindowSizeInPixels(w, &pw, &ph)
	win.scene.setSize(pw, ph)
	win.target = sdl.GPUColorTargetInfo{
		ClearColor: sdl.FColor{R: 0.02, G: 0.02, B: 0.05, A: 1},
		LoadOp:     sdl.GPU_LOADOP_CLEAR,
		StoreOp:    sdl.GPU_STOREOP_STORE,
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		win.scene.work(a.done)
	}()
	win.scene.request(seconds())
	return nil
}

// seconds is the SDL clock in seconds.
func seconds() float64 { return float64(sdl.GetTicksNS()) / 1e9 }

// loop runs until every window is closed, Escape or quit, or maxFrames
// iterations (0 means no limit). The window titles show the frame rate; stats
// also prints it once per second.
func (a *app) loop(maxFrames int, stats bool) error {
	var ev sdl.Event
	a.start = sdl.GetTicksNS()
	lastTitle := a.start
	iters := 0
	for iter := 0; maxFrames == 0 || iter < maxFrames; iter++ {
		for sdl.PollEvent(&ev) {
			if a.handleEvent(&ev) {
				return nil
			}
		}
		if len(a.windows) == 0 {
			return nil
		}
		// One blocking wait paces the loop to the first window's display.
		sdl.WaitForGPUSwapchain(a.dev, a.windows[0].w)
		begin := sdl.GetTicksNS()
		next := float64(begin) / 1e9
		wait := a.acquireNS
		for _, win := range a.windows {
			if err := a.draw(win); err != nil {
				return err
			}
			// The worker builds the next frame while the GPU renders this one.
			win.scene.request(next)
		}
		now := sdl.GetTicksNS()
		a.workNS += now - begin - (a.acquireNS - wait)
		iters++
		if now-lastTitle >= 1e9 {
			for _, win := range a.windows {
				fps := float64(win.presents) * 1e9 / float64(now-lastTitle)
				sdl.SetWindowTitle(win.w, fmt.Sprintf("puresdl GPU window %d — %.0f fps", win.scene.index+1, fps))
				if stats {
					fmt.Printf("window %d: %.1f fps  ", win.scene.index+1, fps)
				}
				win.presents = 0
			}
			if stats {
				fmt.Printf("| per frame: work %.2f ms, swapchain wait %.2f ms\n",
					float64(a.workNS)/float64(iters)/1e6, float64(a.acquireNS)/float64(iters)/1e6)
			}
			a.workNS, a.acquireNS, iters = 0, 0, 0
			lastTitle = now
		}
	}
	return nil
}

// handleEvent routes an event to its window and reports whether to quit.
func (a *app) handleEvent(ev *sdl.Event) (quit bool) {
	switch ev.Type() {
	case sdl.EVENT_QUIT:
		return true
	case sdl.EVENT_KEY_DOWN:
		return ev.Key().Key == sdl.SDLK_ESCAPE
	case sdl.EVENT_WINDOW_CLOSE_REQUESTED:
		a.closeWindow(ev.Window().WindowID)
		return len(a.windows) == 0
	case sdl.EVENT_WINDOW_PIXEL_SIZE_CHANGED:
		if win := a.find(ev.Window().WindowID); win != nil {
			win.scene.setSize(ev.Window().Data1, ev.Window().Data2)
		}
	case sdl.EVENT_MOUSE_MOTION:
		m := ev.Motion()
		if win := a.find(m.WindowID); win != nil {
			var w, h int32
			sdl.GetWindowSize(win.w, &w, &h)
			if w > 0 && h > 0 {
				win.scene.setMouse(2*m.X/float32(w)-1, 1-2*m.Y/float32(h))
			}
		}
	case sdl.EVENT_WINDOW_MOUSE_LEAVE:
		if win := a.find(ev.Window().WindowID); win != nil {
			win.scene.setMouse(float32(math.NaN()), 0)
		}
	}
	return false
}

// find returns the open window with the given ID, or nil.
func (a *app) find(id sdl.WindowID) *window {
	for _, win := range a.windows {
		if win.id == id {
			return win
		}
	}
	return nil
}

// draw records and submits one frame for win. The swapchain is acquired
// without blocking: when it is not ready (another display's refresh, or a
// minimized window) the window keeps its previous image and its newest frame
// stays in the mailbox.
func (a *app) draw(win *window) error {
	cb := sdl.AcquireGPUCommandBuffer(a.dev)
	if cb == nil {
		return fmt.Errorf("couldn't acquire command buffer: %s", sdl.GetError())
	}
	var tex *sdl.GPUTexture
	t := sdl.GetTicksNS()
	ok := sdl.AcquireGPUSwapchainTexture(cb, win.w, &tex, nil, nil)
	a.acquireNS += sdl.GetTicksNS() - t
	if !ok {
		sdl.CancelGPUCommandBuffer(cb)
		return fmt.Errorf("couldn't acquire swapchain texture: %s", sdl.GetError())
	}
	if tex == nil {
		sdl.CancelGPUCommandBuffer(cb)
		return nil
	}
	if f := win.scene.box.take(); f != nil {
		a.upload(cb, win, f)
		win.scene.box.release(f)
	}
	win.target.Texture = tex
	pass := sdl.BeginGPURenderPass(cb, &win.target, 1, nil)
	if win.verts > 0 {
		p, err := a.pipelineFor(sdl.GetGPUSwapchainTextureFormat(a.dev, win.w))
		if err != nil {
			sdl.EndGPURenderPass(pass)
			sdl.SubmitGPUCommandBuffer(cb)
			return err
		}
		sdl.BindGPUGraphicsPipeline(pass, p)
		binding := sdl.GPUBufferBinding{Buffer: win.vbuf}
		sdl.BindGPUVertexBuffers(pass, 0, &binding, 1)
		sdl.PushGPUVertexUniformData(cb, 0, unsafe.Pointer(&win.uniform), uint32(unsafe.Sizeof(win.uniform)))
		sdl.DrawGPUPrimitives(pass, win.verts, 1, 0, 0)
	}
	sdl.EndGPURenderPass(pass)
	if !sdl.SubmitGPUCommandBuffer(cb) {
		return fmt.Errorf("couldn't submit command buffer: %s", sdl.GetError())
	}
	win.presents++
	win.total++
	return nil
}

// upload copies f into the window's vertex buffer. Cycling the transfer and
// vertex buffers lets SDL hand out fresh memory while the GPU still reads the
// previous frame, so the copy never waits for the GPU.
func (a *app) upload(cb *sdl.GPUCommandBuffer, win *window, f *frame) {
	size := uint32(len(f.verts) * int(unsafe.Sizeof(vertex{})))
	dst := sdl.MapGPUTransferBuffer(a.dev, win.tbuf, true)
	if dst == nil {
		return
	}
	copy(unsafe.Slice((*vertex)(dst), len(f.verts)), f.verts)
	sdl.UnmapGPUTransferBuffer(a.dev, win.tbuf)
	cp := sdl.BeginGPUCopyPass(cb)
	src := sdl.GPUTransferBufferLocation{TransferBuffer: win.tbuf}
	region := sdl.GPUBufferRegion{Buffer: win.vbuf, Size: size}
	sdl.UploadToGPUBuffer(cp, &src, &region, true)
	sdl.EndGPUCopyPass(cp)
	win.verts = uint32(len(f.verts))
	win.uniform = f.transform
}

// closeWindow releases the window with the given ID and stops its worker.
func (a *app) closeWindow(id sdl.WindowID) {
	for i, win := range a.windows {
		if win.id == id {
			a.release(win)
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			return
		}
	}
}

// release frees the GPU resources of win and destroys it. Its worker keeps
// running until close and only touches its own scene.
func (a *app) release(win *window) {
	if a.start != 0 {
		elapsed := float64(sdl.GetTicksNS()-a.start) / 1e9
		fmt.Printf("window %d: %d frames presented, %.1f fps average\n", win.scene.index+1, win.total, float64(win.total)/elapsed)
	}
	sdl.WaitForGPUIdle(a.dev)
	if win.vbuf != nil {
		sdl.ReleaseGPUBuffer(a.dev, win.vbuf)
	}
	if win.tbuf != nil {
		sdl.ReleaseGPUTransferBuffer(a.dev, win.tbuf)
	}
	sdl.ReleaseWindowFromGPUDevice(a.dev, win.w)
	sdl.DestroyWindow(win.w)
}

// close stops the workers and releases every GPU object.
func (a *app) close() {
	close(a.done)
	a.workers.Wait()
	for _, win := range a.windows {
		a.release(win)
	}
	a.windows = nil
	for _, p := range a.pipeline {
		sdl.ReleaseGPUGraphicsPipeline(a.dev, p)
	}
	if a.vert != nil {
		sdl.ReleaseGPUShader(a.dev, a.vert)
	}
	if a.frag != nil {
		sdl.ReleaseGPUShader(a.dev, a.frag)
	}
	sdl.DestroyGPUDevice(a.dev)
}
