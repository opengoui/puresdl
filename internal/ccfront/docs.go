package ccfront

import (
	"os"
	"strings"
)

// sources caches header contents for doc comment recovery.
type sources map[string][]byte

func (s sources) get(path string) []byte {
	if b, ok := s[path]; ok {
		return b
	}
	b, _ := os.ReadFile(path)
	s[path] = b
	return b
}

// cleanComment strips the comment markers from a doxygen block and the
// leading asterisks of each line.
func cleanComment(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		t := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(t, "*") {
			t = strings.TrimPrefix(strings.TrimPrefix(t, "*"), " ")
			lines[i] = t
		} else {
			lines[i] = strings.TrimLeft(l, " ")
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}

// docBefore returns the doxygen block (/** ... */) that ends right before
// offset off, separated from it only by whitespace.
func docBefore(src []byte, off int) string {
	if off > len(src) {
		return ""
	}
	i := off
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t' || src[i-1] == '\n' || src[i-1] == '\r') {
		i--
	}
	if i < 2 || src[i-1] != '/' || src[i-2] != '*' {
		return ""
	}
	start := strings.LastIndex(string(src[:i-2]), "/*")
	if start < 0 {
		return ""
	}
	body := string(src[start+2 : i-2])
	if !strings.HasPrefix(body, "*") || strings.HasPrefix(body, "*<") || len(body) < 2 {
		return ""
	}
	return cleanComment(body[1:])
}

// memberStart finds the start of the declaration that contains the name
// token at offset off, scanning back over comments to the previous ';', '{',
// '}' or ',' (the latter for enumerators).
func memberStart(src []byte, off int) int {
	i := off
	for i > 0 {
		c := src[i-1]
		switch {
		case c == '/' && i >= 2 && src[i-2] == '*':
			// skip a whole comment
			start := strings.LastIndex(string(src[:i-2]), "/*")
			if start < 0 {
				return i
			}
			i = start
		case c == ';' || c == '{' || c == '}' || c == ',':
			return i
		default:
			i--
		}
	}
	return 0
}

// memberDoc returns the leading doc block of the member whose name token is
// at off, and its trailing /**< ... */ comment.
func memberDoc(src []byte, off int) (leading, trailing string) {
	if off > len(src) {
		return "", ""
	}
	start := memberStart(src, off)
	// leading: the last /** block in [start, off) that is not a trailing one
	region := string(src[start:off])
	for {
		k := strings.LastIndex(region, "/**")
		if k < 0 {
			break
		}
		end := strings.Index(region[k:], "*/")
		if end < 0 {
			break
		}
		body := region[k+2 : k+end]
		if !strings.HasPrefix(body, "*<") {
			leading = cleanComment(body[1:])
			break
		}
		region = region[:k]
	}
	// trailing: first /**< before the terminating ',', ';' or '}' at depth 0,
	// or directly after it.
	depth := 0
	for i := off; i < len(src); i++ {
		c := src[i]
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			end := strings.Index(string(src[i+2:]), "*/")
			if end < 0 {
				return
			}
			body := string(src[i+2 : i+2+end])
			if strings.HasPrefix(body, "*<") {
				return leading, cleanComment(body[2:])
			}
			i += end + 3
			continue
		}
		switch c {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '}':
			if depth <= 0 {
				return
			}
		case ',', ';':
			if depth == 0 {
				j := i + 1
				for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
					j++
				}
				if j+3 < len(src) && string(src[j:j+4]) == "/**<" {
					if end := strings.Index(string(src[j+2:]), "*/"); end >= 0 {
						return leading, cleanComment(string(src[j+4 : j+2+end]))
					}
				}
				return
			}
		}
	}
	return
}

// lineTrailingDoc returns a /**< ... */ comment on the line containing off.
func lineTrailingDoc(src []byte, off int) string {
	end := off
	for end < len(src) && src[end] != '\n' {
		end++
	}
	line := string(src[off:end])
	i := strings.Index(line, "/**<")
	if i < 0 {
		return ""
	}
	rest := string(src[off+i+4:])
	j := strings.Index(rest, "*/")
	if j < 0 {
		return ""
	}
	return cleanComment(rest[:j])
}
