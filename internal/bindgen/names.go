package bindgen

import (
	"strings"
	"unicode"
)

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true, "default": true,
	"defer": true, "else": true, "fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true, "map": true, "package": true,
	"range": true, "return": true, "select": true, "struct": true, "switch": true, "type": true,
	"var": true,
}

// shadowed identifiers must not be used as parameter names because the
// generated signatures refer to them.
var shadowed = map[string]bool{
	"string": true, "bool": true, "byte": true, "int": true, "uint": true, "uintptr": true, "error": true,
	"unsafe": true, "fmt": true, "purego": true, "nil": true, "len": true, "cap": true, "new": true, "make": true,
}

// goName maps a C identifier to an exported Go identifier by dropping the
// SDL_ prefix and capitalising.
func goName(c string) string {
	n := strings.TrimPrefix(c, "SDL_")
	if n == "" {
		n = c
	}
	if r := []rune(n)[0]; unicode.IsLower(r) && strings.Contains(n, "_") {
		parts := strings.Split(n, "_")
		for i, p := range parts {
			if v, ok := initialisms[p]; ok {
				parts[i] = v
			} else {
				parts[i] = upperFirst(p)
			}
		}
		var b strings.Builder
		for i, p := range parts {
			if i > 0 && len(p) > 0 && unicode.IsDigit(rune(p[0])) {
				b.WriteByte('_') // keep Murmur3_32 readable
			}
			b.WriteString(p)
		}
		return b.String()
	}
	return upperFirst(n)
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// fieldName converts a C member name to an exported Go field name:
// snake_case becomes CamelCase and lowerCamel gets a leading capital.
func fieldName(c string) string {
	parts := strings.Split(c, "_")
	for i, p := range parts {
		parts[i] = upperFirst(p)
	}
	n := strings.Join(parts, "")
	if n == "" {
		return "X"
	}
	if r := []rune(n)[0]; unicode.IsDigit(r) {
		n = "F" + n
	}
	return n
}

// paramName makes a C parameter name safe to use as a Go identifier.
func paramName(c string, i int) string {
	if c == "" {
		return "p" + itoa(i)
	}
	if goKeywords[c] || shadowed[c] {
		return c + "_"
	}
	return c
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// fileName maps SDL_video.h to video.
func fileName(header string) string {
	n := strings.TrimSuffix(strings.TrimPrefix(header, "SDL_"), ".h")
	if n == "" || n == "SDL" {
		return "sdl"
	}
	return n
}
