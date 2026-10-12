package bindgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ccfront"
	"github.com/opengoui/puresdl/internal/ir"
)

func cstrType(konst bool) *ir.Type {
	return &ir.Type{Kind: ir.Pointer, Size: 8, Const: konst, Elem: &ir.Type{Kind: ir.Int, Size: 1, Char: true, Signed: true}}
}

func fn(name string, ret *ir.Type, doc string, params ...ir.Param) *ir.Function {
	return &ir.Function{Name: name, File: "SDL_x.h", Doc: doc, Sig: &ir.Sig{Ret: ret, Params: params}}
}

func TestCheckNullPolicySynthetic(t *testing.T) {
	api := &ir.API{Funcs: []*ir.Function{
		// Classified entries are accepted.
		fn("SDL_SetStringProperty", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "value", Type: cstrType(true), Doc: "the new value, or NULL to delete the property."}),
		fn("SDL_GetHint", cstrType(true), "\\returns the value or NULL if the hint isn't set."),
		// New nullable parameters and results are reported.
		fn("SDL_NewThing", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "title", Type: cstrType(true), Doc: "a title, may be NULL."}),
		fn("SDL_NewName", cstrType(true), "\\returns a name, or NULL on failure. \\since 3.6"),
		// "must not be NULL" and NULL-terminated are not nullable.
		fn("SDL_Strict", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "s", Type: cstrType(true), Doc: "a string that must not be NULL."}),
		fn("SDL_Terminated", cstrType(true), "\\returns a NULL-terminated string."),
	}}
	got := CheckNullPolicy(api)
	var unclassified, stale int
	for _, m := range got {
		switch {
		case strings.HasPrefix(m, "unclassified nullable parameter SDL_NewThing.title"):
			unclassified++
		case strings.HasPrefix(m, "unclassified NULL-able string result SDL_NewName"):
			unclassified++
		case strings.HasPrefix(m, "stale"):
			stale++
		default:
			t.Errorf("unexpected message %q", m)
		}
	}
	if unclassified != 2 {
		t.Errorf("want both new declarations reported, got %q", got)
	}
	if stale == 0 {
		t.Error("table entries missing from the API must be reported as stale")
	}
}

func sdlInclude(t *testing.T) string {
	t.Helper()
	for _, d := range []string{os.Getenv("SDL_INCLUDE"), "../../../SDL/include", "/opt/homebrew/include"} {
		if d == "" {
			continue
		}
		if _, err := os.Stat(d + "/SDL3/SDL.h"); err == nil {
			return d
		}
	}
	t.Skip("SDL3 headers not found; set SDL_INCLUDE")
	return ""
}

// TestNullPolicyCoversHeaders fails when the SDL headers declare a nullable
// string parameter or result that nullParams / the result tables do not
// classify, so an SDL upgrade is reviewed. It also checks the generated
// signatures of the classified entries.
func TestNullPolicyCoversHeaders(t *testing.T) {
	api, _, err := ccfront.Load(ccfront.Options{IncludeDir: sdlInclude(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range CheckNullPolicy(api) {
		t.Error(m)
	}
	out := t.TempDir()
	if _, err := Generate(api, Options{OutDir: out}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	props, hints, init, stdinc := read("properties_gen.go"), read("hints_gen.go"), read("init_gen.go"), read("stdinc_gen.go")
	for _, want := range []struct{ src, sig string }{
		{props, "func SetStringProperty(props PropertiesID, name string, value *string) bool"},
		{props, "return fnSetStringProperty(props, cstr(name), cstrPtr(value))"},
		{props, "func GetStringProperty(props PropertiesID, name string, default_value string) string"},
		{init, "func SetAppMetadataProperty(name string, value *string) bool"},
		{init, "func GetAppMetadataPropertyOK(name string) (string, bool)"},
		{hints, "func GetHint(name string) string"},
		{hints, "func GetHintOK(name string) (string, bool)"},
		{hints, "func SetHint(name string, value string) bool"},
		{stdinc, "func GetEnvironmentVariableOK(env *Environment, name string) (string, bool)"},
		{stdinc, "func GetenvOK(name string) (string, bool)"},
	} {
		if !strings.Contains(want.src, want.sig) {
			t.Errorf("generated code lacks %q", want.sig)
		}
	}
	if strings.Contains(read("dialog_gen.go"), "default_location *string") {
		t.Error("interchangeable parameters must stay plain strings")
	}
}
