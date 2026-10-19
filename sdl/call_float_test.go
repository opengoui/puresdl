package sdl

import (
	"runtime"
	"testing"
)

// softwareRenderer returns a software renderer drawing into a w x h RGBA32
// surface, both destroyed when the test ends.
func softwareRenderer(t *testing.T, w, h int32) (*Renderer, *Surface) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	surface := CreateSurface(w, h, PIXELFORMAT_RGBA32)
	if surface == nil {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { DestroySurface(surface) })
	renderer := CreateSoftwareRenderer(surface)
	if renderer == nil {
		t.Fatal(GetError())
	}
	t.Cleanup(func() { DestroyRenderer(renderer) })
	return renderer, surface
}

// Floating-point arguments and results reach C in the right registers, in
// parameter order, interleaved with integer arguments.
func TestFloatDirectCallValues(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	const key = "puresdl.float\x00"
	if !SetFloatProperty(props, key, 1.25) {
		t.Fatal(GetError())
	}
	if got := GetFloatProperty(props, key, -1); got != 1.25 {
		t.Errorf("GetFloatProperty = %v, want 1.25", got)
	}
	if got := GetFloatProperty(props, "puresdl.missing\x00", -3.5); got != -3.5 {
		t.Errorf("GetFloatProperty(missing) = %v, want the default -3.5", got)
	}

	renderer, _ := softwareRenderer(t, 8, 8)
	if !SetRenderScale(renderer, 2.5, 0.75) {
		t.Fatal(GetError())
	}
	var sx, sy float32
	if !GetRenderScale(renderer, &sx, &sy) || sx != 2.5 || sy != 0.75 {
		t.Errorf("GetRenderScale = %v, %v, want 2.5, 0.75", sx, sy)
	}
	if !SetRenderDrawColorFloat(renderer, 0.125, 0.25, 0.5, 0.75) {
		t.Fatal(GetError())
	}
	var r, g, b, a float32
	if !GetRenderDrawColorFloat(renderer, &r, &g, &b, &a) || r != 0.125 || g != 0.25 || b != 0.5 || a != 0.75 {
		t.Errorf("GetRenderDrawColorFloat = %v %v %v %v, want 0.125 0.25 0.5 0.75", r, g, b, a)
	}
	// Floats between pointer arguments: x and y go to float registers while
	// the out-pointers keep their integer registers.
	var wx, wy float32
	if !RenderCoordinatesToWindow(renderer, 2, 4, &wx, &wy) || wx != 5 || wy != 3 {
		t.Errorf("RenderCoordinatesToWindow(2, 4) = %v, %v, want 5, 3", wx, wy)
	}

	surface := CreateSurface(4, 4, PIXELFORMAT_RGBA128_FLOAT)
	if surface == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(surface)
	if !WriteSurfacePixelFloat(surface, 3, 1, 0.25, 0.5, 0.75, 1) {
		t.Fatal(GetError())
	}
	if !ReadSurfacePixelFloat(surface, 3, 1, &r, &g, &b, &a) || r != 0.25 || g != 0.5 || b != 0.75 || a != 1 {
		t.Errorf("pixel (3, 1) = %v %v %v %v, want 0.25 0.5 0.75 1", r, g, b, a)
	}
}

// A double argument is passed with its full 64-bit image: rotating by 180
// degrees swaps the halves of a two-color texture.
func TestFloatDirectCallDouble(t *testing.T) {
	renderer, _ := softwareRenderer(t, 4, 2)
	src := CreateSurface(2, 1, PIXELFORMAT_RGBA32)
	if src == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(src)
	red, blue := MapSurfaceRGBA(src, 255, 0, 0, 255), MapSurfaceRGBA(src, 0, 0, 255, 255)
	FillSurfaceRect(src, &Rect{X: 0, Y: 0, W: 1, H: 1}, red)
	FillSurfaceRect(src, &Rect{X: 1, Y: 0, W: 1, H: 1}, blue)
	tex := CreateTextureFromSurface(renderer, src)
	if tex == nil {
		t.Fatal(GetError())
	}
	defer DestroyTexture(tex)
	SetTextureScaleMode(tex, SCALEMODE_NEAREST)
	if !RenderTextureRotated(renderer, tex, nil, nil, 180, nil, FLIP_NONE) {
		t.Fatal(GetError())
	}
	out := RenderReadPixels(renderer, nil)
	if out == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(out)
	var r, g, b, a uint8
	if !ReadSurfacePixel(out, 0, 0, &r, &g, &b, &a) {
		t.Fatal(GetError())
	}
	if r != 0 || b != 255 {
		t.Errorf("left pixel after a 180 degree rotation = %d,%d,%d,%d, want blue", r, g, b, a)
	}
}

// Floating-point direct calls do not allocate either.
func TestFloatDirectCallsDoNotAllocate(t *testing.T) {
	props := CreateProperties()
	if props == 0 {
		t.Fatal(GetError())
	}
	defer DestroyProperties(props)
	const key = "puresdl.float\x00"
	SetFloatProperty(props, key, 1)
	renderer, _ := softwareRenderer(t, 8, 8)
	for name, f := range map[string]func(){
		"GetFloatProperty":        func() { _ = GetFloatProperty(props, key, 0) },
		"SetRenderDrawColorFloat": func() { _ = SetRenderDrawColorFloat(renderer, 1, 1, 1, 1) },
		"RenderLine":              func() { _ = RenderLine(renderer, 0, 0, 7, 7) },
	} {
		if !floatCalls {
			t.Skip("floating-point direct calls fall back to purego.RegisterFunc on this platform")
		}
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s allocates %v times per call", name, n)
		}
	}
}
