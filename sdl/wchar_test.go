package sdl

import (
	"testing"
	"unicode/utf8"
)

func TestWcharRoundTrip(t *testing.T) {
	for _, s := range []string{"", "abc", "héllo", "日本語", "emoji \U0001F600 and \U00010348"} {
		c := WcharCString(s)
		if c[len(c)-1] != 0 {
			t.Fatalf("%q: missing terminator", s)
		}
		if got := WcharString(&c[0]); got != s {
			t.Errorf("round trip %q = %q", s, got)
		}
	}
	if WcharString(nil) != "" {
		t.Error("nil must decode to empty string")
	}
	if wcstrNull("") != nil {
		t.Error("empty string must map to NULL")
	}
	if got := WcharString(wcstrNull("\U0001F600x")); got != "\U0001F600x" {
		t.Errorf("wcstrNull round trip = %q", got)
	}
}

func TestHIDEnumerate(t *testing.T) {
	coreSkipUnless(t, "SDL_hid_init", "SDL_hid_enumerate", "SDL_hid_free_enumeration", "SDL_hid_exit")
	if HIDInit() != 0 {
		t.Skipf("hid_init failed: %s", GetError())
	}
	defer HIDExit()
	devs := HIDEnumerate(0, 0)
	defer HIDFreeEnumeration(devs)
	n := 0
	for d := devs; d != nil; d = d.Next {
		for name, p := range map[string]*Wchar{"serial": d.SerialNumber, "manufacturer": d.ManufacturerString, "product": d.ProductString} {
			if got := WcharString(p); !utf8.ValidString(got) {
				t.Errorf("device %d %s = %q is not valid UTF-8", n, name, got)
			}
		}
		if d.Path == nil {
			t.Errorf("device %d has no path", n)
		}
		n++
	}
	if (devs == nil) != (n == 0) {
		t.Errorf("list head %v but %d devices walked", devs, n)
	}
}
