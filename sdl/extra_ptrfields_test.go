package sdl

import (
	"slices"
	"testing"
)

func TestPaletteColorSlice(t *testing.T) {
	var nilPal *Palette
	if got := nilPal.ColorSlice(); got != nil {
		t.Fatalf("nil palette: got %v, want nil", got)
	}
	if got := (&Palette{Ncolors: 3}).ColorSlice(); got != nil {
		t.Fatalf("nil Colors: got %v, want nil", got)
	}
	colors := []Color{{R: 1, G: 2, B: 3, A: 4}, {R: 5, G: 6, B: 7, A: 8}, {R: 9}}
	if got := (&Palette{Ncolors: 0, Colors: &colors[0]}).ColorSlice(); got != nil {
		t.Fatalf("zero Ncolors: got %v, want nil", got)
	}
	pal := &Palette{Ncolors: 2, Colors: &colors[0]}
	got := pal.ColorSlice()
	if !slices.Equal(got, colors[:2]) {
		t.Fatalf("got %v, want %v", got, colors[:2])
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestDropEventStrings(t *testing.T) {
	var nilEv *DropEvent
	if nilEv.DataString() != "" || nilEv.SourceString() != "" {
		t.Fatal("nil event should give empty strings")
	}
	if ev := (&DropEvent{}); ev.DataString() != "" || ev.SourceString() != "" {
		t.Fatal("NULL pointers should give empty strings")
	}
	data, src := CString("/tmp/file.txt"), CString("app")
	ev := &DropEvent{Data: data, Source: src}
	if got := ev.DataString(); got != "/tmp/file.txt" {
		t.Fatalf("DataString = %q", got)
	}
	if got := ev.SourceString(); got != "app" {
		t.Fatalf("SourceString = %q", got)
	}
	first := ev.DataString()
	firstSrc := ev.SourceString()
	*data = 'X' // mutate the C buffer after the strings were taken
	*src = 'Z'
	if first != "/tmp/file.txt" || firstSrc != "app" {
		t.Fatalf("earlier strings changed: %q, %q", first, firstSrc)
	}
	if got := ev.DataString(); got != "Xtmp/file.txt" {
		t.Fatalf("DataString after mutate = %q", got)
	}
	if got := ev.SourceString(); got != "Zpp" {
		t.Fatalf("SourceString after mutate = %q", got)
	}
}
