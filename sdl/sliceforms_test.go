package sdl

import "testing"

// pixelAt reads the surface pixel at (x, y) as RGBA.
func pixelAt(t *testing.T, s *Surface, x, y int32) [4]uint8 {
	t.Helper()
	var p [4]uint8
	if !ReadSurfacePixel(s, x, y, &p[0], &p[1], &p[2], &p[3]) {
		t.Fatal(GetError())
	}
	return p
}

func TestGeneratedSliceForms(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(16, 16, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dst)
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)
	SetRenderDrawColor(r, 255, 0, 0, 255)

	ClearError()
	for name, ok := range map[string]bool{
		"nil points":      RenderPointsSlice(r, nil),
		"empty lines":     RenderLinesSlice(r, []FPoint{}),
		"empty rects":     RenderRectsSlice(r, nil),
		"empty fillrects": RenderFillRectsSlice(r, []FRect{}),
	} {
		if !ok {
			t.Errorf("%s: empty slice must return true", name)
		}
	}
	if e := GetError(); e != "" {
		t.Fatalf("an empty slice must not call SDL, error %q", e)
	}

	// A slice shorter than its backing array draws only its own elements.
	pts := []FPoint{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}}
	if !RenderPointsSlice(r, pts[:2]) {
		t.Fatal(GetError())
	}
	RenderPresent(r)
	if got := pixelAt(t, dst, 2, 2); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("pixel (2,2) = %v, want red", got)
	}
	if got := pixelAt(t, dst, 3, 3); got[0] != 0 {
		t.Errorf("pixel (3,3) = %v, want untouched", got)
	}

	if !RenderFillRectsSlice(r, []FRect{{X: 8, Y: 8, W: 2, H: 2}}) {
		t.Fatal(GetError())
	}
	RenderPresent(r)
	if got := pixelAt(t, dst, 9, 9); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("filled pixel = %v, want red", got)
	}
}
