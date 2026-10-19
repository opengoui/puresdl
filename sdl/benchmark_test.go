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

// BenchmarkDirectCall measures the call0 ... call8 bridge against the
// reflective purego.RegisterFunc path, which floating-point signatures use.
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
	b.Run("GetFloatProperty_reflect", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = GetFloatProperty(props, key, 0)
		}
	})
}
