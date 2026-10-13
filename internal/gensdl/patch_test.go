package gensdl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x_gen.go")
	if err := os.WriteFile(path, []byte("package sdl\n\nfunc A() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := applyPatches(dir, []Patch{{Name: "a", File: "x_gen.go", Apply: Replace("return 1", "return 2")}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "return 2") {
		t.Errorf("patch not applied:\n%s", b)
	}
	for name, p := range map[string]Patch{
		"missing text": {Name: "m", File: "x_gen.go", Apply: Replace("nope", "x")},
		"missing file": {Name: "f", File: "y_gen.go", Apply: Replace("a", "b")},
		"bad syntax":   {Name: "s", File: "x_gen.go", Apply: Replace("return 2", "return (")},
	} {
		if err := applyPatches(dir, []Patch{p}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
