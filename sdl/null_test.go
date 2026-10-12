package sdl

import "testing"

func TestNullStringProperty(t *testing.T) {
	p := CreateProperties()
	defer DestroyProperties(p)

	if !SetStringProperty(p, "empty", Ptr("")) {
		t.Fatal(GetError())
	}
	if !HasProperty(p, "empty") || GetPropertyType(p, "empty") != PROPERTY_TYPE_STRING {
		t.Fatal(`SetStringProperty(p, "empty", Ptr("")) did not store a string`)
	}
	if got := GetStringProperty(p, "empty", "default"); got != "" {
		t.Errorf("empty value read back as %q, not the stored empty string", got)
	}

	if !SetStringProperty(p, "empty", nil) {
		t.Fatal(GetError())
	}
	if HasProperty(p, "empty") {
		t.Error("nil value (NULL) must delete the property")
	}
	if got := GetStringProperty(p, "empty", "default"); got != "default" {
		t.Errorf("deleted property read as %q, want the default", got)
	}

	SetStringProperty(p, "s", Ptr("v"))
	if got := GetStringProperty(p, "s", ""); got != "v" {
		t.Errorf("value = %q", got)
	}
}

func TestNullHintOK(t *testing.T) {
	const name = "SDL_NULL_TEST_HINT"
	if v, ok := GetHintOK(name); ok || v != "" {
		t.Fatalf("unset hint = (%q, %v)", v, ok)
	}
	if !SetHint(name, "") {
		t.Fatal(GetError())
	}
	defer ResetHint(name)
	// An empty value is "set": it differs from NULL only through the OK form.
	if v, ok := GetHintOK(name); !ok || v != "" {
		t.Errorf("empty hint = (%q, %v), want (\"\", true)", v, ok)
	}
	SetHint(name, "x")
	if v, ok := GetHintOK(name); !ok || v != "x" || GetHint(name) != "x" {
		t.Errorf("hint = (%q, %v)", v, ok)
	}
	ResetHint(name)
	if _, ok := GetHintOK(name); ok {
		t.Error("ResetHint should leave the hint unset")
	}
}

func TestNullEnvironmentVariableOK(t *testing.T) {
	env := CreateEnvironment(false)
	defer DestroyEnvironment(env)
	if v, ok := GetEnvironmentVariableOK(env, "NULL_X"); ok || v != "" {
		t.Fatalf("unset variable = (%q, %v)", v, ok)
	}
	if !SetEnvironmentVariable(env, "NULL_X", "", true) {
		t.Fatal(GetError())
	}
	if v, ok := GetEnvironmentVariableOK(env, "NULL_X"); !ok || v != "" {
		t.Errorf("empty variable = (%q, %v), want (\"\", true)", v, ok)
	}
	SetEnvironmentVariable(env, "NULL_X", "1", true)
	if v, ok := GetEnvironmentVariableOK(env, "NULL_X"); !ok || v != "1" {
		t.Errorf("variable = (%q, %v)", v, ok)
	}
	UnsetEnvironmentVariable(env, "NULL_X")
	if _, ok := GetEnvironmentVariableOK(env, "NULL_X"); ok {
		t.Error("unset variable still reported as set")
	}
}

func TestNullAppMetadataProperty(t *testing.T) {
	const name = PROP_APP_METADATA_URL_STRING
	if v, ok := GetAppMetadataPropertyOK(name); ok {
		t.Skipf("URL metadata preset to %q", v)
	}
	if !SetAppMetadataProperty(name, Ptr("https://example.org")) {
		t.Fatal(GetError())
	}
	if v, ok := GetAppMetadataPropertyOK(name); !ok || v != "https://example.org" {
		t.Errorf("metadata = (%q, %v)", v, ok)
	}
	if !SetAppMetadataProperty(name, nil) {
		t.Fatal(GetError())
	}
	if v, ok := GetAppMetadataPropertyOK(name); ok {
		t.Errorf("nil value must remove the property, got %q", v)
	}
}

func TestPtr(t *testing.T) {
	a := Ptr("x")
	b := Ptr("x")
	if a == b || *a != "x" {
		t.Error("Ptr must return a pointer to a fresh copy")
	}
}
