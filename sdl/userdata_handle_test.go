package sdl

import (
	"testing"
	"unsafe"
)

func skipWithoutSDL(t *testing.T) {
	t.Helper()
	if err := LoadError(); err != nil {
		t.Skipf("libSDL3 not loadable: %v", err)
	}
}

func TestUserdataHandleTokens(t *testing.T) {
	skipWithoutSDL(t)
	type payload struct{ n int }
	want := &payload{n: 7}
	h := NewUserdataHandle(want)
	h2 := NewUserdataHandle("x")
	defer FreeUserdataHandle(h)
	defer FreeUserdataHandle(h2)
	for _, p := range []unsafe.Pointer{h, h2} {
		if p == nil || uintptr(p) < 4096 {
			t.Fatalf("token %#x is not a valid non-Go address", uintptr(p))
		}
	}
	if h == h2 {
		t.Fatal("live tokens must be distinct")
	}
	if got, _ := UserdataHandleValue(h).(*payload); got != want {
		t.Fatalf("value = %v, want %v", got, want)
	}
	if got := UserdataHandleValue(h2); got != "x" {
		t.Fatalf("value = %v, want x", got)
	}
}

func TestUserdataHandleFree(t *testing.T) {
	skipWithoutSDL(t)
	h := NewUserdataHandle(1)
	FreeUserdataHandle(h)
	if UserdataHandleValue(h) != nil {
		t.Fatal("value still present after free")
	}
	FreeUserdataHandle(h) // double free must be a no-op, not a C double free
	FreeUserdataHandle(nil)
	if UserdataHandleValue(nil) != nil {
		t.Fatal("nil token must have no value")
	}
}

//go:noinline
func holdTokenAcrossGrowth(n int, tok unsafe.Pointer) unsafe.Pointer {
	var pad [4 << 10]byte
	pad[n%len(pad)] = byte(n)
	if n > 0 {
		r := holdTokenAcrossGrowth(n-1, tok)
		return unsafe.Add(r, int(pad[0])*0)
	}
	return tok
}

// A token held live in a frame must survive stack copying; small-integer
// pointers made the runtime abort with "invalid pointer found on stack".
func TestUserdataHandleSurvivesStackGrowth(t *testing.T) {
	skipWithoutSDL(t)
	for i := 0; i < 50; i++ {
		done := make(chan struct{})
		go func() {
			defer close(done)
			tok := NewUserdataHandle(i)
			defer FreeUserdataHandle(tok)
			got := holdTokenAcrossGrowth(64, tok)
			if got != tok || UserdataHandleValue(got) != i {
				t.Errorf("token changed across stack growth")
			}
		}()
		<-done
	}
}
