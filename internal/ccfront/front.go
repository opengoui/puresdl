// Package ccfront parses the SDL3 headers with modernc.org/cc/v4 for one or
// more target platforms and converts them to the ir model. It is hermetic:
// it needs no C compiler or SDK because it embeds the predefined macros of
// each target and a handful of libc stub headers.
package ccfront

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"modernc.org/cc/v4"

	"github.com/opengoui/puresdl/internal/ir"
)

//go:embed data
var dataFS embed.FS

// Target is one platform whose #if branches are evaluated.
type Target struct {
	GOOS, GOARCH string
	// Triple names the predefined macro file (captured with
	// clang --target=<triple> -dM -E).
	Triple string
}

// Targets are the platforms merged into the generated API. The first one
// supplies the type layouts; the others add the functions that only exist
// there and are cross-checked for layout differences.
var Targets = []Target{
	{"darwin", "arm64", "arm64-apple-darwin"},
	{"linux", "amd64", "x86_64-unknown-linux-gnu"},
	{"linux", "arm64", "aarch64-unknown-linux-gnu"},
	{"windows", "amd64", "x86_64-pc-windows-msvc"},
	{"windows", "arm64", "aarch64-pc-windows-msvc"},
}

// Options configures Load.
type Options struct {
	// IncludeDir is the directory that contains the SDL3 header directory.
	IncludeDir string
	Targets    []Target
}

// excluded names the headers that are not part of the libSDL3 ABI or are
// compatibility or third-party material.
var excluded = map[string]bool{
	"SDL_oldnames.h": true, "SDL_egl.h": true, "SDL_intrin.h": true, "SDL_main_impl.h": true,
	"SDL_dlopennote.h": true, "SDL_begin_code.h": true, "SDL_close_code.h": true, "SDL_revision.h": true,
	"SDL_copying.h": true, "SDL_platform_defines.h": true,
}

// headerExcluded reports whether the header with base name base is skipped.
func headerExcluded(base string) bool {
	return excluded[base] || strings.HasPrefix(base, "SDL_opengl") || strings.HasPrefix(base, "SDL_test")
}

// umbrella is the translation unit that pulls in every public SDL3 header.
const umbrella = "#include <SDL3/SDL.h>\n#include <SDL3/SDL_main.h>\n#include <SDL3/SDL_vulkan.h>\n"

// Load parses the headers for every target and returns the merged API and
// non-fatal warnings (for example layout differences between targets).
func Load(o Options) (*ir.API, []string, error) {
	if len(o.Targets) == 0 {
		o.Targets = Targets
	}
	dir, err := os.MkdirTemp("", "ccfront-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)
	if err := extract(dir); err != nil {
		return nil, nil, err
	}
	sdlDir := filepath.Join(o.IncludeDir, "SDL3")
	c := newConverter(sdlDir)
	funcs := map[string]*ir.Function{}
	for i, t := range o.Targets {
		ast, err := translate(o.IncludeDir, dir, t)
		if err != nil {
			return nil, nil, fmt.Errorf("%s/%s: %w", t.GOOS, t.GOARCH, err)
		}
		c.collectDocs(ast)
		c.collectFuncs(ast, t, funcs)
		c.collectTypedefs(ast)
		if i == 0 {
			c.collectConsts(ast)
		}
	}
	return c.api, c.warnings, nil
}

// extract writes the embedded data tree (predefined macros, stub headers)
// under dst.
func extract(dst string) error {
	return fs.WalkDir(dataFS, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dst, strings.TrimPrefix(p, "data"))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := dataFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
}

// translate parses the umbrella header for target t. includeDir holds the
// SDL3 headers and dataDir the tree written by extract.
func translate(includeDir, dataDir string, t Target) (*cc.AST, error) {
	abi, err := cc.NewABI(t.GOOS, t.GOARCH)
	if err != nil {
		return nil, err
	}
	predef, err := os.ReadFile(filepath.Join(dataDir, "predef", t.Triple+".h"))
	if err != nil {
		return nil, err
	}
	stubs := filepath.Join(dataDir, "stubs")
	p := string(predef)
	if t.GOOS == "windows" {
		stubs = filepath.Join(dataDir, "stubs_win")
		p += "\n#define __cdecl\n#define __stdcall\n#define __fastcall\n#define __declspec(x)\n#define __forceinline inline\n#define __inline inline\n#define __ptr64\n#define __unaligned\n"
	}
	cfg := &cc.Config{
		ABI:             abi,
		SysIncludePaths: []string{includeDir, stubs},
		IncludePaths:    []string{"", includeDir, stubs},
		EvalAllMacros:   true,
		Header:          true,
	}
	return cc.Translate(cfg, []cc.Source{
		{Name: "<predefined>", Value: p},
		{Name: "<builtin>", Value: cc.Builtin},
		{Name: "umbrella.c", Value: umbrella},
	})
}

// walkDecls calls fn for each top-level declaration in ast, in source order.
func walkDecls(ast *cc.AST, fn func(*cc.Declaration)) {
	for tu := ast.TranslationUnit; tu != nil; tu = tu.TranslationUnit {
		if ed := tu.ExternalDeclaration; ed != nil && ed.Declaration != nil {
			fn(ed.Declaration)
		}
	}
}

// declarators returns the declarators introduced by d.
func declarators(d *cc.Declaration) (r []*cc.Declarator) {
	for l := d.InitDeclaratorList; l != nil; l = l.InitDeclaratorList {
		if l.InitDeclarator != nil && l.InitDeclarator.Declarator != nil {
			r = append(r, l.InitDeclarator.Declarator)
		}
	}
	return
}

// wanted reports whether declarations in the file at pos belong to the API:
// they must come from a non-excluded SDL header.
func (c *converter) wanted(pos string) bool {
	return c.inSDL(pos) && !headerExcluded(filepath.Base(pos))
}

// collectDocs records the doxygen block above every declarator.
func (c *converter) collectDocs(ast *cc.AST) {
	walkDecls(ast, func(d *cc.Declaration) {
		toks := cc.NodeTokens(d)
		if len(toks) == 0 {
			return
		}
		pos := toks[0].Position()
		if !c.inSDL(pos.Filename) {
			return
		}
		doc := docBefore(c.src.get(pos.Filename), pos.Offset)
		for _, dd := range declarators(d) {
			c.docs[dd] = doc
		}
	})
}

// collectFuncs adds the externally linked functions declared in ast to the
// API. funcs indexes functions by name across targets, so a function seen on
// several targets is recorded once with each target in Targets.
func (c *converter) collectFuncs(ast *cc.AST, t Target, funcs map[string]*ir.Function) {
	tn := t.GOOS + "/" + t.GOARCH
	walkDecls(ast, func(d *cc.Declaration) {
		for _, dd := range declarators(d) {
			if dd.IsTypename() || dd.Type() == nil || dd.Type().Kind() != cc.Function || dd.IsStatic() || dd.IsInline() || strings.HasPrefix(dd.Name(), "_") {
				continue
			}
			pos := dd.Position()
			if !c.wanted(pos.Filename) {
				continue
			}
			if f, ok := funcs[dd.Name()]; ok {
				f.Targets = append(f.Targets, tn)
				continue
			}
			sig := c.sig(dd.Type().(*cc.FunctionType))
			doc := c.docs[dd]
			pd := paramDocs(doc)
			for i := range sig.Params {
				sig.Params[i].Doc = pd[sig.Params[i].Name]
			}
			f := &ir.Function{Name: dd.Name(), Sig: sig, Doc: doc, File: filepath.Base(pos.Filename), Targets: []string{tn}}
			funcs[f.Name] = f
			c.api.Funcs = append(c.api.Funcs, f)
		}
	})
}

// collectTypedefs converts every SDL typedef declared in ast.
func (c *converter) collectTypedefs(ast *cc.AST) {
	walkDecls(ast, func(d *cc.Declaration) {
		for _, dd := range declarators(d) {
			if dd.IsTypename() && c.wanted(dd.Position().Filename) {
				c.typedefType(dd)
			}
		}
	})
}

// collectConsts adds the constant object-like macros of ast to the API,
// sorted by header file path and then by position in the file, and derives API.Version from SDL_*_VERSION.
func (c *converter) collectConsts(ast *cc.AST) {
	type entry struct {
		m    *cc.Macro
		name string
		pos  struct {
			file string
			line int
			off  int
		}
	}
	var all []entry
	for name, m := range ast.Macros {
		if m.IsFnLike || !m.IsConst || strings.HasPrefix(name, "_") {
			continue
		}
		p := m.Position()
		if !c.wanted(p.Filename) {
			continue
		}
		e := entry{m: m, name: name}
		e.pos.file, e.pos.line, e.pos.off = p.Filename, p.Line, p.Offset
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.pos.file != b.pos.file {
			return a.pos.file < b.pos.file
		}
		return a.pos.off < b.pos.off
	})
	versions := map[string]string{}
	for _, e := range all {
		k := &ir.Const{Name: e.name, File: filepath.Base(e.pos.file), Offset: e.pos.off}
		switch v := e.m.Value().(type) {
		case cc.Int64Value:
			k.Kind, k.Int = ir.ConstInt, bigInt(int64(v))
		case cc.UInt64Value:
			k.Kind, k.Int = ir.ConstInt, bigUint(uint64(v))
		case cc.Float64Value:
			k.Kind, k.Float = ir.ConstFloat, float64(v)
		case cc.StringValue:
			k.Kind, k.Str = ir.ConstString, strings.TrimSuffix(string(v), "\x00")
		default:
			continue
		}
		if e.name == "SDL_LINE" || e.name == "SDL_FILE" || e.name == "SDL_FUNCTION" || e.name == "SDL_ASSERT_FILE" {
			// these expand to __FILE__/__LINE__/__func__ and would make the
			// output depend on where the headers live
			continue
		}
		src := c.src.get(e.pos.file)
		// the doc block sits above the '#' of the #define line
		lineStart := e.pos.off
		for lineStart > 0 && src[lineStart-1] != '\n' {
			lineStart--
		}
		k.Doc = docBefore(src, lineStart)
		if k.Doc == "" {
			k.Doc = lineTrailingDoc(src, lineStart)
		}
		if k.Kind == ir.ConstInt {
			versions[e.name] = k.Int.String()
		}
		if k.Kind == ir.ConstInt {
			k.After = c.precedingTypedef(e.pos.file, e.pos.off)
		}
		c.api.Consts = append(c.api.Consts, k)
	}
	if ma, ok := versions["SDL_MAJOR_VERSION"]; ok {
		c.api.Version = fmt.Sprintf("%s.%s.%s", ma, versions["SDL_MINOR_VERSION"], versions["SDL_MICRO_VERSION"])
	}
}

// paramDocs extracts "\param name text" entries from a doxygen block.
func paramDocs(doc string) map[string]string {
	out := map[string]string{}
	var name string
	for _, l := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "\\param "):
			f := strings.Fields(strings.TrimPrefix(t, "\\param "))
			if len(f) == 0 {
				name = ""
				continue
			}
			name = f[0]
			out[name] = strings.Join(f[1:], " ")
		case t == "" || strings.HasPrefix(t, "\\"):
			name = ""
		case name != "":
			out[name] += " " + t
		}
	}
	return out
}
