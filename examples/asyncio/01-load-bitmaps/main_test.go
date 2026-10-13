package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }

// waitTimeoutMS and waitAttempts bound a blocking wait for a load to finish.
// They are failure bounds, never delays: the wait returns as soon as a result
// is queued. SDL_WaitAsyncIOResult may return false early (spurious wakeup),
// so the wait is retried.
const (
	waitTimeoutMS = 1000
	waitAttempts  = 30
)

// waitResult blocks until the queue yields an outcome. Main thread only.
func waitResult(q *sdl.AsyncIOQueue, o *sdl.AsyncIOOutcome) bool {
	for range waitAttempts {
		if sdl.WaitAsyncIOResult(q, o, waitTimeoutMS) {
			return true
		}
	}
	return false
}

var black = color.RGBA{0, 0, 0, 255}

// writeAssets writes the embedded PNGs into a fresh temporary directory.
func writeAssets(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range pngs {
		data, err := assets.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// newTestApp initializes SDL and starts the loads from dir, drawing on a canvas.
func newTestApp(t *testing.T, dir string) (*app, *sdltest.Canvas) {
	t.Helper()
	sdltest.Setup(t, sdl.INIT_VIDEO)
	c := sdltest.NewCanvas(t, 640, 480)
	a := sdltest.Call(t, func() *app {
		a, err := newApp(c.Renderer, dir)
		if err != nil {
			return nil
		}
		return a
	})
	if a == nil {
		t.Fatal("newApp failed")
	}
	t.Cleanup(func() { sdltest.OnMain(t, a.close) })
	return a, c
}

// waitOutcome blocks until one load finishes and returns what it carried,
// after letting the app handle it. It reports the userdata index, the result
// and the byte count seen before the app released the buffer.
type seen struct {
	index  int
	result sdl.AsyncIOResult
	size   int
}

func waitAndHandle(t *testing.T, a *app) seen {
	t.Helper()
	var s seen
	var errMsg string
	sdltest.OnMain(t, func() {
		var o sdl.AsyncIOOutcome
		if !waitResult(a.queue, &o) {
			errMsg = "timed out waiting for an async load"
			return
		}
		s.index, _ = sdl.UserdataHandleValue(o.Userdata).(int)
		s.result = o.Result
		s.size = len(o.Bytes())
		if err := a.handleOutcome(&o); err != nil {
			errMsg = err.Error()
		}
	})
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	return s
}

func loadAll(t *testing.T, a *app) []seen {
	t.Helper()
	var all []seen
	for range pngs {
		all = append(all, waitAndHandle(t, a))
	}
	return all
}

func render(t *testing.T, a *app, c *sdltest.Canvas) *image.RGBA {
	t.Helper()
	sdltest.OnMain(t, a.draw)
	return c.Capture(t)
}

func decode(t *testing.T, name string) image.Image {
	t.Helper()
	data, err := assets.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func rectOf(i int) image.Rectangle {
	r := textureRects[i]
	return image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))
}

func hasNonBlack(img *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if sdltest.PixelAt(img, x, y) != black {
				return true
			}
		}
	}
	return false
}

func TestInitialFrameIsBlack(t *testing.T) {
	a, c := newTestApp(t, writeAssets(t))
	// Nothing has been consumed from the queue yet, so no texture exists.
	img := render(t, a, c)
	sdltest.AssertRegion(t, img, image.Rect(0, 0, 640, 480), black)
	for i, tex := range a.textures {
		if tex != nil {
			t.Errorf("texture %d exists before any load was handled", i)
		}
	}
}

func TestEachLoadIsKeyedByItsFile(t *testing.T) {
	a, _ := newTestApp(t, writeAssets(t))
	got := map[int]seen{}
	for _, s := range loadAll(t, a) {
		got[s.index] = s
	}
	for i, name := range pngs {
		s, ok := got[i]
		if !ok {
			t.Fatalf("no outcome for index %d (%s)", i, name)
		}
		want, _ := assets.ReadFile(name)
		if s.result != sdl.ASYNCIO_COMPLETE || s.size != len(want) {
			t.Errorf("%s: result=%d size=%d, want complete with %d bytes", name, s.result, s.size, len(want))
		}
		tex := a.textures[i]
		if tex == nil {
			t.Fatalf("%s: no texture", name)
		}
		var w, h float32
		sdltest.OnMain(t, func() { sdl.GetTextureSize(tex, &w, &h) })
		src := decode(t, name).Bounds()
		if int(w) != src.Dx() || int(h) != src.Dy() {
			t.Errorf("%s: texture is %vx%v, PNG is %dx%d", name, w, h, src.Dx(), src.Dy())
		}
	}
}

func TestLoadedImagesAppearAtTheirRects(t *testing.T) {
	a, c := newTestApp(t, writeAssets(t))
	loadAll(t, a)
	img := render(t, a, c)

	// Unscaled images match the decoded PNG pixel for pixel where opaque.
	for _, i := range []int{0, 3} {
		src := image.NewRGBA(decode(t, pngs[i]).Bounds())
		draw.Draw(src, src.Bounds(), decode(t, pngs[i]), image.Point{}, draw.Src)
		r := rectOf(i)
		if r.Dx() != src.Bounds().Dx() || r.Dy() != src.Bounds().Dy() {
			t.Fatalf("%s is scaled; pick another assertion", pngs[i])
		}
		checked := 0
		for y := 0; y < r.Dy(); y++ {
			for x := 0; x < r.Dx(); x++ {
				want := src.RGBAAt(x, y)
				if want.A == 255 {
					sdltest.AssertPixel(t, img, r.Min.X+x, r.Min.Y+y, want)
					checked++
				}
			}
		}
		if checked == 0 {
			t.Errorf("%s: no opaque pixels were compared", pngs[i])
		}
	}
	// Scaled images only need to show up inside their rect.
	for _, i := range []int{1, 2} {
		if !hasNonBlack(img, rectOf(i)) {
			t.Errorf("%s: rect %v is all black", pngs[i], rectOf(i))
		}
	}
	// Everything outside the four rects stays the black clear colour.
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			p := image.Pt(x, y)
			inside := false
			for i := range pngs {
				inside = inside || p.In(rectOf(i))
			}
			if !inside && sdltest.PixelAt(img, x, y) != black {
				t.Fatalf("pixel (%d,%d) outside every rect is %v, want black", x, y, sdltest.PixelAt(img, x, y))
			}
		}
	}
}

func TestImagesAppearOneAtATime(t *testing.T) {
	a, c := newTestApp(t, writeAssets(t))
	loaded := map[int]bool{}
	for n := 1; n <= len(pngs); n++ {
		loaded[waitAndHandle(t, a).index] = true
		img := render(t, a, c)
		for i := range pngs {
			if got := hasNonBlack(img, rectOf(i)); got != loaded[i] {
				t.Errorf("after %d loads, %s drawn=%v, want %v", n, pngs[i], got, loaded[i])
			}
		}
	}
}

func TestIterateConsumesAtMostOneResultPerCall(t *testing.T) {
	a, _ := newTestApp(t, writeAssets(t))
	// iterate polls without blocking; between polls, block on the queue so the
	// loop makes progress without sleeping.
	count := func() (n int) {
		for _, tex := range a.textures {
			if tex != nil {
				n++
			}
		}
		return n
	}
	prev := 0
	for count() < len(pngs) {
		var err error
		sdltest.OnMain(t, func() { err = a.iterate() })
		if err != nil {
			t.Fatal(err)
		}
		n := count()
		if n > prev+1 {
			t.Fatalf("one iterate created %d textures", n-prev)
		}
		prev = n
		if prev < len(pngs) {
			var o sdl.AsyncIOOutcome
			var got bool
			sdltest.OnMain(t, func() {
				if got = waitResult(a.queue, &o); got {
					err = a.handleOutcome(&o)
				}
			})
			if !got || err != nil {
				t.Fatalf("wait failed: got=%v err=%v", got, err)
			}
			prev = count()
		}
	}
}

func TestMissingFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	// Only one file exists; the other three fail to load.
	data, _ := assets.ReadFile(pngs[3])
	if err := os.WriteFile(filepath.Join(dir, pngs[3]), data, 0o600); err != nil {
		t.Fatal(err)
	}
	a, c := newTestApp(t, dir)
	// A file that cannot be opened never starts a task (SDL_LoadFileAsync
	// fails synchronously), so exactly one outcome arrives.
	s := waitAndHandle(t, a)
	if s.index != 3 || s.result != sdl.ASYNCIO_COMPLETE {
		t.Errorf("outcome = %+v, want a completed load of index 3", s)
	}
	var o sdl.AsyncIOOutcome
	if sdltest.Call(t, func() bool { return sdl.GetAsyncIOResult(a.queue, &o) }) {
		t.Errorf("unexpected extra outcome for index %v", sdl.UserdataHandleValue(o.Userdata))
	}
	for i := range pngs {
		if (a.textures[i] != nil) != (i == 3) {
			t.Errorf("%s: texture present=%v", pngs[i], a.textures[i] != nil)
		}
	}
	img := render(t, a, c)
	if !hasNonBlack(img, rectOf(3)) {
		t.Error("the one valid image was not drawn")
	}
	for i := 0; i < 3; i++ {
		sdltest.AssertRegion(t, img, rectOf(i), black)
	}
}

func TestInvalidPNGDataIsIgnored(t *testing.T) {
	dir := writeAssets(t)
	if err := os.WriteFile(filepath.Join(dir, pngs[1]), []byte("not a png at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, c := newTestApp(t, dir)
	loadAll(t, a) // handleOutcome must not report an error for undecodable data
	if a.textures[1] != nil {
		t.Error("texture created from invalid PNG data")
	}
	img := render(t, a, c)
	sdltest.AssertRegion(t, img, rectOf(1), black)
	if !hasNonBlack(img, rectOf(0)) {
		t.Error("valid images must still be drawn")
	}
}

func TestOutOfRangeUserdataIsIgnored(t *testing.T) {
	a, c := newTestApp(t, writeAssets(t))
	loadAll(t, a)
	before := a.textures
	// Feed a synthetic outcome with a bogus index and a real SDL buffer.
	err := sdltest.Call(t, func() error {
		buf := sdl.Malloc(4)
		o := sdl.AsyncIOOutcome{
			Result:           sdl.ASYNCIO_COMPLETE,
			Buffer:           buf,
			BytesTransferred: 4,
			Userdata:         sdl.NewUserdataHandle(len(pngs)),
		}
		return a.handleOutcome(&o)
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.textures != before {
		t.Error("textures changed for an out-of-range index")
	}
	if !hasNonBlack(render(t, a, c), rectOf(0)) {
		t.Error("existing textures must survive")
	}
}

func TestQuitEventOnly(t *testing.T) {
	a, _ := newTestApp(t, writeAssets(t))
	in := sdltest.NewInput(t, 0)
	in.Press(sdl.SCANCODE_ESCAPE)
	in.MouseButton(1, true, 10, 10)
	in.Resized(800, 600)
	in.CloseRequested()
	for _, e := range sdltest.Drain(t) {
		ev := e
		if a.handleEvent(&ev) {
			t.Errorf("event type %#x must not quit", e.Type())
		}
	}
	in.Quit()
	quit := false
	sdltest.OnMain(t, func() {
		sdl.PumpEvents()
		var e sdl.Event
		for sdl.PollEvent(&e) {
			quit = quit || a.handleEvent(&e)
		}
	})
	if !quit {
		t.Error("EVENT_QUIT did not quit")
	}
}

func TestCloseReleasesTextures(t *testing.T) {
	a, _ := newTestApp(t, writeAssets(t))
	loadAll(t, a)
	sdltest.OnMain(t, a.close)
	for i, tex := range a.textures {
		if tex != nil {
			t.Errorf("texture %d still held after close", i)
		}
	}
	// The cleanup calls close again only for textures; queue is destroyed once
	// per app, so replace it to keep the cleanup safe.
	a.queue = sdltest.Call(t, sdl.CreateAsyncIOQueue)
}

func TestRunSmoke(t *testing.T) {
	sdltest.Skip(t)
	var err error
	sdltest.OnMain(t, func() { err = run(30) })
	if err != nil {
		t.Fatal(err)
	}
}
