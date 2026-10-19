package sdl

import (
	"runtime"
	"testing"
)

func BenchmarkGetNumberProperty(b *testing.B) {
	props := CreateProperties()
	if props == 0 {
		b.Fatal(GetError())
	}
	defer DestroyProperties(props)

	const nulTerminated = "puresdl.benchmark.number\x00"
	if !SetNumberProperty(props, nulTerminated, 42) {
		b.Fatal(GetError())
	}
	for _, tc := range []struct {
		name string
		key  string
	}{
		{name: "nul_terminated", key: nulTerminated},
		{name: "needs_copy", key: "puresdl.benchmark.number"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if got := GetNumberProperty(props, tc.key, 0); got != 42 {
					b.Fatalf("GetNumberProperty = %d, want 42", got)
				}
			}
		})
	}
}

func BenchmarkRenderPointsSlice(b *testing.B) {
	runtime.LockOSThread()
	b.Cleanup(runtime.UnlockOSThread)
	if !Init(INIT_VIDEO) {
		b.Fatal(GetError())
	}
	b.Cleanup(Quit)

	surface := CreateSurface(64, 48, PIXELFORMAT_RGBA32)
	if surface == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroySurface(surface) })

	renderer := CreateSoftwareRenderer(surface)
	if renderer == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroyRenderer(renderer) })

	points := make([]FPoint, 100)
	for i := range points {
		points[i] = FPoint{X: float32(i % 64), Y: float32(i % 48)}
	}
	b.ReportAllocs()
	for b.Loop() {
		if !RenderPointsSlice(renderer, points) {
			b.Fatal(GetError())
		}
	}
}

// BenchmarkDirectCall measures the integer call0 ... call8 bridge and the
// floating-point callF0 ... callF8 bridge (GetFloatProperty, which uses
// purego.RegisterFunc where floatCalls is false).
func BenchmarkDirectCall(b *testing.B) {
	props := CreateProperties()
	if props == 0 {
		b.Fatal(GetError())
	}
	defer DestroyProperties(props)
	const key = "puresdl.benchmark.float\x00"
	SetFloatProperty(props, key, 1)
	b.Run("GetTicks_0args", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = GetTicks()
		}
	})
	b.Run("HasProperty_2args", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = HasProperty(props, key)
		}
	})
	b.Run("GetFloatProperty", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = GetFloatProperty(props, key, 0)
		}
	})
}

// BenchmarkFloatCall measures floating-point signatures, which call C through
// callF0 ... callF8 where floatCalls is true and through purego.RegisterFunc
// elsewhere.
func BenchmarkFloatCall(b *testing.B) {
	runtime.LockOSThread()
	b.Cleanup(runtime.UnlockOSThread)
	surface := CreateSurface(64, 64, PIXELFORMAT_RGBA32)
	if surface == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroySurface(surface) })
	renderer := CreateSoftwareRenderer(surface)
	if renderer == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroyRenderer(renderer) })
	b.Run("SetRenderDrawColorFloat", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = SetRenderDrawColorFloat(renderer, 0.25, 0.5, 0.75, 1)
		}
	})
	b.Run("RenderLine", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = RenderLine(renderer, 0, 0, 1, 1)
		}
	})
}

// BenchmarkScratchParams measures calls whose single-value pointer
// parameters are copied into the call frame instead of escaping to the heap.
func BenchmarkScratchParams(b *testing.B) {
	runtime.LockOSThread()
	b.Cleanup(runtime.UnlockOSThread)
	if !InitSubSystem(INIT_EVENTS) {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { QuitSubSystem(INIT_EVENTS) })
	surface := CreateSurface(64, 64, PIXELFORMAT_RGBA32)
	if surface == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroySurface(surface) })
	renderer := CreateSoftwareRenderer(surface)
	if renderer == nil {
		b.Fatal(GetError())
	}
	b.Cleanup(func() { DestroyRenderer(renderer) })
	b.Run("RenderFillRect_literal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			_ = RenderFillRect(renderer, &FRect{X: float32(i & 7), Y: 1, W: 2, H: 2})
		}
	})
	b.Run("GetRenderViewport_local", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var r Rect
			_ = GetRenderViewport(renderer, &r)
		}
	})
	b.Run("PollEvent_local", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var ev Event
			_ = PollEvent(&ev)
		}
	})
}
