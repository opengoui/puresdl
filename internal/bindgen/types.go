package bindgen

import (
	"fmt"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// builtinNames are C typedef names that map straight to Go predeclared
// types instead of becoming named Go types.
var builtinNames = map[string]string{
	"Uint8": "uint8", "Uint16": "uint16", "Uint32": "uint32", "Uint64": "uint64",
	"Sint8": "int8", "Sint16": "int16", "Sint32": "int32", "Sint64": "int64",
	"uint8_t": "uint8", "uint16_t": "uint16", "uint32_t": "uint32", "uint64_t": "uint64",
	"int8_t": "int8", "int16_t": "int16", "int32_t": "int32", "int64_t": "int64",
	"size_t": "uint", "ptrdiff_t": "int", "intptr_t": "int", "uintptr_t": "uintptr",
	"intmax_t": "int64", "uintmax_t": "uint64",
	// Wchar is defined by hand-written build-tagged files in package sdl
	// (int32, or uint16 on Windows).
	"wchar_t": "Wchar",
}

// unsupportedNames are typedefs the generator cannot express portably.
var unsupportedNames = map[string]bool{"va_list": true, "__builtin_va_list": true}

func intName(size int64, signed bool) string {
	switch size {
	case 1:
		if signed {
			return "int8"
		}
		return "uint8"
	case 2:
		if signed {
			return "int16"
		}
		return "uint16"
	case 4:
		if signed {
			return "int32"
		}
		return "uint32"
	}
	if signed {
		return "int64"
	}
	return "uint64"
}

// basicName is the predeclared Go type of a scalar ir type.
func basicName(t *ir.Type) (string, bool) {
	switch t.Kind {
	case ir.Bool:
		return "bool", true
	case ir.Int:
		if t.Char {
			return "byte", true
		}
		return intName(t.Size, t.Signed), true
	case ir.Float:
		if t.Size == 4 {
			return "float32", true
		}
		return "float64", true
	}
	return "", false
}

// recGoName returns the Go name of a record that is emitted.
func (g *gen) recGoName(r *ir.Record) (string, bool) {
	n, ok := g.recName[r]
	return n, ok
}

// typ maps a C type to a Go type expression.
func (g *gen) typ(t *ir.Type) (*jen.Statement, error) {
	switch t.Kind {
	case ir.Named:
		if unsupportedNames[t.Name] {
			return nil, fmt.Errorf("unsupported type %s", t.Name)
		}
		if b, ok := builtinNames[t.Name]; ok {
			return jen.Id(b), nil
		}
		if n, ok := g.tdGo[t.Name]; ok {
			return jen.Id(n), nil
		}
		// a typedef that is not emitted (libc or platform type): use its
		// underlying type
		return g.typ(t.Under)
	case ir.Void:
		return nil, fmt.Errorf("void value")
	case ir.Bool, ir.Int, ir.Float:
		n, _ := basicName(t)
		return jen.Id(n), nil
	case ir.Pointer:
		return g.pointerTo(t.Elem)
	case ir.Array:
		e, err := g.typ(t.Elem)
		if err != nil {
			return nil, err
		}
		return jen.Index(jen.Lit(int(t.Len))).Add(e), nil
	case ir.Struct, ir.Union:
		n, ok := g.recGoName(t.Rec)
		if !ok {
			return nil, fmt.Errorf("record %q is not emitted", t.Rec.Tag)
		}
		return jen.Id(n), nil
	case ir.Enum:
		if n, ok := g.enumName[t.Enum]; ok {
			return jen.Id(n), nil
		}
		return jen.Id(intName(t.Size, t.Signed)), nil
	}
	return nil, fmt.Errorf("unsupported C type %q", t.Name)
}

func (g *gen) pointerTo(elem *ir.Type) (*jen.Statement, error) {
	r := elem.Resolve()
	if elem.Kind == ir.Named && unsupportedNames[elem.Name] {
		return nil, fmt.Errorf("unsupported type %s", elem.Name)
	}
	switch r.Kind {
	case ir.Func:
		return jen.Id("uintptr"), nil
	case ir.Void, ir.Invalid:
		return jen.Qual("unsafe", "Pointer"), nil
	case ir.Int:
		if r.Char {
			return jen.Op("*").Id("byte"), nil
		}
	case ir.Struct, ir.Union:
		if _, ok := g.recGoName(r.Rec); !ok {
			return jen.Qual("unsafe", "Pointer"), nil
		}
	}
	e, err := g.typ(elem)
	if err != nil {
		return nil, err
	}
	return jen.Op("*").Add(e), nil
}

// isCString reports whether t is const char * (or char *).
func isCString(t *ir.Type) bool {
	r := t.Resolve()
	if r == nil || r.Kind != ir.Pointer {
		return false
	}
	e := r.Elem.Resolve()
	return e != nil && e.Kind == ir.Int && e.Char
}

// isFuncPtr reports whether t is a pointer to function.
func isFuncPtr(t *ir.Type) bool {
	r := t.Resolve()
	return r != nil && r.Kind == ir.Pointer && r.Elem.Resolve().Kind == ir.Func
}

// isWcharString reports whether t is const wchar_t *.
func isWcharString(t *ir.Type) bool {
	r := t.Resolve()
	if r == nil || r.Kind != ir.Pointer || !r.Const {
		return false
	}
	return r.Elem.Kind == ir.Named && r.Elem.Name == "wchar_t"
}
