package bindgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ccfront"
)

// TestLayoutTestSkipsOn32Bit checks that the generated layout test guards
// against 32-bit targets, whose struct sizes differ from the 64-bit C layout.
func TestLayoutTestSkipsOn32Bit(t *testing.T) {
	api, _, err := ccfront.Load(ccfront.Options{IncludeDir: sdlInclude(t)})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := Generate(api, Options{OutDir: out, HandDir: "../../sdl"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "layout_gen_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	guard := strings.Index(src, "t.Skip(")
	first := strings.Index(src, "checkLayout(")
	if guard < 0 || first < 0 || guard > first {
		t.Errorf("layout test lacks a 32-bit skip before the first check (skip at %d, check at %d)", guard, first)
	}
}
