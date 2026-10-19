package bindgen

import (
	"fmt"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// maxDirectArgs matches the call0 ... call8 helpers in sdl/call.go. Up to
// eight integer arguments travel in registers on every supported ABI; Apple
// arm64 packs further stack arguments by their natural size, which
// purego.SyscallN does not do.
const maxDirectArgs = 8

// wordKind classifies a C value that is passed or returned in one integer
// register.
type wordKind int

const (
	wordNone    wordKind = iota // not a single integer word (float, struct by value)
	wordInt                     // integer or enum, including named typedefs
	wordBool                    // C bool
	wordPtr                     // data pointer: *T, *byte or unsafe.Pointer
	wordFuncPtr                 // function pointer, uintptr in Go
)

// wordKindOf returns how t crosses the C boundary as one integer word.
func wordKindOf(t *ir.Type) wordKind {
	r := t.Resolve()
	if r == nil {
		return wordNone
	}
	switch r.Kind {
	case ir.Bool:
		return wordBool
	case ir.Int, ir.Enum:
		return wordInt
	case ir.Pointer:
		if e := r.Elem.Resolve(); e != nil && e.Kind == ir.Func {
			return wordFuncPtr
		}
		return wordPtr
	}
	return wordNone
}

// directCall reports whether f is called through the allocation-free
// call0 ... call8 bridge instead of a purego.RegisterFunc wrapper: every
// parameter and the result must be one integer word, there are at most
// maxDirectArgs parameters, and f is not variadic. Floating-point values need
// the float registers that only RegisterFunc fills, and structs passed by
// value need its ABI classification.
func directCall(f *ir.Function) bool {
	sig := f.Sig
	if sig.Variadic || len(sig.Params) > maxDirectArgs {
		return false
	}
	for _, p := range sig.Params {
		if wordKindOf(p.Type) == wordNone || isGUID(p.Type) {
			return false
		}
	}
	if r := sig.Ret.Resolve(); r.Kind != ir.Void && (wordKindOf(sig.Ret) == wordNone || isGUID(sig.Ret)) {
		return false
	}
	return true
}

// emitDirect emits fn<Name> as a function that calls the C symbol at
// addr<Name> through callN, and the addr<Name> variable that bindAll fills.
// Its signature equals the raw function variable it replaces, so every
// wrapper calls it the same way.
func (g *gen) emitDirect(fb *fileBuf, f *ir.Function, fnVar, addrVar string, plans []argPlan, rawParams []jen.Code, rawRet *jen.Statement) {
	args := []jen.Code{jen.Id(addrVar)}
	for i, p := range f.Sig.Params {
		name := jen.Id(plans[i].name)
		switch wordKindOf(p.Type) {
		case wordBool:
			args = append(args, jen.Id("cbool").Call(name))
		case wordPtr:
			args = append(args, jen.Uintptr().Call(jen.Qual("unsafe", "Pointer").Call(name)))
		default:
			args = append(args, jen.Uintptr().Call(name))
		}
	}
	call := jen.Id(fmt.Sprintf("call%d", len(f.Sig.Params))).Call(args...)
	body := []jen.Code{
		jen.If(jen.Id(addrVar).Op("==").Lit(0)).Block(jen.Id("missingSymbol").Call(jen.Lit(f.Name))),
	}
	switch {
	case rawRet == nil:
		body = append(body, call)
	case wordKindOf(f.Sig.Ret) == wordBool:
		// C defines only the low byte of a bool result.
		body = append(body, jen.Return(jen.Uint8().Call(call).Op("!=").Lit(0)))
	case wordKindOf(f.Sig.Ret) == wordPtr:
		p := jen.Id("cptr").Call(call)
		if fmt.Sprintf("%#v", rawRet) == "unsafe.Pointer" {
			body = append(body, jen.Return(p))
		} else {
			body = append(body, jen.Return(jen.Parens(rawRet.Clone()).Call(p)))
		}
	default:
		body = append(body, jen.Return(rawRet.Clone().Call(call)))
	}
	fb.funcs = append(fb.funcs, jen.Var().Id(addrVar).Uintptr())
	fn := jen.Func().Id(fnVar).Params(rawParams...)
	if rawRet != nil {
		fn = fn.Add(rawRet)
	}
	fb.funcs = append(fb.funcs, fn.Block(body...))
}
