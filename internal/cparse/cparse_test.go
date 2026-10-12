package cparse

import (
	"strings"
	"testing"
)

func parseSrc(t *testing.T, src string) (*Result, *Preprocessor) {
	t.Helper()
	pp := NewPreprocessor()
	var p Parser
	p.ParseTokens("t.h", pp.Process("t.h", src))
	if len(p.Unit.Skipped) > 0 {
		t.Fatalf("skipped: %v", p.Unit.Skipped)
	}
	return &Result{Unit: &p.Unit, Env: pp.Env}, pp
}

func evalMacro(t *testing.T, pp *Preprocessor, name string) Value {
	t.Helper()
	v, err := pp.Env.Eval(pp.Env.Macros[name].Body)
	if err != nil {
		t.Fatalf("eval %s: %v", name, err)
	}
	return v
}

func TestFunctionDeclarations(t *testing.T) {
	r, _ := parseSrc(t, `
/**
 * Get the ticks.
 *
 * \returns ticks.
 * \since This function is available since SDL 3.2.0.
 */
extern SDL_DECLSPEC Uint64 SDLCALL SDL_GetTicks(void);
extern SDL_DECLSPEC SDL_MALLOC SDL_ALLOC_SIZE2(1, 2) void * SDLCALL SDL_calloc(size_t nmemb, size_t size);
extern SDL_DECLSPEC bool SDLCALL SDL_Fn(const char * const *names, int *w, SDL_Window **out, ...) SDL_PRINTF_VARARG_FUNC(1);
SDL_FORCE_INLINE bool SDL_Inline(const SDL_Rect *r) { return r == NULL; }
`)
	u := r.Unit
	if len(u.Funcs) != 3 || len(u.Inline) != 1 || u.Inline[0] != "SDL_Inline" {
		t.Fatalf("funcs=%d inline=%v", len(u.Funcs), u.Inline)
	}
	if f := u.Funcs[0]; f.Name != "SDL_GetTicks" || f.Ret.Name != "Uint64" || len(f.Params) != 0 || !strings.Contains(f.Doc, "Get the ticks.") {
		t.Fatalf("GetTicks: %+v", f)
	}
	if f := u.Funcs[1]; f.Ret.Name != "void" || f.Ret.Ptr != 1 || len(f.Params) != 2 {
		t.Fatalf("calloc: %+v", f)
	}
	f := u.Funcs[2]
	if !f.Variadic || len(f.Params) != 3 || f.Params[0].Type.Ptr != 2 || !f.Params[0].Type.Const || f.Params[2].Type.Ptr != 2 || f.Params[2].Name != "out" {
		t.Fatalf("Fn: %+v", f)
	}
}

func TestTypes(t *testing.T) {
	r, _ := parseSrc(t, `
typedef Uint32 SDL_InitFlags;
typedef struct SDL_Window SDL_Window;
typedef enum SDL_Kind {
    SDL_KIND_A, /**< first */
    SDL_KIND_B = 5,
    SDL_KIND_C
} SDL_Kind;
typedef struct SDL_Pt { float x, y; Uint8 data[16]; const char *name; } SDL_Pt;
typedef union SDL_U { Uint32 type; SDL_Pt pt; Uint8 padding[8]; } SDL_U;
typedef bool (SDLCALL *SDL_Cb)(void *userdata, const char *s);
struct SDL_Surface { int w; int h; };
`)
	u := r.Unit
	if len(u.Typedefs) != 6 || len(u.Enums) != 1 || len(u.Aggs) != 1 {
		t.Fatalf("typedefs=%d enums=%d aggs=%d", len(u.Typedefs), len(u.Enums), len(u.Aggs))
	}
	en := u.Enums[0]
	if en.Name != "SDL_Kind" || len(en.Members) != 3 || en.Members[0].Doc != "first" || en.Members[1].Expr == nil {
		t.Fatalf("enum: %+v", en)
	}
	pt := u.Typedefs[3].Type.Agg
	if len(pt.Fields) != 4 || pt.Fields[1].Name != "y" || !pt.Fields[2].Type.IsArray || pt.Fields[3].Type.Ptr != 1 {
		t.Fatalf("pt: %+v", pt.Fields)
	}
	if cb := u.Typedefs[5]; cb.Func == nil || len(cb.Func.Params) != 2 || cb.Func.Ret.Name != "bool" {
		t.Fatalf("callback: %+v", cb)
	}
}

func TestConditionals(t *testing.T) {
	r, _ := parseSrc(t, `
#ifdef SDL_WIKI_DOCUMENTATION_SECTION
extern SDL_DECLSPEC int SDLCALL Wiki(void);
#else
extern SDL_DECLSPEC int SDLCALL Real(void);
#endif
#if SDL_BYTEORDER == SDL_LIL_ENDIAN
typedef enum E { LIL = 1 } E;
#else
typedef enum E { BIG = 2 } E;
#endif
#ifdef SDL_PLATFORM_WINDOWS
extern SDL_DECLSPEC int SDLCALL Win(void);
#elif defined(SDL_PLATFORM_LINUX)
extern SDL_DECLSPEC int SDLCALL Lin(void);
#endif
`)
	var names []string
	for _, f := range r.Unit.Funcs {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "Real,Win,Lin" {
		t.Fatalf("funcs = %s", got)
	}
	if len(r.Unit.Enums) != 1 || r.Unit.Enums[0].Members[0].Name != "LIL" {
		t.Fatalf("enum = %+v", r.Unit.Enums)
	}
}

func TestEval(t *testing.T) {
	_, pp := parseSrc(t, `
#define A 0x10u
#define B (1u << 3)
#define C SDL_UINT64_C(0x8000000000000000)
#define D ((Uint32) 0xFFFFFFFFu)
#define E (A | B)
#define F(X) ((X) * 2)
#define G F(E)
#define H "SDL" "_hint"
#define I 9.80665f
#define J ((Sint32) 0xFFFFFFFE)
`)
	pp.Env.CastType = func(n string) (CastInfo, bool) {
		switch n {
		case "Uint32":
			return CastInfo{32, false}, true
		case "Sint32":
			return CastInfo{32, true}, true
		}
		return CastInfo{}, false
	}
	for name, want := range map[string]string{"A": "16", "B": "8", "C": "9223372036854775808", "D": "4294967295", "E": "24", "G": "48", "J": "-2"} {
		if got := evalMacro(t, pp, name).Int.String(); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if v := evalMacro(t, pp, "H"); v.Str != "SDL_hint" {
		t.Errorf("H = %q", v.Str)
	}
	if v := evalMacro(t, pp, "I"); v.Kind != VFloat || v.Float < 9.8 || v.Float > 9.81 {
		t.Errorf("I = %+v", v)
	}
}
