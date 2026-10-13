package sdltest

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// update is the -update flag: when set, golden comparisons rewrite the files
// instead of failing.
var update = flag.Bool("update", false, "rewrite sdltest golden PNG files under testdata/")

// Tolerance bounds how far an image may deviate from its golden file, to
// absorb anti-aliasing differences. The zero value demands an exact match.
type Tolerance struct {
	// Channel is the largest allowed absolute difference of any color channel.
	Channel uint8
	// Pixels is the number of pixels allowed to exceed Channel.
	Pixels int
}

// AssertImage compares img with testdata/<name>.png relative to the package
// under test (the working directory of go test) and requires an exact match.
// With -update the file is rewritten instead; review the diff afterwards. On
// mismatch the actual image is written to testdata/failed/<name>.png, which
// should be listed in .gitignore.
func AssertImage(t testing.TB, name string, img *image.RGBA) {
	t.Helper()
	AssertImageTol(t, name, img, Tolerance{})
}

// AssertImageTol is [AssertImage] with a [Tolerance].
func AssertImageTol(t testing.TB, name string, img *image.RGBA, tol Tolerance) {
	t.Helper()
	path := filepath.Join("testdata", name+".png")
	if *update {
		writePNG(t, path, img)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writePNG(t, filepath.Join("testdata", "failed", name+".png"), img)
		t.Fatalf("golden %s: %v (run with -update to create it)", path, err)
	}
	defer f.Close()
	dec, err := png.Decode(f)
	if err != nil {
		t.Fatalf("golden %s: %v", path, err)
	}
	if diff := diffImages(dec, img, tol); diff != "" {
		writePNG(t, filepath.Join("testdata", "failed", name+".png"), img)
		t.Errorf("image differs from %s: %s (actual written to testdata/failed/%s.png)", path, diff, name)
	}
}

// diffImages compares want with got under tol and returns a description of the
// mismatch, or "" when they match.
func diffImages(want image.Image, got *image.RGBA, tol Tolerance) string {
	if want.Bounds().Size() != got.Bounds().Size() {
		return fmt.Sprintf("size %v, want %v", got.Bounds().Size(), want.Bounds().Size())
	}
	bad, first := 0, ""
	wb := want.Bounds()
	for y := 0; y < wb.Dy(); y++ {
		for x := 0; x < wb.Dx(); x++ {
			wr, wg, wbl, wa := want.At(wb.Min.X+x, wb.Min.Y+y).RGBA()
			c := got.RGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
			gr, gg, gb, ga := uint32(c.R)*0x101, uint32(c.G)*0x101, uint32(c.B)*0x101, uint32(c.A)*0x101
			if absDiff(wr, gr) > uint32(tol.Channel)*0x101 || absDiff(wg, gg) > uint32(tol.Channel)*0x101 ||
				absDiff(wbl, gb) > uint32(tol.Channel)*0x101 || absDiff(wa, ga) > uint32(tol.Channel)*0x101 {
				if bad == 0 {
					first = fmt.Sprintf("first at (%d,%d)", x, y)
				}
				bad++
			}
		}
	}
	if bad > tol.Pixels {
		return fmt.Sprintf("%d pixels differ, %s", bad, first)
	}
	return ""
}

// absDiff returns |a-b|.
func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// writePNG encodes img as PNG to path, creating parent directories. It fails t
// on any error.
func writePNG(t testing.TB, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
