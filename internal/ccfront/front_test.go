package ccfront

import (
	"os"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ir"
)

// includeDir locates the SDL3 headers; tests skip when they are absent.
func includeDir(t *testing.T) string {
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

func TestLoad(t *testing.T) {
	api, warns, err := Load(Options{IncludeDir: includeDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warns {
		t.Log("warning:", w)
	}
	t.Logf("version=%s funcs=%d typedefs=%d enums=%d records=%d consts=%d", api.Version, len(api.Funcs), len(api.Typedefs), len(api.Enums), len(api.Records), len(api.Consts))
	fn := map[string]*ir.Function{}
	for _, f := range api.Funcs {
		fn[f.Name] = f
	}
	f := fn["SDL_CreateWindow"]
	if f == nil || len(f.Sig.Params) != 4 || !strings.Contains(f.Doc, "Create a window") || f.Sig.Params[0].Doc == "" {
		t.Fatalf("SDL_CreateWindow: %+v", f)
	}
	if p := f.Sig.Params[0].Type; p.Kind != ir.Pointer || !p.Const || p.Elem.Resolve().Char != true {
		t.Errorf("title param is not const char*: %+v", p)
	}
	rec := map[string]*ir.Record{}
	for _, r := range api.Records {
		if r.Name != "" {
			rec[r.Name] = r
		}
	}
	if r := rec["SDL_Event"]; r == nil || !r.Union || r.Size != 128 || r.Align != 8 {
		t.Errorf("SDL_Event: %+v", r)
	}
	if r := rec["SDL_Surface"]; r == nil || r.Size != 48 || r.Fields[0].Name != "flags" {
		t.Errorf("SDL_Surface: %+v", r)
	}
	var found bool
	for _, k := range api.Consts {
		if k.Name == "SDL_INIT_VIDEO" && k.Int.Int64() == 0x20 {
			found = true
		}
	}
	if !found {
		t.Error("SDL_INIT_VIDEO missing")
	}
}
