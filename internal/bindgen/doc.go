package bindgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// sdlIdent matches SDL identifiers in doc prose.
var sdlIdent = regexp.MustCompile(`\bSDL[A-Za-z0-9_]*\b`)

// docAllow is the small list of SDL_ identifiers that docs may mention although they are
// neither bound nor hand-written: preprocessor switches and build-time
// macros without a Go counterpart. They are rendered as backticked C names.
var docAllow = map[string]bool{
	// Entry points that the application defines (SDL_main callbacks).
	"SDL_AppInit": true, "SDL_AppIterate": true, "SDL_AppEvent": true, "SDL_AppQuit": true,
	// C macros and build switches without a Go counterpart.
	"SDL_AtomicIncRef": true, "SDL_AtomicDecRef": true, "SDL_MUSTLOCK": true,
	"SDL_MemoryBarrierAcquire": true, "SDL_MemoryBarrierRelease": true,
	"SDL_assert": true, "SDL_REVISION": true, "SDL_revision": true,
	"SDL_INIT_INTERFACE": true, "SDL_HAPTIC_GAIN_MAX": true, "SDL_GDK_TEXTINPUT": true,
	"SDL_GDKSuspendGPU": true, "SDL_UDEV_deviceclass": true,
	"SDL_SRCCOLORKEY": true, "SDL_UpdateRects": true, "SDL_Flip": true,
	"SDL_MAIN_HANDLED": true, "SDL_MAIN_USE_CALLBACKS": true,
	// Names that appear in the headers' prose only (typos, hint values,
	// property names without a constant, library or subsystem names).
	"SDL_FILESYSTEM_BASE_DIR_TYPE": true, "SDL_windowX": true, "SDL_SetJoystickVirtualSensorData": true,
	"SDL_PROP_TEXTURE_CREATE_GPU_TEXTURE_UV_NUMBER": true,
	"SDL_PROP_TEXTURE_CREATE_GPU_TEXTURE_U_NUMBER":  true,
	"SDL_PROP_TEXTURE_CREATE_GPU_TEXTURE_V_NUMBER":  true,
	"SDL_app": true, "SDL_ttf": true, "SDL_gamepad": true, "SDL_sensor": true,
	"SDL_GPU": true, "SDL_Render": true, "SDL_Rects": true,
}

var (
	// reEntity matches HTML entities in doxygen prose.
	reEntity = regexp.MustCompile(`&(?:lt|gt|amp|quot|apos|nbsp|#[0-9]+|#x[0-9a-fA-F]+);`)
	// reURLLink matches markdown links to http(s) URLs.
	reURLLink = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	// reWikiLink matches markdown links with a relative or wiki target.
	reWikiLink = regexp.MustCompile(`\[([^\]]+)\]\([^)\s]*\)`)
	// reBold matches markdown bold or bold-italic spans.
	reBold = regexp.MustCompile(`(?s)\*{2,3}([^\s*](?:.*?[^\s*])?)\*{2,3}`)
	// reCodeTag matches doxygen \p, \c and \a references to a code identifier.
	reCodeTag = regexp.MustCompile("[\\\\@](?:p|c|a)\\s+([A-Za-z_][A-Za-z0-9_]*(?:(?:->|\\.|::)[A-Za-z_][A-Za-z0-9_]*)*(?:\\(\\))?)")
	// reTextTag matches doxygen inline style tags that are dropped from the text.
	reTextTag = regexp.MustCompile(`[\\@](?:b|e|em|ref|p|c|a)\s+`)
	// reInlineSA matches a \sa tag in the middle of a line.
	reInlineSA = regexp.MustCompile(`[ \t][\\@]sa\s+`)
	// reCmd matches a line that starts with a doxygen command.
	reCmd = regexp.MustCompile(`^[\\@]([A-Za-z]+)(?:\[[^\]]*\])?\s*(.*)$`)
	// reListMark matches a markdown list marker at the start of a line.
	reListMark = regexp.MustCompile(`^(?:[-*]|[0-9]+[.)])\s+`)
)

// dline is one preprocessed doc line.
type dline struct {
	text string
	pre  bool // preformatted (code block or table row), already tab-indented
}

// docBlock is one ordered piece of the rendered doc.
type docBlock struct {
	kind  string // body, params, returns, note, threads, since, sa, deprecated
	lines []string
}

// docText converts a doxygen block into Go doc comment lines. SDL names that
// the generator knows are rewritten to their Go names.
func (g *gen) docText(doc string) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	var blocks []*docBlock
	var cur *docBlock
	inList := false
	body := func() *docBlock {
		if cur == nil || cur.kind != "body" {
			cur = &docBlock{kind: "body"}
			blocks = append(blocks, cur)
		}
		return cur
	}
	// open starts a tagged block, merging runs of params and see-also tags.
	open := func(kind string) *docBlock {
		if n := len(blocks); n > 0 && blocks[n-1].kind == kind && (kind == "params" || kind == "sa") {
			cur = blocks[n-1]
			return cur
		}
		cur = &docBlock{kind: kind}
		blocks = append(blocks, cur)
		return cur
	}
	for _, l := range preprocessDoc(doc) {
		if l.pre {
			b := body()
			if n := len(b.lines); strings.TrimSpace(l.text) == "" && n > 0 && strings.TrimSpace(b.lines[n-1]) == "" {
				continue // collapse blank lines inside preformatted text
			}
			b.lines = append(b.lines, l.text)
			inList = false
			continue
		}
		t := strings.TrimSpace(l.text)
		if t == "" {
			inList = false
			if cur != nil && cur.kind == "body" {
				cur.lines = append(cur.lines, "")
			} else {
				cur = nil
			}
			continue
		}
		if m := reCmd.FindStringSubmatch(t); m != nil {
			cmd, rest := m[1], strings.TrimSpace(m[2])
			switch cmd {
			case "param":
				f := strings.Fields(rest)
				if len(f) > 0 {
					b := open("params")
					b.lines = append(b.lines, f[0]+": "+strings.Join(f[1:], " "))
				}
			case "returns", "return", "result":
				open("returns").lines = []string{rest}
			case "since":
				open("since").lines = []string{rest}
			case "threadsafety":
				open("threads").lines = []string{rest}
			case "deprecated":
				open("deprecated").lines = []string{rest}
			case "note", "remarks", "warning":
				open("note").lines = []string{rest}
			case "sa", "see":
				b := open("sa")
				b.lines = append(b.lines, strings.Fields(rest)...)
			case "li":
				b := body()
				b.lines = append(b.lines, "- "+rest)
				inList = true
			case "details", "brief":
				if rest != "" {
					b := body()
					b.lines = append(b.lines, rest)
				}
			default:
				// unknown command (\cond, \name, \ingroup...): drop the line
			}
			continue
		}
		switch cur.kindOr("body") {
		case "params":
			cur.lines[len(cur.lines)-1] += " " + t
		case "sa":
			cur.lines = append(cur.lines, strings.Fields(t)...)
		case "body":
			b := body()
			switch {
			case reListMark.MatchString(t):
				inList = true
				t = reListMark.ReplaceAllStringFunc(t, func(m string) string {
					if m[0] >= '0' && m[0] <= '9' {
						return m
					}
					return "- "
				})
				b.lines = append(b.lines, t)
			case inList && l.text != strings.TrimLeft(l.text, " \t"):
				b.lines = append(b.lines, "  "+t)
			default:
				inList = false
				b.lines = append(b.lines, t)
			}
		default:
			cur.lines[len(cur.lines)-1] += " " + t
		}
	}
	var out []string
	emit := func(lines ...string) {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	for _, b := range blocks {
		switch b.kind {
		case "body":
			if l := collapseBlank(b.lines); len(l) > 0 {
				emit(l...)
			}
		case "params":
			l := []string{"Parameters:"}
			for _, p := range b.lines {
				l = append(l, "  - "+p)
			}
			emit(l...)
		case "returns":
			if b.lines[0] != "" {
				emit("Returns: " + b.lines[0])
			}
		case "note":
			emit("Note: " + b.lines[0])
		case "deprecated":
			emit("Deprecated: " + b.lines[0])
		case "threads":
			if b.lines[0] != "" {
				emit("Thread safety: " + b.lines[0])
			}
		case "since":
			if b.lines[0] != "" {
				emit("Since: " + sinceText(b.lines[0]))
			}
		case "sa":
			var names []string
			for _, s := range b.lines {
				if s = strings.Trim(s, ",.`"); s != "" {
					names = append(names, s)
				}
			}
			if len(names) > 0 {
				emit("See also: " + strings.Join(names, ", "))
			}
		}
	}
	for i, l := range out {
		out[i] = g.rewriteNames(l)
	}
	return out
}

// kindOr returns the block kind, or def when b is nil.
func (b *docBlock) kindOr(def string) string {
	if b == nil {
		return def
	}
	return b.kind
}

// preprocessDoc normalises the inline markup of a doxygen block: code
// fences and tables become preformatted lines, and links, bold text, html
// entities and inline doxygen tags are reduced to plain text.
func preprocessDoc(doc string) []dline {
	var out []dline
	var para []string
	var table []string
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		s := inlineDoc(strings.Join(para, "\n"))
		for _, l := range strings.Split(s, "\n") {
			out = append(out, dline{text: l})
		}
		para = nil
	}
	flushTable := func() {
		if len(table) == 0 {
			return
		}
		out = append(out, dline{})
		for _, r := range table {
			out = append(out, dline{text: "\t" + strings.TrimRight(reBold.ReplaceAllString(html.UnescapeString(r), "$1"), " \t"), pre: true})
		}
		out = append(out, dline{})
		table = nil
	}
	inCode := false
	for _, raw := range strings.Split(doc, "\n") {
		line := strings.TrimRight(raw, " \t")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			flushPara()
			flushTable()
			inCode = !inCode
			out = append(out, dline{})
			continue
		}
		if inCode {
			if strings.HasPrefix(t, "\\endcode") || strings.HasPrefix(t, "@endcode") {
				inCode = false
				out = append(out, dline{})
				continue
			}
			if line != "" {
				line = "\t" + line
			}
			out = append(out, dline{text: line, pre: true})
			continue
		}
		if strings.HasPrefix(t, "\\code") || strings.HasPrefix(t, "@code") {
			flushPara()
			flushTable()
			inCode = true
			out = append(out, dline{})
			continue
		}
		switch {
		case t == "":
			flushPara()
			flushTable()
			out = append(out, dline{})
		case t == "@{" || t == "@}" || t == "\\{" || t == "\\}":
		case strings.HasPrefix(t, "|"):
			flushPara()
			table = append(table, t)
		default:
			flushTable()
			para = append(para, line)
		}
	}
	flushPara()
	flushTable()
	return out
}

// inlineDoc rewrites the inline markup of one paragraph.
func inlineDoc(s string) string {
	if strings.Contains(s, "&") {
		s = reEntity.ReplaceAllStringFunc(s, html.UnescapeString)
	}
	if strings.Contains(s, "](") {
		s = reURLLink.ReplaceAllString(s, "$1 ($2)")
		s = reWikiLink.ReplaceAllString(s, "$1")
	}
	if strings.Contains(s, "**") {
		s = reBold.ReplaceAllString(s, "$1")
	}
	// Leading @tags become backslash tags so one parser handles both.
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		t := strings.TrimLeft(l, " \t")
		if len(t) > 1 && t[0] == '@' && t[1] >= 'a' && t[1] <= 'z' {
			lines[i] = "\\" + t[1:]
		}
	}
	s = strings.Join(lines, "\n")
	s = reInlineSA.ReplaceAllString(s, "\n\\sa ")
	s = reCodeTag.ReplaceAllString(s, "`$1`")
	s = reTextTag.ReplaceAllString(s, "")
	return s
}

// collapseBlank trims blank lines at both ends and collapses runs of blank
// lines into one.
func collapseBlank(l []string) []string {
	var out []string
	for _, s := range l {
		if s == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, s)
	}
	return trimBlank(out)
}

// sinceText reduces an "available since" sentence to the text after it.
func sinceText(s string) string {
	if i := strings.Index(s, "available since "); i >= 0 {
		return strings.TrimSuffix(s[i+len("available since "):], ".") + "."
	}
	return s
}

// trimBlank removes leading and trailing empty lines.
func trimBlank(l []string) []string {
	for len(l) > 0 && l[0] == "" {
		l = l[1:]
	}
	for len(l) > 0 && l[len(l)-1] == "" {
		l = l[:len(l)-1]
	}
	return l
}

// rewriteNames replaces SDL identifiers with their Go names. Identifiers
// that are bound or hand-written become Go names; every other SDL_ name is
// rendered as a backticked C name so go doc does not point at a Go symbol
// that does not exist. Those names are recorded in docUnknown for
// checkDocNames.
func (g *gen) rewriteNames(line string) string {
	locs := sdlIdent.FindAllStringIndex(line, -1)
	if locs == nil {
		return line
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		s := line[loc[0]:loc[1]]
		b.WriteString(line[last:loc[0]])
		last = loc[1]
		if n, ok := g.docNames[s]; ok {
			b.WriteString(n)
			continue
		}
		wildcard := loc[1] < len(line) && line[loc[1]] == '*'
		if !strings.HasPrefix(s, "SDL_") && !strings.HasPrefix(s, "SDLK_") || strings.HasSuffix(s, "_") || wildcard {
			b.WriteString(s) // "SDL", "SDL3", or a prefix pattern such as SDL_HINT_
			continue
		}
		if strings.HasPrefix(s, "SDL_") {
			if n := goName(s); g.handNames[n] {
				b.WriteString(n)
				continue
			}
		}
		if strings.HasPrefix(line[loc[1]:], ".h") {
			b.WriteString(s) // a header file name such as SDL_gamepad.h
			continue
		}
		quoted := strings.Count(line[:loc[0]], "`")%2 == 1
		if g.docUnknown == nil {
			g.docUnknown = map[string]bool{}
		}
		g.docUnknown[s] = true
		if quoted {
			b.WriteString(s)
		} else {
			b.WriteString("`" + s + "`")
		}
	}
	b.WriteString(line[last:])
	return b.String()
}

// checkDocNames reports the SDL_ identifiers that docs mention although
// they are not bound, hand-written, skipped by the generator or allowlisted.
func (g *gen) checkDocNames() {
	var bad []string
	for s := range g.docUnknown {
		if g.skippedNames[s] || docAllow[s] {
			continue
		}
		bad = append(bad, s)
	}
	if len(bad) == 0 {
		return
	}
	sort.Strings(bad)
	g.errorf("docs mention SDL identifiers that are not bound, hand-written or allowlisted (docAllow in doc.go): %s", strings.Join(bad, ", "))
}

// loadHandNames collects the exported top-level identifiers of the
// hand-written (non-generated, non-test) Go files in dir.
func loadHandNames(dir string) map[string]bool {
	names := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	fset := token.NewFileSet()
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_gen.go") || strings.HasSuffix(base, "_test.go") {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			continue
		}
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					names[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								names[n.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return names
}
