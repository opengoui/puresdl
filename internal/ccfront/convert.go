package ccfront

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"

	"modernc.org/cc/v4"

	"github.com/opengoui/puresdl/internal/ir"
)

// converter turns cc types into ir types, sharing nodes across targets.
type converter struct {
	api      *ir.API
	sdlDir   string // directory holding the SDL3 headers
	src      sources
	named    map[string]*ir.Typedef
	recs     map[any]*ir.Type // keyed by cc.Type identity, or by "<isUnion> <tag>" when tagged
	enums    map[any]*ir.Type
	docs     map[*cc.Declarator]string
	warnings []string
}

// newConverter returns a converter that treats files under sdlDir as SDL
// headers (as opposed to libc stubs) when recovering docs and positions.
func newConverter(sdlDir string) *converter {
	return &converter{
		api: &ir.API{}, sdlDir: sdlDir, src: sources{},
		named: map[string]*ir.Typedef{}, recs: map[any]*ir.Type{}, enums: map[any]*ir.Type{},
		docs: map[*cc.Declarator]string{},
	}
}

// inSDL reports whether file lives under the SDL3 header directory.
func (c *converter) inSDL(file string) bool {
	rel, err := filepath.Rel(c.sdlDir, file)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// warnf records a non-fatal conversion problem, returned by Load.
func (c *converter) warnf(format string, a ...any) {
	c.warnings = append(c.warnings, fmt.Sprintf(format, a...))
}

// isConst reports whether t carries a const qualifier.
func isConst(t cc.Type) bool {
	a := t.Attributes()
	return a != nil && a.IsConst()
}

// basic returns a scalar type of the given size with natural alignment.
func basic(kind ir.Kind, size int64, signed, char bool) *ir.Type {
	return &ir.Type{Kind: kind, Size: size, Align: int(size), Signed: signed, Char: char}
}

// typ converts a type reference, preserving typedef names.
func (c *converter) typ(t cc.Type) *ir.Type {
	if td := t.Typedef(); td != nil {
		return c.typedefType(td)
	}
	return c.shape(t)
}

// typedefType converts the typedef declarator td once and returns its Named
// type. It also names anonymous structs, unions and enums after the typedef
// and lends them the typedef's doc.
func (c *converter) typedefType(td *cc.Declarator) *ir.Type {
	name := td.Name()
	if d, ok := c.named[name]; ok {
		return d.Type
	}
	d := &ir.Typedef{Name: name, File: td.Position().Filename, Std: !c.inSDL(td.Position().Filename), Offset: td.Position().Offset}
	d.Type = &ir.Type{Kind: ir.Named, Name: name}
	c.named[name] = d
	c.api.Typedefs = append(c.api.Typedefs, d)
	d.Doc = c.docs[td]
	under := td.Type()
	if u := under.Typedef(); u != nil && u != td {
		d.Type.Under = c.typedefType(u)
	} else {
		d.Type.Under = c.shape(under)
	}
	d.Type.Size, d.Type.Align = d.Type.Under.Size, d.Type.Under.Align
	if r := d.Type.Under.Resolve(); r != nil {
		switch r.Kind {
		case ir.Struct, ir.Union:
			if r.Rec.Name == "" {
				r.Rec.Name = name
				if r.Rec.Doc == "" {
					r.Rec.Doc = d.Doc
				}
			}
		case ir.Enum:
			if r.Enum.Name == "" {
				r.Enum.Name = name
				if r.Enum.Doc == "" {
					r.Enum.Doc = d.Doc
				}
			}
		}
	}
	return d.Type
}

// shape converts the structure of t, ignoring any typedef name on t itself.
// Unsupported kinds (such as long double) become ir.Invalid.
func (c *converter) shape(t cc.Type) *ir.Type {
	switch x := t.(type) {
	case *cc.PointerType:
		e := x.Elem()
		return &ir.Type{Kind: ir.Pointer, Size: 8, Align: 8, Elem: c.typ(e), Const: isConst(e)}
	case *cc.ArrayType:
		return &ir.Type{Kind: ir.Array, Size: x.Size(), Align: x.Align(), Elem: c.typ(x.Elem()), Len: x.Len()}
	case *cc.FunctionType:
		return &ir.Type{Kind: ir.Func, Sig: c.sig(x)}
	case *cc.StructType:
		return c.record(t, false, tagName(x.Tag()), x.NumFields(), x.FieldByIndex, x.Size(), x.Align(), x.IsIncomplete())
	case *cc.UnionType:
		return c.record(t, true, tagName(x.Tag()), x.NumFields(), x.FieldByIndex, x.Size(), x.Align(), x.IsIncomplete())
	case *cc.EnumType:
		return c.enum(x)
	}
	switch t.Kind() {
	case cc.Void:
		return &ir.Type{Kind: ir.Void, Align: 1}
	case cc.Bool:
		return basic(ir.Bool, 1, false, false)
	case cc.Char:
		return basic(ir.Int, 1, true, true)
	case cc.SChar:
		return basic(ir.Int, 1, true, false)
	case cc.UChar:
		return basic(ir.Int, 1, false, false)
	case cc.Short, cc.Int, cc.Long, cc.LongLong:
		return basic(ir.Int, t.Size(), true, false)
	case cc.UShort, cc.UInt, cc.ULong, cc.ULongLong:
		return basic(ir.Int, t.Size(), false, false)
	case cc.Float, cc.Double:
		return basic(ir.Float, t.Size(), true, false)
	}
	return &ir.Type{Kind: ir.Invalid, Size: t.Size(), Align: t.Align(), Name: t.String()}
}

// sig converts a function type, dropping the unnamed (void) parameter of
// prototypes like f(void).
func (c *converter) sig(ft *cc.FunctionType) *ir.Sig {
	s := &ir.Sig{Ret: c.typ(ft.Result()), Variadic: ft.IsVariadic()}
	for _, p := range ft.Parameters() {
		if p.Type().Kind() == cc.Void && p.Name() == "" {
			continue
		}
		s.Params = append(s.Params, ir.Param{Name: p.Name(), Type: c.typ(p.Type())})
	}
	return s
}

// record converts a struct or union, sharing one ir.Record per tag across
// targets. A forward-declared record is completed when a later definition is
// seen, and a size mismatch between targets is reported as a warning.
func (c *converter) record(t cc.Type, union bool, tag string, n int, field func(int) *cc.Field, size int64, align int, incomplete bool) *ir.Type {
	var key any = t
	if tag != "" {
		key = fmt.Sprintf("%v %s", union, tag)
	}
	if rt, ok := c.recs[key]; ok {
		if !incomplete && !rt.Rec.Incomplete && rt.Rec.Size != size {
			c.warnf("layout of %s differs across targets: %d vs %d", tag, rt.Rec.Size, size)
		}
		if !incomplete && rt.Rec.Incomplete {
			c.fillRecord(rt, n, field, size, align)
		}
		return rt
	}
	kind := ir.Struct
	if union {
		kind = ir.Union
	}
	rec := &ir.Record{Union: union, Tag: tag, Incomplete: incomplete, Size: size, Align: align}
	rt := &ir.Type{Kind: kind, Size: size, Align: align, Rec: rec}
	c.recs[key] = rt
	c.api.Records = append(c.api.Records, rec)
	if !incomplete {
		c.fillRecord(rt, n, field, size, align)
	}
	return rt
}

// fillRecord populates rt with layout and fields, attaching member docs from
// the header text. Bitfields are warned about since they are not supported.
func (c *converter) fillRecord(rt *ir.Type, n int, field func(int) *cc.Field, size int64, align int) {
	rec := rt.Rec
	rec.Incomplete, rec.Size, rec.Align = false, size, align
	rt.Size, rt.Align = size, align
	for i := 0; i < n; i++ {
		f := field(i)
		if f.IsBitfield() {
			c.warnf("bitfield %s.%s is not supported", rec.Tag, f.Name())
		}
		fd := ir.Field{Name: f.Name(), Type: c.typ(f.Type()), Offset: f.Offset()}
		if d := f.Declarator(); d != nil && f.Name() != "" {
			pos := d.NameTok().Position()
			if c.inSDL(pos.Filename) {
				lead, trail := memberDoc(c.src.get(pos.Filename), pos.Offset)
				fd.Doc = lead
				if fd.Doc == "" {
					fd.Doc = trail
				}
				if rec.File == "" {
					rec.File = pos.Filename
				}
			}
		}
		rec.Fields = append(rec.Fields, fd)
	}
}

// enum converts an enum type, sharing one ir.EnumDecl per tag. Enumerators
// with non-integer values are skipped with a warning.
func (c *converter) enum(et *cc.EnumType) *ir.Type {
	var key any = et
	tag := tagName(et.Tag())
	if tag != "" {
		key = "enum " + tag
	}
	if e, ok := c.enums[key]; ok {
		return e
	}
	ed := &ir.EnumDecl{Tag: tag, Signed: et.Min() < 0, Size: et.Size()}
	t := &ir.Type{Kind: ir.Enum, Size: et.Size(), Align: et.Align(), Signed: ed.Signed, Enum: ed}
	c.enums[key] = t
	c.api.Enums = append(c.api.Enums, ed)
	for _, e := range et.Enumerators() {
		m := ir.Enumerator{Name: tagName(e.Token)}
		switch v := e.Value().(type) {
		case cc.Int64Value:
			m.Value = big.NewInt(int64(v))
		case cc.UInt64Value:
			m.Value = new(big.Int).SetUint64(uint64(v))
		default:
			c.warnf("enumerator %s has non-integer value %v", m.Name, v)
			continue
		}
		pos := e.Token.Position()
		if c.inSDL(pos.Filename) {
			lead, trail := memberDoc(c.src.get(pos.Filename), pos.Offset)
			m.Doc = lead
			if m.Doc == "" {
				m.Doc = trail
			}
			if ed.File == "" {
				ed.File = pos.Filename
			}
		}
		ed.Members = append(ed.Members, m)
	}
	return t
}

// bigInt boxes v as a *big.Int.
func bigInt(v int64) *big.Int { return big.NewInt(v) }

// bigUint boxes v as a *big.Int.
func bigUint(v uint64) *big.Int { return new(big.Int).SetUint64(v) }

// tagName returns the source spelling of t, "" for an absent token.
func tagName(t cc.Token) string { return t.SrcStr() }
