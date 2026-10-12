package bindgen

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

func intLit(v *big.Int) jen.Code {
	if v.Sign() >= 0 && v.Cmp(big.NewInt(0x100)) >= 0 {
		return jen.Id("0x" + strings.ToUpper(v.Text(16)))
	}
	return jen.Id(v.String())
}

func (g *gen) emitEnums() {
	for _, e := range g.api.Enums {
		f := e.File
		if f == "" {
			if td, ok := g.typedefs[e.Name]; ok {
				f = td.File
			}
		}
		if f == "" || len(e.Members) == 0 {
			continue
		}
		fb := g.file(base(f))
		typeName, typed := g.enumName[e]
		if typed {
			fb.types = append(fb.types, start(g.docText(e.Doc)).Type().Id(typeName).Id(intName(e.Size, e.Signed)))
		}
		var items []jen.Code
		for _, m := range e.Members {
			n := goName(m.Name)
			if !g.claim(n, m.Name) {
				continue
			}
			s := start(g.docText(m.Doc)).Id(n)
			if typed {
				s.Id(typeName)
			}
			items = append(items, s.Op("=").Add(intLit(m.Value)))
		}
		if len(items) > 0 {
			fb.consts = append(fb.consts, jen.Const().Defs(items...))
		}
	}
}

var skipConst = []string{"SDL_PRI", "SDL_SCN"}

func (g *gen) emitConsts() {
	byFile := map[string][]jen.Code{}
	var order []string
	for _, k := range g.api.Consts {
		skip := false
		for _, p := range skipConst {
			if hasPrefix(k.Name, p) {
				skip = true
			}
		}
		if skip {
			continue
		}
		n := goName(k.Name)
		if prev, ok := g.claimed[n]; ok {
			if prev != k.Name {
				g.skip(k.Name, "Go name "+n+" already used by "+prev)
			}
			continue
		}
		g.claim(n, k.Name)
		var v jen.Code
		switch k.Kind {
		case ir.ConstInt:
			v = intLit(k.Int)
		case ir.ConstFloat:
			v = jen.Lit(k.Float)
		default:
			v = jen.Lit(k.Str)
		}
		if _, ok := byFile[k.File]; !ok {
			order = append(order, k.File)
		}
		byFile[k.File] = append(byFile[k.File], start(g.docText(k.Doc)).Id(n).Op("=").Add(v))
	}
	for _, f := range order {
		g.file(f).consts = append(g.file(f).consts, jen.Const().Defs(byFile[f]...))
	}
}

type argPlan struct {
	name string
	pub  *jen.Statement
	raw  *jen.Statement
	expr jen.Code
}

func (g *gen) emitFuncs() {
	for _, f := range g.api.Funcs {
		if why := g.bindFunc(f); why != "" {
			g.skip(f.Name, why)
		}
	}
}

func isFmtParam(p ir.Param) bool {
	return p.Name == "fmt" && isCString(p.Type) && p.Type.Resolve().Const
}

// bindFunc emits the wrapper for f and returns a reason when it is skipped.
func (g *gen) bindFunc(f *ir.Function) string {
	if f.File == "SDL_stdinc.h" && !keptStdinc.MatchString(f.Name) {
		return "libc replacement"
	}
	sig := f.Sig
	printf := false
	if sig.Variadic {
		if n := len(sig.Params); n == 0 || !isFmtParam(sig.Params[n-1]) {
			return "C variadic function"
		}
		printf = true
	}
	var plans []argPlan
	var notes []string
	for i, p := range sig.Params {
		name := paramName(p.Name, i)
		if printf && i == len(sig.Params)-1 {
			plans = append(plans, argPlan{name: name, raw: jen.Op("*").Id("byte"),
				expr: jen.Id("cprintf").Call(jen.Id("format"), jen.Id("args"))})
			continue
		}
		if isCString(p.Type) && p.Type.Resolve().Const {
			fn := "cstr"
			if nullableRe.MatchString(p.Doc) {
				fn = "cstrNull"
				notes = append(notes, "An empty "+name+" is passed as NULL.")
			}
			plans = append(plans, argPlan{name: name, pub: jen.String(), raw: jen.Op("*").Id("byte"),
				expr: jen.Id(fn).Call(jen.Id(name))})
			continue
		}
		t, err := g.typ(p.Type)
		if err != nil {
			return "parameter " + name + ": " + err.Error()
		}
		plans = append(plans, argPlan{name: name, pub: t, raw: t, expr: jen.Id(name)})
	}
	var rawRet, pubRet *jen.Statement
	retString, retFree := false, false
	if rr := sig.Ret.Resolve(); rr.Kind != ir.Void {
		if isCString(sig.Ret) {
			rawRet, pubRet, retString = jen.Op("*").Id("byte"), jen.String(), true
			retFree = !rr.Const && freedStringRe.MatchString(f.Doc)
		} else {
			t, err := g.typ(sig.Ret)
			if err != nil {
				return "result: " + err.Error()
			}
			rawRet, pubRet = t, t
		}
	}
	n := goName(f.Name)
	if !g.claim(n, f.Name) {
		return ""
	}
	fnVar := "fn" + n
	var rawParams, pubParams, args []jen.Code
	for i, p := range plans {
		rawParams = append(rawParams, jen.Id(p.name).Add(p.raw))
		args = append(args, p.expr)
		if printf && i == len(plans)-1 {
			pubParams = append(pubParams, jen.Id("format").String(), jen.Id("args").Op("...").Any())
			continue
		}
		pubParams = append(pubParams, jen.Id(p.name).Add(p.pub))
	}
	rawFn := jen.Func().Params(rawParams...)
	if rawRet != nil {
		rawFn = rawFn.Add(rawRet)
	}
	fb := g.file(f.File)
	fb.funcs = append(fb.funcs, jen.Var().Id(fnVar).Add(rawFn))
	g.binds = append(g.binds, [2]string{fnVar, f.Name})

	doc := g.docText(f.Doc)
	if printf {
		notes = append(notes, "The message is formatted with fmt.Sprintf before it reaches SDL.")
	}
	if len(f.Targets) < 3 {
		notes = append(notes, "Only declared by the SDL headers for "+strings.Join(f.Targets, ", ")+".")
	}
	if len(notes) > 0 {
		doc = append(doc, "")
		doc = append(doc, notes...)
	}
	call := jen.Id(fnVar).Call(args...)
	var body []jen.Code
	switch {
	case rawRet == nil:
		body = []jen.Code{call}
	case retFree:
		body = []jen.Code{
			jen.Id("p").Op(":=").Add(call),
			jen.If(jen.Id("p").Op("==").Nil()).Block(jen.Return(jen.Lit(""))),
			jen.Id("s").Op(":=").Id("GoString").Call(jen.Id("p")),
			jen.Id("Free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("p"))),
			jen.Return(jen.Id("s")),
		}
	case retString:
		body = []jen.Code{jen.Return(jen.Id("GoString").Call(call))}
	default:
		body = []jen.Code{jen.Return(call)}
	}
	w := start(doc).Func().Id(n).Params(pubParams...)
	if pubRet != nil {
		w = w.Add(pubRet)
	}
	fb.funcs = append(fb.funcs, w.Block(body...))
	_ = fmt.Sprint
	return ""
}
