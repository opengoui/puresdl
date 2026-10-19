package bindgen

import (
	"fmt"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// maxDirectArgs matches call0 ... call8 and callF0 ... callF8 in
// sdl/call.go: up to eight integer and, separately, eight floating-point
// arguments travel in registers on every ABI that the trampoline supports;
// Apple arm64 packs further stack arguments by their natural size, which a
// uniform word array cannot express.
const maxDirectArgs = 8

// wordKind classifies a C value that is passed or returned in one register.
type wordKind int

const (
	wordNone    wordKind = iota // not a single register value (struct by value, long double)
	wordInt                     // integer or enum, including named typedefs
	wordBool                    // C bool
	wordPtr                     // data pointer: *T, *byte or unsafe.Pointer
	wordFuncPtr                 // function pointer, uintptr in Go
	wordFloat32                 // C float, in a floating-point register
	wordFloat64                 // C double, in a floating-point register
)

// isFloat reports whether k travels in a floating-point register.
func (k wordKind) isFloat() bool { return k == wordFloat32 || k == wordFloat64 }

// wordKindOf returns how t crosses the C boundary as one register value.
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
	case ir.Float:
		switch r.Size {
		case 4:
			return wordFloat32
		case 8:
			return wordFloat64
		}
	case ir.Pointer:
		if e := r.Elem.Resolve(); e != nil && e.Kind == ir.Func {
			return wordFuncPtr
		}
		return wordPtr
	}
	return wordNone
}

// directCall reports whether f is called through the allocation-free
// call0 ... call8 or callF0 ... callF8 bridge instead of a
// purego.RegisterFunc wrapper: every parameter and the result must be one
// register value, there are at most maxDirectArgs integer and maxDirectArgs
// floating-point parameters, and f is not variadic. Structs passed by value
// need the ABI classification that only RegisterFunc implements.
func directCall(f *ir.Function) bool {
	sig := f.Sig
	if sig.Variadic {
		return false
	}
	ints, floats := 0, 0
	for _, p := range sig.Params {
		k := wordKindOf(p.Type)
		if k == wordNone || isGUID(p.Type) {
			return false
		}
		if k.isFloat() {
			floats++
		} else {
			ints++
		}
	}
	if ints > maxDirectArgs || floats > maxDirectArgs {
		return false
	}
	if r := sig.Ret.Resolve(); r.Kind != ir.Void && (wordKindOf(sig.Ret) == wordNone || isGUID(sig.Ret)) {
		return false
	}
	return true
}

// floatSig reports whether a direct-call function passes or returns a
// floating-point value, so that it uses callFN and needs a purego fallback.
func floatSig(f *ir.Function) bool {
	if r := f.Sig.Ret.Resolve(); r.Kind != ir.Void && wordKindOf(f.Sig.Ret).isFloat() {
		return true
	}
	for _, p := range f.Sig.Params {
		if wordKindOf(p.Type).isFloat() {
			return true
		}
	}
	return false
}

// emitDirect emits fn<Name> as a function that calls the C symbol at
// addr<Name> through callN (or callFN when floating-point values are
// involved), and the addr<Name> variable that bindAll fills. Its signature
// equals the raw function variable it replaces, so every wrapper calls it the
// same way. A floating-point function also gets a reg<Name> function
// variable, which bindAll binds with purego.RegisterFunc on platforms where
// floatCalls is false; fn<Name> calls it there. It returns that variable's
// name, or "" when there is none.
func (g *gen) emitDirect(fb *fileBuf, f *ir.Function, fnVar, addrVar string, plans []argPlan, rawParams []jen.Code, rawRet *jen.Statement) string {
	float := floatSig(f)
	args := []jen.Code{jen.Id(addrVar)}
	var floats, names []jen.Code
	// fmask has bit i set when parameter i is floating-point; the Microsoft
	// x64 ABI assigns registers by position and needs it.
	fmask := 0
	for i, p := range f.Sig.Params {
		name := jen.Id(plans[i].name)
		names = append(names, jen.Id(plans[i].name))
		switch wordKindOf(p.Type) {
		case wordBool:
			args = append(args, jen.Id("cbool").Call(name))
		case wordPtr:
			args = append(args, jen.Uintptr().Call(jen.Qual("unsafe", "Pointer").Call(name)))
		case wordFloat32:
			floats = append(floats, jen.Id("f32").Call(name))
			fmask |= 1 << i
		case wordFloat64:
			floats = append(floats, jen.Id("f64").Call(name))
			fmask |= 1 << i
		default:
			args = append(args, jen.Uintptr().Call(name))
		}
	}
	var call *jen.Statement
	if float {
		args = append([]jen.Code{args[0], jen.Op(fmt.Sprintf("0b%b", fmask)), jen.Op("&").Id("floatArgs").Values(floats...)}, args[1:]...)
		call = jen.Id(fmt.Sprintf("callF%d", len(args)-3)).Call(args...)
	} else {
		call = jen.Id(fmt.Sprintf("call%d", len(args)-1)).Call(args...)
	}
	var body []jen.Code
	regVar := ""
	if float {
		regVar = "reg" + strings.TrimPrefix(fnVar, "fn")
		fallback := jen.Id(regVar).Call(names...)
		if rawRet != nil {
			fallback = jen.Return(fallback)
		} else {
			fallback = jen.Add(fallback).Line().Return()
		}
		body = append(body, jen.If(jen.Op("!").Id("floatCalls")).Block(fallback))
	}
	body = append(body, jen.If(jen.Id(addrVar).Op("==").Lit(0)).Block(jen.Id("missingSymbol").Call(jen.Lit(f.Name))))
	// rName and frName hold the result registers of callFN; they must not
	// shadow a parameter.
	rName, frName := freshName("r", plans), freshName("fr", plans)
	// intResult is the integer result register of call.
	intResult := call
	if float {
		intResult = jen.Id(rName)
	}
	retKind := wordKindOf(f.Sig.Ret)
	switch {
	case rawRet == nil:
		body = append(body, call)
	case retKind.isFloat():
		conv := "retF32"
		if retKind == wordFloat64 {
			conv = "retF64"
		}
		body = append(body, jen.List(jen.Id("_"), jen.Id(frName)).Op(":=").Add(call))
		res := jen.Id(conv).Call(jen.Id(frName))
		if s := fmt.Sprintf("%#v", rawRet); s != "float32" && s != "float64" {
			res = rawRet.Clone().Call(res)
		}
		body = append(body, jen.Return(res))
	default:
		if float {
			body = append(body, jen.List(jen.Id(rName), jen.Id("_")).Op(":=").Add(call))
		}
		switch retKind {
		case wordBool:
			// C defines only the low byte of a bool result.
			body = append(body, jen.Return(jen.Uint8().Call(intResult).Op("!=").Lit(0)))
		case wordPtr:
			p := jen.Id("cptr").Call(intResult)
			if fmt.Sprintf("%#v", rawRet) == "unsafe.Pointer" {
				body = append(body, jen.Return(p))
			} else {
				body = append(body, jen.Return(jen.Parens(rawRet.Clone()).Call(p)))
			}
		default:
			body = append(body, jen.Return(rawRet.Clone().Call(intResult)))
		}
	}
	fb.funcs = append(fb.funcs, jen.Var().Id(addrVar).Uintptr())
	if float {
		regFn := jen.Func().Params(rawParams...)
		if rawRet != nil {
			regFn = regFn.Add(rawRet.Clone())
		}
		fb.funcs = append(fb.funcs, jen.Var().Id(regVar).Add(regFn))
	}
	fn := jen.Func().Id(fnVar).Params(rawParams...)
	if rawRet != nil {
		fn = fn.Add(rawRet)
	}
	fb.funcs = append(fb.funcs, fn.Block(body...))
	return regVar
}

// freshName returns name, with underscores appended until it differs from
// every parameter name in plans.
func freshName(name string, plans []argPlan) string {
	for {
		taken := false
		for _, p := range plans {
			if p.name == name {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
		name += "_"
	}
}
