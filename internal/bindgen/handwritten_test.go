package bindgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandwrittenSymbolsExist fails when a hand-written function that the
// generated documentation refers to by name is missing from package sdl.
func TestHandwrittenSymbolsExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "sdl", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no sdl sources found: %v", err)
	}
	funcs := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				funcs[fd.Name.Name] = true
			}
		}
	}
	for _, name := range []string{"NewUserdataHandle", "UserdataHandleValue", "FreeUserdataHandle"} {
		if !funcs[name] {
			t.Errorf("hand-written sdl.%s is missing but generated docs refer to it", name)
		}
	}
}
