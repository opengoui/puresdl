package bindgen

import (
	"regexp"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// countParamRe matches the names SDL gives the element-count parameter that
// follows an input array.
var countParamRe = regexp.MustCompile(`^(count|num[_a-z]*|n[a-z]*s)$`)

// successRe matches the "true on success" wording of a bool return.
var successRe = regexp.MustCompile(`(?i)true on success`)

// sliceForm describes a function with a (const T *ptr, int count) pair that
// also gets a Go slice form.
type sliceForm struct {
	ptr, cnt int // parameter indexes; cnt == ptr+1
	// opt is the index of a second, optional input array whose count follows
	// it (the indices of RenderGeometry), or -1. An empty optional slice is
	// passed as NULL with a zero count.
	opt int
}

// sliceFormOf returns the slice form of f, or nil when the function does not
// match. The pair must be an input array (a pointer to const, non-void,
// non-char scalar or aggregate elements) immediately followed by an int32
// count parameter with a count-like name. Arrays that may be NULL or that SDL
// retains are skipped, as are functions with a second array pair, except for a
// second input array of the same shape that is documented as optional (it may
// be NULL), which becomes a second slice that may be empty. Functions where another const
// pointer directly precedes the array are skipped: that pointer is likely to
// share the count, so the slice length would be ambiguous.
func sliceFormOf(f *ir.Function, plans []argPlan) *sliceForm {
	if handWritten[f.Name] || f.Sig.Variadic {
		return nil
	}
	ret := f.Sig.Ret.Resolve()
	switch ret.Kind {
	case ir.Void, ir.Bool, ir.Int, ir.Float:
	default:
		return nil
	}
	// "true if ..." results (GetRectEnclosingPoints) would report the wrong
	// answer for an empty slice, so only the standard success/failure result
	// is eligible.
	if ret.Kind == ir.Bool && !successRe.MatchString(f.Doc) {
		return nil
	}
	ps := f.Sig.Params
	for i := 0; i+1 < len(ps); i++ {
		pt := ps[i].Type.Resolve()
		if pt.Kind != ir.Pointer || !pt.Const || isCString(ps[i].Type) {
			continue
		}
		switch pt.Elem.Resolve().Kind {
		case ir.Struct, ir.Int, ir.Float, ir.Enum:
		default:
			continue
		}
		ct := ps[i+1].Type.Resolve()
		if ct.Kind != ir.Int || ct.Size != 4 || !ct.Signed || !countParamRe.MatchString(ps[i+1].Name) {
			continue
		}
		// An empty slice is a no-op, which is wrong when NULL is a
		// meaningful argument, and SDL must not keep the memory afterwards.
		if nullable(ps[i].Doc) || retainedRe.MatchString(ps[i].Doc) || retainedRe.MatchString(f.Doc) {
			return nil
		}
		if i > 0 {
			if prev := ps[i-1].Type.Resolve(); prev.Kind == ir.Pointer && prev.Const {
				return nil
			}
		}
		opt := -1
		for j := i + 2; j+1 < len(ps); j++ {
			if q := ps[j].Type.Resolve(); q.Kind == ir.Pointer && !isCString(ps[j].Type) && ps[j+1].Type.Resolve().Kind == ir.Int && countParamRe.MatchString(ps[j+1].Name) {
				if opt >= 0 || !optionalArray(ps, plans, j) {
					return nil // another array pair: leave both to the raw wrapper
				}
				opt = j
			}
		}
		if plans[i].hidden || plans[i].pub == nil || plans[i+1].pub == nil {
			return nil
		}
		return &sliceForm{ptr: i, cnt: i + 1, opt: opt}
	}
	return nil
}

// optionalArray reports whether ps[j], ps[j+1] is an input array with an int32
// count that the documentation allows to be NULL, so that an empty slice maps
// to NULL with a zero count.
func optionalArray(ps []ir.Param, plans []argPlan, j int) bool {
	pt := ps[j].Type.Resolve()
	if pt.Kind != ir.Pointer || !pt.Const || !nullable(ps[j].Doc) || retainedRe.MatchString(ps[j].Doc) {
		return false
	}
	switch pt.Elem.Resolve().Kind {
	case ir.Struct, ir.Int, ir.Float, ir.Enum:
	default:
		return false
	}
	ct := ps[j+1].Type.Resolve()
	return ct.Kind == ir.Int && ct.Size == 4 && ct.Signed && !plans[j].hidden && plans[j].pub != nil && plans[j+1].pub != nil
}

// sliceNote is the doc note of the raw wrapper that points at its slice form.
func sliceNote(name string) string {
	return "Passing &s[0] of an empty slice panics and the count must match the slice length; " + name + "Slice takes a slice instead."
}

// emitSlice emits <name>Slice for f, which takes the array as a Go slice and
// derives the count from its length.
func (g *gen) emitSlice(f *ir.Function, sf *sliceForm, plans []argPlan, name, fnVar string, fb *fileBuf, rawDoc []string) {
	sliceName := name + "Slice"
	if !g.claim(sliceName, f.Name+" (slice form)") {
		return
	}
	ps := f.Sig.Params
	ptr := plans[sf.ptr]
	elem, err := g.typ(ps[sf.ptr].Type.Resolve().Elem)
	if err != nil {
		return
	}
	var params, args []jen.Code
	var optBody []jen.Code
	if sf.opt >= 0 {
		oplan := plans[sf.opt]
		oelem, err := g.typ(ps[sf.opt].Type.Resolve().Elem)
		if err != nil {
			return
		}
		on := oplan.name
		optBody = []jen.Code{
			jen.Var().Id(on + "Ptr").Op("*").Add(oelem),
			jen.If(jen.Len(jen.Id(on)).Op(">").Lit(0)).Block(
				jen.If(jen.Op("!").Id("fitsInt32").Call(jen.Len(jen.Id(on)), jen.Lit(sliceName))).Block(optFail(f.Sig.Ret.Resolve().Kind)),
				jen.Id(on+"Ptr").Op("=").Op("&").Id(on).Index(jen.Lit(0))),
		}
	}
	for i, p := range plans {
		switch i {
		case sf.opt:
			oelem, _ := g.typ(ps[sf.opt].Type.Resolve().Elem)
			params = append(params, jen.Id(p.name).Index().Add(oelem))
			args = append(args, jen.Id(p.name+"Ptr"))
			continue
		case sf.opt + 1:
			if sf.opt >= 0 {
				args = append(args, jen.Int32().Call(jen.Len(jen.Id(plans[sf.opt].name))))
				continue
			}
		}
		switch i {
		case sf.cnt:
			args = append(args, jen.Int32().Call(jen.Len(jen.Id(ptr.name))))
			continue
		case sf.ptr:
			params = append(params, jen.Id(ptr.name).Index().Add(elem))
			args = append(args, jen.Op("&").Id(ptr.name).Index(jen.Lit(0)))
			continue
		}
		params = append(params, jen.Id(p.name).Add(p.pub))
		args = append(args, p.expr)
	}
	ret := f.Sig.Ret.Resolve()
	var retT *jen.Statement
	var empty []jen.Code
	emptyDoc, failDoc := "returns without calling SDL", "returns"
	switch ret.Kind {
	case ir.Void:
	case ir.Bool:
		retT = jen.Bool()
		empty = []jen.Code{jen.True()}
		emptyDoc, failDoc = "returns true without calling SDL", "returns false"
	default:
		t, err := g.typ(f.Sig.Ret)
		if err != nil {
			return
		}
		retT = t
		empty = []jen.Code{jen.Lit(0)}
		emptyDoc, failDoc = "returns zero without calling SDL", "returns zero"
	}
	fail := []jen.Code{jen.Return()}
	if retT != nil {
		fail = []jen.Code{jen.Return(jen.Lit(0))}
		if ret.Kind == ir.Bool {
			fail = []jen.Code{jen.Return(jen.False())}
		}
	}
	s := jen.Id(ptr.name)
	callName := fnVar
	if fast, ok := fastSliceCalls[f.Name]; ok {
		callName = fast
	}
	call := jen.Id(callName).Call(args...)
	body := []jen.Code{
		jen.If(jen.Len(s).Op("==").Lit(0)).Block(jen.Return(empty...)),
		jen.If(jen.Op("!").Id("fitsInt32").Call(jen.Len(s), jen.Lit(sliceName))).Block(fail...),
	}
	body = append(body, optBody...)
	keep := []jen.Code{jen.Qual("runtime", "KeepAlive").Call(s)}
	if sf.opt >= 0 {
		keep = append(keep, jen.Qual("runtime", "KeepAlive").Call(jen.Id(plans[sf.opt].name)))
	}
	if retT == nil {
		body = append(body, call)
		body = append(body, keep...)
	} else {
		body = append(body, jen.Id("ret").Op(":=").Add(call))
		body = append(body, keep...)
		body = append(body, jen.Return(jen.Id("ret")))
	}
	doc := []string{
		sliceName + " is the slice form of " + name + ": it takes " + ptr.name + " as a Go slice and derives the " + ps[sf.cnt].Name + " argument from its length.",
		"",
		"An empty or nil " + ptr.name + " slice is a no-op that " + emptyDoc + ". A slice with more than MaxInt32 elements sets the SDL error and " + failDoc + ".",
		"",
		"See also: " + name,
	}
	if sf.opt >= 0 {
		on := plans[sf.opt].name
		doc = append(doc[:2], append([]string{"An empty or nil " + on + " slice is passed as NULL with a zero " + ps[sf.opt+1].Name + ", which SDL documents as the optional case.", ""}, doc[2:]...)...)
	}
	w := start(doc).Func().Id(sliceName).Params(params...)
	if retT != nil {
		w = w.Add(retT)
	}
	fb.funcs = append(fb.funcs, w.Block(body...))
}

// optFail is the statement a slice form runs when its optional slice is too
// long for an int32 count: it returns the failure value of the result kind.
func optFail(k ir.Kind) jen.Code {
	switch k {
	case ir.Bool:
		return jen.Return(jen.False())
	case ir.Void:
		return jen.Return()
	}
	return jen.Return(jen.Lit(0))
}
