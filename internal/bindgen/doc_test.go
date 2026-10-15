package bindgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/opengoui/puresdl/internal/ccfront"
	"github.com/opengoui/puresdl/internal/ir"
)

func testGen() *gen {
	return &gen{
		docNames:  map[string]string{"SDL_Init": "Init", "SDL_Window": "Window"},
		handNames: map[string]bool{"VERSIONNUM": true},
	}
}

func docOf(g *gen, s string) string { return strings.Join(g.docText(s), "\n") }

func TestDocParagraphsKeepOrderAfterReturns(t *testing.T) {
	got := docOf(testGen(), strings.ReplaceAll(`Do a thing.

\\param x the x.
\\returns true on success.

   An indented paragraph after the returns.

\\since This function is available since SDL 3.2.0.`, `\\`, `\`))
	want := `Do a thing.

Parameters:
  - x: the x.

Returns: true on success.

An indented paragraph after the returns.

Since: SDL 3.2.0.`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDocMarkup(t *testing.T) {
	g := testGen()
	cases := []struct{ name, in, want string }{
		{"wiki link", "see the [order that SDL expects](CategoryAudio#channel-layouts) here", "see the order that SDL expects here"},
		{"url link", "read [this post](https://example.com/a) now", "read this post (https://example.com/a) now"},
		{"bold", "**WARNING**: careful and ***very*** careful", "WARNING: careful and very careful"},
		{"bold across lines", "***Please note this\nspans lines!***", "Please note this\nspans lines!"},
		{"entities", "a &lt; b &amp;&amp; c &gt; d", "a < b && c > d"},
		{"inline tags", `call \\p SDL_Init then \\c foo() and \\a bar, \\b bold \\e em \\ref SDL_Window.`, "call `Init` then `foo()` and `bar`, bold em Window."},
		{"at tags", "@param a first\n@returns nothing", "Parameters:\n  - a: first\n\nReturns: nothing"},
		{"li", "Items:\n\\\\li one\n\\\\li two", "Items:\n- one\n- two"},
		{"sa inline", `Text. \\sa SDL_Init SDL_Window`, "Text.\n\nSee also: Init, Window"},
		{"group marks dropped", "text\n@{\n@}", "text"},
		{"unknown commands dropped", "\\\\ingroup Foo\ntext", "text"},
		{"collapse blanks", "a\n\n\n\nb", "a\n\nb"},
		{"param with direction", `\\param[in] x the x`, "Parameters:\n  - x: the x"},
		{"deprecated", `\\deprecated use Foo`, "Deprecated: use Foo"},
	}
	for _, c := range cases {
		if got := docOf(g, strings.ReplaceAll(c.in, `\\`, `\`)); got != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestDocTableAndCode(t *testing.T) {
	g := testGen()
	got := g.docText("Intro\n| a | b |\n| - | - |\n| 1 | **2** |\nOutro")
	want := []string{"Intro", "", "\t| a | b |", "\t| - | - |", "\t| 1 | 2 |", "", "Outro"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("table: got %q, want %q", got, want)
	}
	got = g.docText("Example:\n```c\nSDL_Init(0);\n\n\nfoo();\n```\nAfter.")
	want = []string{"Example:", "", "\tInit(0);", "", "\tfoo();", "", "After."}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("code: got %q, want %q", got, want)
	}
}

func TestDocList(t *testing.T) {
	got := docOf(testGen(), "Values:\n- \"a\": first one that\n  continues here.\n* second\n\nDone.")
	want := "Values:\n- \"a\": first one that\n  continues here.\n- second\n\nDone."
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRewriteNames(t *testing.T) {
	g := testGen()
	cases := map[string]string{
		"call SDL_Init()":                       "call Init()",
		"use SDL_VERSIONNUM here":               "use VERSIONNUM here",
		"uses SDL_strlen":                       "uses `SDL_strlen`",
		"already `SDL_strlen()` quoted":         "already `SDL_strlen()` quoted",
		"the SDL_HINT_* hints and SDL_PROP_":    "the SDL_HINT_* hints and SDL_PROP_",
		"see SDL_gamepad.h and SDL3 or SDL":     "see SDL_gamepad.h and SDL3 or SDL",
		"SDLK_ESCAPE is not a known name":       "`SDLK_ESCAPE` is not a known name",
		"bound SDL_Window wins over SDL_Window": "bound Window wins over Window",
	}
	for in, want := range cases {
		if got := g.rewriteNames(in); got != want {
			t.Errorf("rewriteNames(%q) = %q, want %q", in, got, want)
		}
	}
	for _, n := range []string{"SDL_strlen", "SDLK_ESCAPE"} {
		if !g.docUnknown[n] {
			t.Errorf("%s must be recorded as unknown", n)
		}
	}
}

func TestCheckDocNames(t *testing.T) {
	g := testGen()
	g.skipped = nil
	g.skip("SDL_libcThing", "libc replacement")
	g.rewriteNames("SDL_libcThing SDL_Mystery SDL_AppInit SDL_Another")
	g.checkDocNames()
	if len(g.errs) != 1 {
		t.Fatalf("errs = %q", g.errs)
	}
	e := g.errs[0]
	if !strings.Contains(e, "SDL_Mystery, SDL_Another") && !strings.Contains(e, "SDL_Another, SDL_Mystery") {
		t.Errorf("offenders missing from %q", e)
	}
	for _, ok := range []string{"SDL_libcThing", "SDL_AppInit"} {
		if strings.Contains(e, ok) {
			t.Errorf("%s is skipped or allowlisted and must not be reported: %q", ok, e)
		}
	}
}

func TestLoadHandNames(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("macros.go", "package sdl\nfunc FOURCC() {}\nfunc (T) Method() {}\nfunc private() {}\ntype Wchar int32\nconst Max = 1\nvar Err = 0\n")
	write("x_gen.go", "package sdl\nfunc Generated() {}\n")
	write("x_test.go", "package sdl\nfunc TestOnly() {}\n")
	got := loadHandNames(dir)
	for _, n := range []string{"FOURCC", "Wchar", "Max", "Err"} {
		if !got[n] {
			t.Errorf("%s missing from %v", n, got)
		}
	}
	for _, n := range []string{"Method", "private", "Generated", "TestOnly"} {
		if got[n] {
			t.Errorf("%s must not be collected", n)
		}
	}
}

func TestKeepAliveNote(t *testing.T) {
	ptr := &ir.Type{Kind: ir.Pointer, Size: 8, Elem: &ir.Type{Kind: ir.Void}}
	mk := func(doc, pdoc string, params ...ir.Param) *ir.Function {
		for i := range params {
			params[i].Doc = pdoc
		}
		return &ir.Function{Name: "SDL_F", Doc: doc, Sig: &ir.Sig{Params: params}}
	}
	p := ir.Param{Name: "mem", Type: ptr}
	for _, doc := range []string{
		"This memory buffer is not copied by the stream.",
		"the pointer you provide must remain valid until you close the stream.",
		"must stay valid while SDL is used",
		"The array must remain valid at least until the dialog is closed.",
		"SDL keeps a reference to it.",
	} {
		if keepAliveNote(mk(doc, "", p)) == "" {
			t.Errorf("no note for %q", doc)
		}
	}
	if !strings.HasPrefix(keepAliveNote(mk("", "is not copied, so it must stay valid", p)), "Keep alive:") {
		t.Error("parameter docs must be searched too")
	}
	if keepAliveNote(&ir.Function{Name: "SDL_PutNoCopy", Sig: &ir.Sig{Params: []ir.Param{p}}}) == "" {
		t.Error("NoCopy in the function name must match")
	}
	for _, doc := range []string{"Copies the buffer.", "The returned value will remain valid until the next call."} {
		if n := keepAliveNote(mk(doc, "", p)); n != "" {
			t.Errorf("unexpected note %q for %q", n, doc)
		}
	}
	if keepAliveNote(mk("not copied", "", ir.Param{Name: "n", Type: &ir.Type{Kind: ir.Int, Size: 4}})) != "" {
		t.Error("functions without pointer parameters get no note")
	}
	retained := mk("The memory is not copied.", "",
		ir.Param{Name: "mem", Type: ptr}, ir.Param{Name: "other", Type: ptr})
	if !retainedParam(retained, retained.Sig.Params[0]) || !retainedParam(retained, retained.Sig.Params[1]) {
		t.Error("function-level retention docs must conservatively retain every pointer parameter")
	}
	retained.Sig.Params[0].Doc = "the memory is not copied"
	retained.Sig.Params[1].Doc = "copied before returning"
	if !retainedParam(retained, retained.Sig.Params[0]) || retainedParam(retained, retained.Sig.Params[1]) {
		t.Error("parameter-level retention docs must identify only the retained pointer")
	}
}

func TestRetainedCStringUsesParameterDocumentation(t *testing.T) {
	filter := &ir.Type{Kind: ir.Pointer, Size: 8, Elem: &ir.Type{Kind: ir.Struct}}
	path := &ir.Type{Kind: ir.Pointer, Size: 8, Const: true, Elem: &ir.Type{Kind: ir.Int, Size: 1, Char: true}}
	f := &ir.Function{
		Name: "SDL_ShowOpenFileDialog",
		Doc:  "This is an asynchronous function.",
		Sig: &ir.Sig{Params: []ir.Param{
			{Name: "filters", Type: filter, Doc: "If non-NULL, it must remain valid at least until the callback is invoked."},
			{Name: "default_location", Type: path, Doc: "The default folder or file to start the dialog at."},
		}},
	}

	if !retainedParam(f, f.Sig.Params[0]) {
		t.Fatal("filters must be recognized as retained")
	}
	if retainedParam(f, f.Sig.Params[1]) {
		t.Fatal("an asynchronous function must not make an undocumented string parameter retained")
	}
	if !isCString(f.Sig.Params[1].Type) {
		t.Fatal("default_location must be recognized as a C string")
	}
}

func TestGeneratedDocsAreClean(t *testing.T) {
	api, _, err := ccfront.Load(ccfront.Options{IncludeDir: sdlInclude(t)})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := Generate(api, Options{OutDir: out, HandDir: "../../sdl"}); err != nil {
		t.Fatal(err)
	}
	bad := regexp.MustCompile("^\\s*//.*(\\*\\*|\\]\\(|&[a-z#0-9]+;| [\\\\@](?:ref|p|c|a|b|em|e|sa|li|param|returns?)\\b|``)")
	files, _ := filepath.Glob(filepath.Join(out, "*_gen.go"))
	if len(files) == 0 {
		t.Fatal("nothing generated")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i, l := range lines {
			if bad.MatchString(l) {
				t.Errorf("%s:%d: leftover doc markup: %s", filepath.Base(f), i+1, l)
			}
			if i > 0 && strings.TrimSpace(l) == "//" && strings.TrimSpace(lines[i-1]) == "//" {
				t.Errorf("%s:%d: double blank comment line", filepath.Base(f), i+1)
			}
		}
	}
}

// docLines returns the doc comment text of the named function in src, parsed
// as Go, so the test does not depend on how the comment is laid out.
func docLines(t *testing.T, src, name string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "gen.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
			if fd.Doc == nil {
				t.Fatalf("%s has no doc comment", name)
			}
			return fd.Doc.Text()
		}
	}
	t.Fatalf("function %s not generated", name)
	return ""
}

func TestSliceAndPrintfDocNotes(t *testing.T) {
	i32 := &ir.Type{Kind: ir.Int, Size: 4, Signed: true}
	cchar := &ir.Type{Kind: ir.Int, Size: 1, Signed: true, Char: true, Const: true}
	cfmt := ptrTo(cchar)
	cfmt.Const = true
	fmtParam := ir.Param{Name: "fmt", Type: cfmt}
	src := synthFile(t,
		&ir.Function{Name: "SDL_GetInts", Doc: "The caller should free the array with SDL_free().",
			Sig: &ir.Sig{Ret: ptrTo(i32), Params: []ir.Param{{Name: "count", Type: ptrTo(i32)}}}},
		&ir.Function{Name: "SDL_LogThing", Doc: "Log a thing.",
			Sig: &ir.Sig{Ret: &ir.Type{Kind: ir.Void}, Params: []ir.Param{fmtParam}, Variadic: true}},
	)
	got := docLines(t, src, "GetInts")
	want := "A nil result means SDL reported failure (see GetError); a non-nil empty slice means success with no items."
	if !strings.Contains(strings.Join(strings.Fields(got), " "), want) {
		t.Errorf("slice doc lacks the nil/empty line:\n%s", got)
	}
	got = docLines(t, src, "LogThing")
	want = "The message is formatted with fmt.Sprintf when args are given and used verbatim otherwise. Verbs follow Go fmt, not C printf: write %d instead of %u."
	if !strings.Contains(strings.Join(strings.Fields(got), " "), want) {
		t.Errorf("printf doc lacks the verb line:\n%s", got)
	}
}
