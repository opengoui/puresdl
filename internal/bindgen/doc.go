package bindgen

import (
	"regexp"
	"strings"
)

var sdlIdent = regexp.MustCompile(`\bSDL[A-Za-z0-9_]*\b`)

// docText converts a doxygen block into Go doc comment lines. SDL names that
// the generator knows are rewritten to their Go names.
func (g *gen) docText(doc string) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	var body, params, sa []string
	var returns, since, threads, notes []string
	section := &body
	inCode := false
	flush := func(lines *[]string, s string) { *lines = append(*lines, s) }
	for _, raw := range strings.Split(doc, "\n") {
		line := strings.TrimRight(raw, " \t")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			if inCode {
				flush(section, "")
			}
			continue
		}
		if inCode {
			if strings.HasPrefix(t, "\\endcode") {
				inCode = false
				continue
			}
			flush(section, "\t"+line)
			continue
		}
		switch {
		case strings.HasPrefix(t, "\\code"):
			inCode = true
			flush(section, "")
			continue
		case strings.HasPrefix(t, "\\param"):
			f := strings.Fields(strings.TrimPrefix(t, "\\param"))
			if len(f) > 0 {
				name := f[0]
				if i := strings.Index(name, "["); i >= 0 {
					name = name[:i]
				}
				params = append(params, name+": "+strings.Join(f[1:], " "))
				section = &params
				_ = name
			}
			continue
		case strings.HasPrefix(t, "\\returns"), strings.HasPrefix(t, "\\return"):
			returns = []string{strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, "\\returns"), "\\return"))}
			section = &returns
			continue
		case strings.HasPrefix(t, "\\since"):
			since = []string{strings.TrimSpace(strings.TrimPrefix(t, "\\since"))}
			section = &since
			continue
		case strings.HasPrefix(t, "\\threadsafety"):
			threads = []string{strings.TrimSpace(strings.TrimPrefix(t, "\\threadsafety"))}
			section = &threads
			continue
		case strings.HasPrefix(t, "\\sa"):
			sa = append(sa, strings.Fields(strings.TrimSpace(strings.TrimPrefix(t, "\\sa")))...)
			section = &sa
			continue
		case strings.HasPrefix(t, "\\note"), strings.HasPrefix(t, "\\remarks"), strings.HasPrefix(t, "\\warning"):
			notes = append(notes, strings.TrimSpace(t[strings.Index(t, " ")+1:]))
			section = &notes
			continue
		case strings.HasPrefix(t, "\\details"), strings.HasPrefix(t, "\\brief"):
			t = strings.TrimSpace(t[strings.Index(t+" ", " "):])
			line = t
		case strings.HasPrefix(t, "\\"):
			// unknown command (\cond, \name, \ingroup...): drop the line
			continue
		}
		if t == "" {
			if section == &body {
				flush(&body, "")
			} else {
				section = &body
				flush(&body, "")
			}
			continue
		}
		if section == &params || section == &returns || section == &since || section == &threads || section == &notes {
			(*section)[len(*section)-1] += " " + t
			continue
		}
		if section == &sa {
			sa = append(sa, strings.Fields(t)...)
			continue
		}
		flush(&body, line)
	}
	var out []string
	out = append(out, trimBlank(body)...)
	if len(params) > 0 {
		out = append(out, "", "Parameters:")
		for _, p := range params {
			out = append(out, "  - "+p)
		}
	}
	if len(returns) > 0 && returns[0] != "" {
		out = append(out, "", "Returns: "+returns[0])
	}
	for _, n := range notes {
		out = append(out, "", "Note: "+n)
	}
	if len(threads) > 0 && threads[0] != "" {
		out = append(out, "", "Thread safety: "+threads[0])
	}
	if len(since) > 0 && since[0] != "" {
		out = append(out, "", "Since: "+sinceText(since[0]))
	}
	if len(sa) > 0 {
		var names []string
		for _, s := range sa {
			names = append(names, strings.Trim(s, ",.`"))
		}
		out = append(out, "", "See also: "+strings.Join(names, ", "))
	}
	for i, l := range out {
		out[i] = g.rewriteNames(l)
	}
	return out
}

func sinceText(s string) string {
	if i := strings.Index(s, "available since "); i >= 0 {
		return strings.TrimSuffix(s[i+len("available since "):], ".") + "."
	}
	return s
}

func trimBlank(l []string) []string {
	for len(l) > 0 && l[0] == "" {
		l = l[1:]
	}
	for len(l) > 0 && l[len(l)-1] == "" {
		l = l[:len(l)-1]
	}
	return l
}

// rewriteNames replaces known SDL identifiers with their Go names.
func (g *gen) rewriteNames(line string) string {
	return sdlIdent.ReplaceAllStringFunc(line, func(s string) string {
		if n, ok := g.docNames[s]; ok {
			return n
		}
		return s
	})
}
