package bindgen

import (
	"regexp"
	"strings"

	"github.com/opengoui/puresdl/internal/ir"
)

// scratchBytes matches the call frame's scratch area in sdl/call.go.
const scratchBytes = 512

// scratchRecords lists the plain value structs and unions whose single-object
// pointer parameters a direct call copies into the call frame's scratch area
// instead of passing the caller's address. Passing Go memory to C moves it to
// the heap (go:uintptrescapes), so without the copy every
// RenderFillRect(r, &FRect{...}) or PollEvent(&ev) allocates. The copy is
// only correct for types that SDL treats as values: never for objects whose
// address matters (SDL_AtomicInt, SDL_TLSID, SDL_InitState, SDL-owned
// SDL_PixelFormatDetails ...), which is why this is a list and not a rule.
// Every listed type must be free of Go pointers (checked by scratchParams),
// so the scratch area never hides a pointer from the garbage collector.
var scratchRecords = map[string]bool{
	"SDL_Rect":        true,
	"SDL_FRect":       true,
	"SDL_Point":       true,
	"SDL_FPoint":      true,
	"SDL_Color":       true,
	"SDL_FColor":      true,
	"SDL_Event":       true,
	"SDL_AudioSpec":   true,
	"SDL_DateTime":    true,
	"SDL_GPUViewport": true,
}

// arrayDocRe matches parameter docs that describe an array argument.
var arrayDocRe = regexp.MustCompile(`(?i)\barray\b|\blist of\b`)

// scratchMode says how a direct call passes a pointer parameter.
type scratchMode int

const (
	scratchNone  scratchMode = iota // the caller's address (moved to the heap)
	scratchIn                       // a copy in the scratch area (const pointer)
	scratchInOut                    // a copy that is copied back after the call
)

// scratchParam is the scratch layout of one parameter.
type scratchParam struct {
	mode scratchMode
	off  int64
}

// scratchParams returns the scratch layout of every parameter of the direct
// function f, or nil when none is copied. A parameter is copied when it
// points to a single object of a type in scratchRecords: a parameter followed
// by an element count, with a plural name or with "array" in its doc is an
// array, and a pointer that SDL retains after the call must keep its address.
func scratchParams(f *ir.Function) []scratchParam {
	var out []scratchParam
	var off int64
	ps := f.Sig.Params
	for i, p := range ps {
		pt := p.Type.Resolve()
		if pt.Kind != ir.Pointer {
			continue
		}
		et := pt.Elem
		r := et.Resolve()
		if et.Kind != ir.Named || !scratchRecords[et.Name] || r.Rec == nil || r.Rec.Incomplete {
			continue
		}
		if !goPointerFree(r) {
			panic("bindgen: scratch record " + et.Name + " contains Go pointers")
		}
		if strings.HasSuffix(p.Name, "s") || arrayDocRe.MatchString(p.Doc) || retainedParam(f, p) {
			continue
		}
		if i+1 < len(ps) {
			if ct := ps[i+1].Type.Resolve(); (ct.Kind == ir.Int || ct.Kind == ir.Enum) && countParamRe.MatchString(ps[i+1].Name) {
				continue
			}
		}
		size := (r.Rec.Size + 7) &^ 7
		if off+size > scratchBytes {
			continue
		}
		if out == nil {
			out = make([]scratchParam, len(ps))
		}
		mode := scratchInOut
		if pt.Const {
			mode = scratchIn
		}
		out[i] = scratchParam{mode: mode, off: off}
		off += size
	}
	return out
}

// goPointerFree reports whether the Go type that t maps to contains no Go
// pointers: function pointers map to uintptr and unions to word arrays.
func goPointerFree(t *ir.Type) bool {
	t = t.Resolve()
	switch t.Kind {
	case ir.Pointer:
		e := t.Elem.Resolve()
		return e != nil && e.Kind == ir.Func
	case ir.Array:
		return goPointerFree(t.Elem)
	case ir.Union:
		return true
	case ir.Struct:
		for _, f := range t.Rec.Fields {
			if !goPointerFree(f.Type) {
				return false
			}
		}
	}
	return true
}
