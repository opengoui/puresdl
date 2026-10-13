package bindgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ccfront"
	"github.com/opengoui/puresdl/internal/ir"
	"github.com/opengoui/puresdl/internal/sdlsync"
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
	for _, d := range []string{os.Getenv("SDL_INCLUDE"), pinnedInclude()} {
		if d == "" {
			continue
		}
		if _, err := os.Stat(d + "/SDL3/SDL.h"); err == nil {
			return d
		}
	}
	if os.Getenv("CI") != "" {
		t.Fatal("SDL3 headers not found on CI; run `go run ./cmd/sdlsync` or set SDL_INCLUDE")
	}
	t.Skip("SDL3 headers not found; run `go run ./cmd/sdlsync` or set SDL_INCLUDE")
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
	if _, err := Generate(api, Options{OutDir: out, HandDir: "../../sdl"}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	props, hints, init, stdinc := funcSigs(t, read("properties_gen.go")), funcSigs(t, read("hints_gen.go")), funcSigs(t, read("init_gen.go")), funcSigs(t, read("stdinc_gen.go"))
	for _, want := range []struct {
		sigs      map[string]string
		name, sig string
	}{
		{props, "SetStringProperty", "func(props PropertiesID, name string, value *string) bool"},
		{props, "GetStringProperty", "func(props PropertiesID, name string, default_value string) string"},
		{init, "SetAppMetadataProperty", "func(name string, value *string) bool"},
		{init, "GetAppMetadataPropertyOK", "func(name string) (string, bool)"},
		{hints, "GetHint", "func(name string) string"},
		{hints, "GetHintOK", "func(name string) (string, bool)"},
		{hints, "SetHint", "func(name string, value string) bool"},
		{stdinc, "GetEnvironmentVariableOK", "func(env *Environment, name string) (string, bool)"},
		{stdinc, "GetenvOK", "func(name string) (string, bool)"},
	} {
		if got := want.sigs[want.name]; got != want.sig {
			t.Errorf("%s = %q, want %q", want.name, got, want.sig)
		}
	}
	// Interchangeable parameters stay plain strings: no dialog function takes
	// default_location as a *string.
	dialog, err := parser.ParseFile(token.NewFileSet(), "dialog_gen.go", read("dialog_gen.go"), 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, d := range dialog.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		for _, fld := range fd.Type.Params.List {
			for _, n := range fld.Names {
				if n.Name == "default_location" {
					seen = true
					if _, isPtr := fld.Type.(*ast.StarExpr); isPtr {
						t.Errorf("%s: default_location is a pointer, want a plain string", fd.Name.Name)
					}
				}
			}
		}
	}
	if !seen {
		t.Error("no dialog function with a default_location parameter; the check is vacuous")
	}
}

// pinnedInclude returns the cached headers of the release in SDL_VERSION, or
// "" when they have not been synced.
func pinnedInclude() string {
	d, err := sdlsync.PinnedInclude("../..")
	if err != nil {
		return ""
	}
	return d
}

func TestClearByNullParamsSynthetic(t *testing.T) {
	api := &ir.API{Funcs: []*ir.Function{
		// Distinct entries are already *string and not reported.
		fn("SDL_SetStringProperty", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "value", Type: cstrType(true), Doc: "the value, or NULL to delete the property."}),
		fn("SDL_SetNewThing", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "label", Type: cstrType(true), Doc: "the label, or NULL to clear it."}),
		fn("SDL_Other", &ir.Type{Kind: ir.Bool}, "", ir.Param{Name: "path", Type: cstrType(true), Doc: "a path, may be NULL."}),
	}}
	got := ClearByNullParams(api)
	if len(got) != 1 || got[0] != "SDL_SetNewThing.label" {
		t.Errorf("got %q", got)
	}
}

// clearByNullAllowed lists the clear-by-NULL parameters that were reviewed
// and deliberately stay plain strings. A new SDL parameter must either be
// made distinct in nullParams or be added here after review.
var clearByNullAllowed = map[string]bool{}

func TestClearByNullParamsReviewed(t *testing.T) {
	api, _, err := ccfront.Load(ccfront.Options{IncludeDir: sdlInclude(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range ClearByNullParams(api) {
		if !clearByNullAllowed[k] {
			t.Errorf("%s clears something with NULL but is not a distinct *string parameter; classify it in nullParams or allow it here", k)
		}
	}
}

func TestValueFormGenerated(t *testing.T) {
	api, _, err := ccfront.Load(ccfront.Options{IncludeDir: sdlInclude(t)})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := Generate(api, Options{OutDir: out, HandDir: "../../sdl"}); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(out, "init_gen.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	sigs := map[string]*ast.FuncType{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			sigs[fd.Name.Name] = fd.Type
		}
	}
	names := func(fl *ast.FieldList) (n int, types []string) {
		for _, fld := range fl.List {
			c := max(1, len(fld.Names))
			for range c {
				types = append(types, types2s(fld.Type))
			}
			n += c
		}
		return
	}
	if ft := sigs["SetAppMetadataPropertyValue"]; ft == nil {
		t.Error("SetAppMetadataPropertyValue not generated")
	} else if _, ty := names(ft.Params); len(ty) != 2 || ty[0] != "string" || ty[1] != "string" {
		t.Errorf("SetAppMetadataPropertyValue params = %v", ty)
	}
	if ft := sigs["ClearAppMetadataProperty"]; ft == nil {
		t.Error("ClearAppMetadataProperty not generated")
	} else if _, ty := names(ft.Params); len(ty) != 1 || ty[0] != "string" {
		t.Errorf("ClearAppMetadataProperty params = %v", ty)
	}
}

func types2s(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}
