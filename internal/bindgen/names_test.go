package bindgen

import "testing"

func TestGoName(t *testing.T) {
	for in, want := range map[string]string{
		"SDL_CreateWindow":   "CreateWindow",
		"SDL_INIT_VIDEO":     "INIT_VIDEO",
		"SDLK_A":             "SDLK_A",
		"SDL_hid_init":       "HIDInit",
		"SDL_aligned_alloc":  "AlignedAlloc",
		"SDL_murmur3_32":     "Murmur3_32",
		"SDL_getenv":         "Getenv",
		"SDL_rand_bits_r":    "RandBitsR",
		"SDL_hid_device":     "HIDDevice",
		"SDL_GPUBufferUsage": "GPUBufferUsage",
	} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFieldAndParamNames(t *testing.T) {
	if got := fieldName("min_depth"); got != "MinDepth" {
		t.Errorf("fieldName = %q", got)
	}
	if got := fieldName("windowID"); got != "WindowID" {
		t.Errorf("fieldName = %q", got)
	}
	if got := paramName("type", 0); got != "type_" {
		t.Errorf("paramName = %q", got)
	}
	if got := paramName("", 3); got != "p3" {
		t.Errorf("paramName = %q", got)
	}
}

func TestNullable(t *testing.T) {
	for doc, want := range map[string]bool{
		"the name of the driver, or NULL to use the first one": true,
		"a string that must not be NULL":                       false,
		"a NULL-terminated list of names":                      false,
		"a title":                                              false,
	} {
		if got := nullable(doc); got != want {
			t.Errorf("nullable(%q) = %v", doc, got)
		}
	}
}
