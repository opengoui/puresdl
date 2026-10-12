package cparse

// CType is a parsed C type: a base type plus pointer depth.
type CType struct {
	// Tag is "struct", "union" or "enum" when the base type was introduced
	// with that keyword, otherwise "".
	Tag string
	// Name is the base type: a builtin ("unsigned int"), a typedef name
	// ("Uint32", "SDL_Window") or a tag name. Empty for an inline anonymous
	// aggregate.
	Name  string
	Const bool
	Ptr   int
	// Array holds the length expression tokens of a fixed-size array
	// declarator, and IsArray reports whether one was present.
	Array   []Token
	IsArray bool
	// Agg is set when the type is an inline struct/union definition.
	Agg *Aggregate
}

// Param is a function or callback parameter.
type Param struct {
	Name string
	Type CType
}

// FuncSig is a function signature.
type FuncSig struct {
	Ret      CType
	Params   []Param
	Variadic bool
}

// Func is an exported function declaration.
type Func struct {
	Name string
	FuncSig
	Doc  string
	File string
	Line int
}

// Field is a struct or union member.
type Field struct {
	Name string
	Type CType
	// Func is set for function pointer members.
	Func *FuncSig
	Doc  string
}

// Aggregate is a struct or union definition.
type Aggregate struct {
	Kind   string // "struct" or "union"
	Tag    string
	Fields []Field
	Doc    string
	File   string
	Line   int
}

// EnumMember is an enumerator with its (unevaluated) value expression.
type EnumMember struct {
	Name string
	Expr []Token // nil means previous+1
	Doc  string
}

// Enum is an enum definition.
type Enum struct {
	Name    string
	Members []EnumMember
	Doc     string
	File    string
	Line    int
}

// Typedef is a typedef declaration.
type Typedef struct {
	Name string
	Type CType
	// Func is set for function pointer typedefs (callbacks).
	Func *FuncSig
	// Enum is set when the typedef defines an enum inline.
	Enum *Enum
	Doc  string
	File string
	Line int
}

// Unit is the result of parsing a set of headers.
type Unit struct {
	Files    []string
	Funcs    []*Func
	Typedefs []*Typedef
	Enums    []*Enum
	Aggs     []*Aggregate // struct/union definitions that are not typedefs
	Inline   []string     // names of inline function bodies skipped
	Skipped  []string     // diagnostics for statements that were not understood
}
