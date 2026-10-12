// Package cparse is a small C header front end tailored to the SDL3 public
// headers. It runs a conditional-aware preprocessor, extracts declarations
// (functions, typedefs, enums, structs, unions, callbacks) together with
// their doxygen comments, and evaluates constant #define expressions.
//
// It is not a general C parser: it understands exactly the declaration
// shapes that appear in SDL3 and reports anything it skips.
package cparse

import "fmt"

// Kind classifies a lexical token.
type Kind int

const (
	Ident Kind = iota
	Number
	String
	Char
	Punct
	// Doc is a leading doxygen comment (/** ... */), Post a trailing one
	// (/**< ... */). Text holds the comment body with the comment markers
	// and per-line leading asterisks removed.
	Doc
	Post
)

// Token is a lexical token.
type Token struct {
	Kind Kind
	Text string
	File string
	Line int
}

func (t Token) String() string { return fmt.Sprintf("%q@%s:%d", t.Text, t.File, t.Line) }

func (t Token) is(s string) bool { return (t.Kind == Punct || t.Kind == Ident) && t.Text == s }

func joinTokens(ts []Token) string {
	var b []byte
	for i, t := range ts {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, t.Text...)
	}
	return string(b)
}
