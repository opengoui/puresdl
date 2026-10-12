// Package ir is the target-neutral description of a C API that the front end
// (internal/ccfront) produces and the Go emitter (internal/bindgen)
// consumes. It carries ABI facts (sizes, alignments, offsets, enumerator
// values) computed by the C front end, so the emitter never has to guess a
// layout.
package ir

import "math/big"

// Kind classifies a Type.
type Kind int

const (
	Invalid Kind = iota
	Void
	Bool
	Int     // integer of Size bytes; Signed tells the sign (char included)
	Float   // Size is 4 or 8 (long double is Invalid)
	Pointer // Elem is the pointee
	Array   // Elem and Len
	Func    // Sig
	Struct  // Rec
	Union   // Rec
	Enum    // Enum
	Named   // typedef reference: Name, Under
)

// Type is a C type. Aggregate and typedef nodes are shared, so a *Type can
// be compared by pointer to detect the same declaration.
type Type struct {
	Kind   Kind
	Size   int64
	Align  int
	Signed bool
	// Const is set on a Pointer whose pointee is const (const char *).
	Const bool

	Elem  *Type
	Len   int64
	Sig   *Sig
	Rec   *Record
	Enum  *EnumDecl
	Name  string // Named: the typedef name
	Under *Type  // Named: the aliased type
	// Char is set for the C char family of Int types.
	Char bool
}

// Resolve strips typedef layers.
func (t *Type) Resolve() *Type {
	for t != nil && t.Kind == Named {
		t = t.Under
	}
	return t
}

// Param is a function or callback parameter.
type Param struct {
	Name string
	Type *Type
	Doc  string // text of the matching \param tag, if any
}

// Sig is a function signature.
type Sig struct {
	Ret      *Type
	Params   []Param
	Variadic bool
}

// Field is a struct or union member.
type Field struct {
	Name   string
	Type   *Type
	Offset int64
	Doc    string
}

// Record is a struct or union definition.
type Record struct {
	Union      bool
	Tag        string // C tag, "" if anonymous
	Name       string // typedef name that first names it, "" if none
	Fields     []Field
	Size       int64
	Align      int
	Incomplete bool // declared but not defined (opaque handle)
	Doc        string
	File       string
}

// Enumerator is one enum constant.
type Enumerator struct {
	Name  string
	Value *big.Int
	Doc   string
}

// EnumDecl is an enum definition.
type EnumDecl struct {
	Tag     string
	Name    string // typedef name, "" if none
	Members []Enumerator
	// Signed is false when no enumerator is negative and the underlying
	// type is unsigned.
	Signed bool
	Size   int64
	Doc    string
	File   string
}

// Typedef is a named alias declared in the headers.
type Typedef struct {
	Name string
	Type *Type // the Named node for this typedef
	Doc  string
	File string
	// Std is set for typedefs that come from the libc stubs (uint8_t...)
	// instead of the SDL headers.
	Std bool
	// Offset is the byte offset of the typedef name in File (0 for Std).
	Offset int
}

// Function is an exported function.
type Function struct {
	Name string
	Sig  *Sig
	Doc  string
	File string
	// Targets lists the "goos/goarch" configurations that declare it.
	Targets []string
}

// ConstKind selects the representation of a Const.
type ConstKind int

const (
	ConstInt ConstKind = iota
	ConstFloat
	ConstString
)

// Const is an object-like macro with a constant value.
type Const struct {
	Name  string
	Kind  ConstKind
	Int   *big.Int
	Float float64
	Str   string
	Doc   string
	File  string
	// Offset is the byte offset of the #define in File.
	Offset int
	// After names the typedef whose declaration is immediately followed
	// by this macro's contiguous #define block: nothing but directives,
	// comments and blank lines separates them in the same header. "" if
	// there is none.
	After string
}

// API is everything extracted from the headers, merged across targets.
type API struct {
	Funcs    []*Function
	Typedefs []*Typedef
	Enums    []*EnumDecl // every enum, typedef'd or not
	Records  []*Record   // every struct/union with a definition
	Consts   []*Const
	// Version is the SDL version the headers declare.
	Version string
}
