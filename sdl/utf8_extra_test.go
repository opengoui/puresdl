package sdl

import "testing"

func TestStepUTF8(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantR    rune
		wantSize int
	}{
		{"empty", "", 0, 0},
		{"ascii", "ab", 'a', 1},
		{"two byte", "é!", 'é', 2},
		{"four byte", "😀x", '😀', 4},
		{"stray continuation", "\x80a", INVALID_UNICODE_CODEPOINT, 1},
		{"truncated", "\xe2\x82", INVALID_UNICODE_CODEPOINT, 1},
		{"encoded replacement", "\uFFFD", INVALID_UNICODE_CODEPOINT, 3},
	}
	for _, tc := range tests {
		r, n := StepUTF8(tc.in)
		if r != tc.wantR || n != tc.wantSize {
			t.Errorf("%s: StepUTF8(%q) = (%U, %d), want (%U, %d)", tc.name, tc.in, r, n, tc.wantR, tc.wantSize)
		}
	}
}

func TestStepBackUTF8(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantR    rune
		wantSize int
	}{
		{"empty", "", 0, 0},
		{"ascii", "ab", 'b', 1},
		{"two byte", "a€", '€', 3},
		{"four byte", "x😀", '😀', 4},
		{"stray continuation", "a\x80", INVALID_UNICODE_CODEPOINT, 1},
		{"truncated", "a\xe2\x82", INVALID_UNICODE_CODEPOINT, 1},
	}
	for _, tc := range tests {
		r, n := StepBackUTF8(tc.in)
		if r != tc.wantR || n != tc.wantSize {
			t.Errorf("%s: StepBackUTF8(%q) = (%U, %d), want (%U, %d)", tc.name, tc.in, r, n, tc.wantR, tc.wantSize)
		}
	}
}

func TestUCS4ToUTF8(t *testing.T) {
	tests := []struct {
		in   rune
		want string
	}{
		{'a', "a"},
		{0xE9, "é"},
		{0x20AC, "€"},
		{0x1F600, "😀"},
		{0x10FFFF, "\U0010FFFF"},
		{0xD800, "\uFFFD"},
		{0x110000, "\uFFFD"},
		{-1, "\uFFFD"},
	}
	for _, tc := range tests {
		if got := UCS4ToUTF8(tc.in); got != tc.want {
			t.Errorf("UCS4ToUTF8(%U) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
