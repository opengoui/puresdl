package main

import (
	"errors"
	"math"
	"os"
	"testing"
	"unsafe"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// clip applies the frame transform (a diagonal scale) to a vertex.
func clip(f *frame, v vertex) (x, y float32) { return v.X * f.transform[0], v.Y * f.transform[5] }

func TestFillCoversDiskEvenly(t *testing.T) {
	f := &frame{verts: make([]vertex, 3*4000)}
	nan := float32(math.NaN())
	for _, tm := range []float64{0, 3, 40, 1234.5} {
		fill(f, 1, tm, 1280, 720, nan, 0)
		var quadrant [4]int
		for i := 0; i < len(f.verts); i += vertsPerParticle {
			x, y := clip(f, f.verts[i])
			if x < -1 || x > 1 || y < -1 || y > 1 {
				t.Fatalf("t=%v: particle %d at (%v, %v) is outside clip space", tm, i/3, x, y)
			}
			k := 0
			if x > 0 {
				k++
			}
			if y > 0 {
				k += 2
			}
			quadrant[k]++
		}
		// A correlated speed sequence once squeezed the field into one arm.
		for k, n := range quadrant {
			if n < 900 || n > 1100 {
				t.Errorf("t=%v: quadrant %d holds %d of 4000 particles: %v", tm, k, n, quadrant)
			}
		}
	}
}

func TestFillIsRoundOnWideTargets(t *testing.T) {
	f := &frame{verts: make([]vertex, 3*2000)}
	fill(f, 0, 2, 1600, 400, float32(math.NaN()), 0)
	var maxX, maxY float32
	for i := 0; i < len(f.verts); i += vertsPerParticle {
		x, y := clip(f, f.verts[i])
		maxX, maxY = max(maxX, abs(x)), max(maxY, abs(y))
	}
	// In pixels the field must be as wide as it is tall: 1600*maxX == 400*maxY.
	if r := 1600 * maxX / (400 * maxY); r < 0.9 || r > 1.1 {
		t.Errorf("width/height of the field in pixels = %v, want 1", r)
	}
}

func TestFillFollowsMouse(t *testing.T) {
	f := &frame{verts: make([]vertex, 3*2000)}
	fill(f, 0, 5, 800, 800, 0.5, -0.25)
	var sx, sy float32
	n := float32(len(f.verts) / vertsPerParticle)
	for i := 0; i < len(f.verts); i += vertsPerParticle {
		x, y := clip(f, f.verts[i])
		sx, sy = sx+x, sy+y
	}
	if cx, cy := sx/n, sy/n; abs(cx-0.5) > 0.05 || abs(cy+0.25) > 0.05 {
		t.Errorf("field centre = (%v, %v), want about (0.5, -0.25)", cx, cy)
	}
}

func abs(v float32) float32 { return float32(math.Abs(float64(v))) }

func TestMailboxKeepsNewestAndRecyclesFrames(t *testing.T) {
	m := newMailbox(3)
	if m.take() != nil {
		t.Fatal("empty mailbox returned a frame")
	}
	a, b := <-m.free, <-m.free
	m.publish(a)
	m.publish(b) // a was never taken: it goes back to the free list
	if got := m.take(); got != b {
		t.Fatal("take did not return the newest frame")
	}
	if m.take() != nil {
		t.Fatal("a frame was returned twice")
	}
	m.release(b)
	if n := len(m.free); n != 3 {
		t.Errorf("%d free frames, want all 3 back", n)
	}
}

func TestRequestReplacesPendingTime(t *testing.T) {
	s := newScene(0, 1)
	s.request(1)
	s.request(2)
	if got := <-s.tick; got != 2 {
		t.Errorf("worker sees t=%v, want the latest request 2", got)
	}
}

func TestWorkerPublishesRequestedFrameAndStops(t *testing.T) {
	s := newScene(0, 50)
	s.setSize(200, 100)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { s.work(done); close(stopped) }()
	s.request(1.5)
	want := &frame{verts: make([]vertex, 50*vertsPerParticle)}
	fill(want, 0, 1.5, 200, 100, float32(math.NaN()), 0)
	var got *frame
	for got == nil {
		got = s.box.take()
	}
	if got.verts[7] != want.verts[7] || got.transform != want.transform {
		t.Errorf("worker frame differs from fill for the requested time")
	}
	close(done)
	<-stopped
}

func TestShaderCodeFollowsDeviceFormats(t *testing.T) {
	for _, tc := range []struct {
		formats sdl.GPUShaderFormat
		format  sdl.GPUShaderFormat
		entry   string
	}{
		{sdl.GPU_SHADERFORMAT_MSL | sdl.GPU_SHADERFORMAT_METALLIB, sdl.GPU_SHADERFORMAT_MSL, "main0\x00"},
		{sdl.GPU_SHADERFORMAT_SPIRV, sdl.GPU_SHADERFORMAT_SPIRV, "main\x00"},
		{sdl.GPU_SHADERFORMAT_DXIL | sdl.GPU_SHADERFORMAT_DXBC, sdl.GPU_SHADERFORMAT_DXIL, "main\x00"},
	} {
		vert, frag, entry, format, err := shaderCode(tc.formats)
		if err != nil || len(vert) == 0 || len(frag) == 0 || entry != tc.entry || format != tc.format {
			t.Errorf("shaderCode(%#x) = %d/%d bytes, %q, %#x, %v", tc.formats, len(vert), len(frag), entry, format, err)
		}
	}
	if _, _, _, _, err := shaderCode(sdl.GPU_SHADERFORMAT_DXBC); err == nil {
		t.Error("shaderCode accepted a format the example does not ship")
	}
}

// The vertex layout must match the float3 position and float4 color inputs
// of PositionColorTransform.vert.
func TestVertexLayoutMatchesShader(t *testing.T) {
	if s, o := unsafe.Sizeof(vertex{}), unsafe.Offsetof(vertex{}.R); s != 28 || o != 12 {
		t.Errorf("vertex size %d, color offset %d; want 28 and 12", s, o)
	}
}

func TestPresentModeFlag(t *testing.T) {
	for name, want := range map[string]sdl.GPUPresentMode{
		"vsync": sdl.GPU_PRESENTMODE_VSYNC, "mailbox": sdl.GPU_PRESENTMODE_MAILBOX, "immediate": sdl.GPU_PRESENTMODE_IMMEDIATE,
	} {
		if got, err := presentMode(name); err != nil || got != want {
			t.Errorf("presentMode(%q) = %v, %v", name, got, err)
		}
	}
	if _, err := presentMode("fifo"); err == nil {
		t.Error("presentMode accepted an unknown name")
	}
}

// The dummy video driver has no GPU swapchain, so the smoke test skips when
// the device or the window claim is unavailable.
func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(config{frames: 3, windows: 2, particles: 10, present: "vsync"}) })
	if errors.Is(err, errNoGPU) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
}
