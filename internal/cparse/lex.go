package cparse

import (
	"strings"
)

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

var puncts3 = []string{"...", "<<=", ">>="}
var puncts2 = []string{"<<", ">>", "&&", "||", "==", "!=", "<=", ">=", "->", "##", "++", "--"}

// lexLine tokenizes a comment-free fragment of C.
func lexFragment(s, file string, line int) []Token {
	var out []Token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case isIdentStart(c):
			j := i + 1
			for j < len(s) && (isIdentStart(s[j]) || isDigit(s[j])) {
				j++
			}
			out = append(out, Token{Ident, s[i:j], file, line})
			i = j
		case isDigit(c) || c == '.' && i+1 < len(s) && isDigit(s[i+1]):
			j := i + 1
			for j < len(s) {
				d := s[j]
				if isIdentStart(d) || isDigit(d) || d == '.' {
					j++
				} else if (d == '+' || d == '-') && (s[j-1] == 'e' || s[j-1] == 'E') && !strings.HasPrefix(strings.ToLower(s[i:]), "0x") {
					j++
				} else {
					break
				}
			}
			out = append(out, Token{Number, s[i:j], file, line})
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			k := String
			if c == '\'' {
				k = Char
			}
			if j >= len(s) {
				j = len(s) - 1
			}
			out = append(out, Token{k, s[i : j+1], file, line})
			i = j + 1
		default:
			n := 1
			for _, p := range puncts3 {
				if strings.HasPrefix(s[i:], p) {
					n = 3
				}
			}
			if n == 1 {
				for _, p := range puncts2 {
					if strings.HasPrefix(s[i:], p) {
						n = 2
					}
				}
			}
			out = append(out, Token{Punct, s[i : i+n], file, line})
			i += n
		}
	}
	return out
}

// cleanDoc strips comment markers from the body of a doxygen comment.
func cleanDoc(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		t := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(t, "*") {
			t = strings.TrimPrefix(t, "*")
			t = strings.TrimPrefix(t, " ")
			lines[i] = t
		} else {
			lines[i] = strings.TrimLeft(l, " ")
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}
