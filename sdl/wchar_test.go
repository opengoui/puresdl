package sdl

import "testing"

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
		_ = WcharString(d.SerialNumber)
		_ = WcharString(d.ManufacturerString)
		_ = WcharString(d.ProductString)
		n++
	}
	t.Logf("%d HID devices", n)
}
