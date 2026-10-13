package sdl

import (
	"slices"
	"testing"
)

// TestStorageSliceForms round-trips data through a filesystem storage with the
// generated []byte and typed forms, including an empty file.
func TestStorageSliceForms(t *testing.T) {
	mediaInit(t, 0)
	st := OpenFileStorage(t.TempDir())
	if st == nil {
		t.Fatal(GetError())
	}
	defer CloseStorage(st)

	words := []uint32{1, 2, 0xdeadbeef}
	if !WriteStorageFileSlice(st, "words.bin", words) {
		t.Fatal(GetError())
	}
	var size uint64
	if !GetStorageFileSize(st, "words.bin", &size) || size != 12 {
		t.Fatalf("size = %d (%s), want 12", size, GetError())
	}
	got := make([]uint32, 3)
	if !ReadStorageFileSlice(st, "words.bin", got) || !slices.Equal(got, words) {
		t.Fatalf("read %v (%s), want %v", got, GetError(), words)
	}
	raw := make([]byte, 12)
	if !ReadStorageFileBytes(st, "words.bin", raw) {
		t.Fatal(GetError())
	}
	if !WriteStorageFileBytes(st, "copy.bin", raw) {
		t.Fatal(GetError())
	}
	back := make([]uint32, 3)
	if !ReadStorageFileSlice(st, "copy.bin", back) || !slices.Equal(back, words) {
		t.Fatalf("copy read %v, want %v", back, words)
	}
	// An empty slice writes an empty file, which a no-op would not.
	if !WriteStorageFileBytes(st, "empty.bin", nil) {
		t.Fatal(GetError())
	}
	if !GetStorageFileSize(st, "empty.bin", &size) || size != 0 {
		t.Fatalf("empty file size = %d (%s)", size, GetError())
	}
	if !ReadStorageFileBytes(st, "empty.bin", nil) {
		t.Errorf("reading an empty file with an empty slice failed: %s", GetError())
	}
}

func TestSurfacePixelBytes(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	s := CreateSurface(5, 3, PIXELFORMAT_RGBA32)
	if s == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(s)
	px := s.PixelBytes()
	if want := int(s.Pitch) * 3; len(px) != want {
		t.Fatalf("len = %d, want pitch*h = %d", len(px), want)
	}
	// The slice aliases the surface: a write is visible through SDL.
	px[int(s.Pitch)+4], px[int(s.Pitch)+5], px[int(s.Pitch)+6], px[int(s.Pitch)+7] = 1, 2, 3, 255
	if got, want := mediaPixel(t, s, 1, 1), [4]uint8{1, 2, 3, 255}; got != want {
		t.Errorf("pixel = %v, want %v", got, want)
	}
	var nilSurface *Surface
	if nilSurface.PixelBytes() != nil {
		t.Error("nil surface must give nil")
	}
}

func TestRenderGeometrySlice(t *testing.T) {
	mediaInit(t, INIT_VIDEO)
	dst := CreateSurface(8, 8, PIXELFORMAT_RGBA32)
	if dst == nil {
		t.Fatal(GetError())
	}
	defer DestroySurface(dst)
	r := CreateSoftwareRenderer(dst)
	if r == nil {
		t.Fatal(GetError())
	}
	defer DestroyRenderer(r)

	red := FColor{R: 1, A: 1}
	quad := []Vertex{
		{Position: FPoint{X: 0, Y: 0}, Color: red},
		{Position: FPoint{X: 8, Y: 0}, Color: red},
		{Position: FPoint{X: 0, Y: 8}, Color: red},
		{Position: FPoint{X: 8, Y: 8}, Color: red},
	}
	clear := func() {
		SetRenderDrawColor(r, 0, 0, 0, 255)
		RenderClear(r)
	}
	pixel := func() [4]uint8 {
		out := RenderReadPixels(r, nil)
		if out == nil {
			t.Fatal(GetError())
		}
		defer DestroySurface(out)
		return mediaPixel(t, out, 6, 6)
	}
	clear()
	if !RenderGeometrySlice(r, nil, quad[:3], nil) { // no indices: sequential triangle
		t.Fatal(GetError())
	}
	if got := pixel(); got[0] != 0 {
		t.Errorf("(6,6) is outside the triangle, pixel = %v", got)
	}
	clear()
	if !RenderGeometrySlice(r, nil, quad, []int32{0, 1, 2, 1, 2, 3}) {
		t.Fatal(GetError())
	}
	if got := pixel(); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("(6,6) is covered by the indexed quad, pixel = %v", got)
	}
	clear()
	if !RenderGeometrySlice(r, nil, nil, nil) {
		t.Errorf("empty vertices must be a successful no-op: %s", GetError())
	}
}

func TestPutAudioStreamSlice(t *testing.T) {
	mediaInit(t, INIT_AUDIO)
	spec := AudioSpec{Format: AUDIO_F32, Channels: 1, Freq: 8000}
	st := CreateAudioStream(&spec, &spec)
	if st == nil {
		t.Fatal(GetError())
	}
	defer DestroyAudioStream(st)
	if !PutAudioStreamSlice(st, []float32{0, 0.5, -0.5, 1}) {
		t.Fatal(GetError())
	}
	FlushAudioStream(st)
	if got := GetAudioStreamAvailable(st); got != 16 {
		t.Errorf("available = %d bytes, want 16 (4 float32 samples)", got)
	}
}
