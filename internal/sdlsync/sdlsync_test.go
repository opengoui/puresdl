package sdlsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/opengoui/puresdl/internal/gensdl"
)

const fakeCommit = "829a65d76aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type entry struct {
	name     string
	body     string
	typeflag byte
	link     string
	size     int64 // overrides len(body) in the header when non-zero
}

// tarball builds a source tarball like git archive would: a pax global header
// with the commit, then the entries.
func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": fakeCommit}}); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o644, Linkname: e.link, Size: int64(len(e.body))}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
		}
		if e.size != 0 {
			h.Size = e.size
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			body := e.body
			if e.size != 0 {
				body = strings.Repeat("x", int(e.size))
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func versionHeader(major, minor, micro int) string {
	return fmt.Sprintf("#define SDL_MAJOR_VERSION %d\n#define SDL_MINOR_VERSION %d\n#define SDL_MICRO_VERSION %d\n", major, minor, micro)
}

// fakeTree returns a believable include/SDL3 tree for release-3.4.18.
func fakeTree(top, revision string) []entry {
	es := []entry{
		{name: top + "/", typeflag: tar.TypeDir},
		{name: top + "/README.md", body: "not extracted"},
		{name: top + "/src/SDL.c", body: "not extracted"},
		{name: top + "/include/", typeflag: tar.TypeDir},
		{name: top + "/include/SDL3/", typeflag: tar.TypeDir},
		{name: top + "/include/build_config/SDL_build_config.h", body: "not extracted"},
		{name: top + "/include/SDL3/SDL_version.h", body: versionHeader(3, 4, 18)},
		{name: top + "/include/SDL3/SDL_revision.h", body: revision},
	}
	for i := 0; i < 70; i++ {
		es = append(es, entry{name: fmt.Sprintf("%s/include/SDL3/SDL_h%02d.h", top, i), body: "/* header */\n"})
	}
	return es
}

const bakedRevision = "#ifdef DOC\n#define SDL_REVISION \"Some arbitrary string\"\n#elif defined(SDL_VENDOR_INFO)\n#define SDL_REVISION \"SDL-release-3.4.18-0-gbaked (\" SDL_VENDOR_INFO \")\"\n#else\n#define SDL_REVISION \"SDL-release-3.4.18-0-gbaked\"\n#endif\n"
const plainRevision = "#define SDL_REVISION \"\"\n"

func TestParseTag(t *testing.T) {
	tests := []struct {
		in   string
		pre  bool
		want string
		err  bool
	}{
		{in: "3.4.18", want: "release-3.4.18"},
		{in: "v3.4.18", want: "release-3.4.18"},
		{in: "release-3.4.18", want: "release-3.4.18"},
		{in: "  release-3.2.0\n", want: "release-3.2.0"},
		{in: "release-v3.4.18", want: "release-3.4.18"},
		{in: "prerelease-3.1.2", err: true},
		{in: "preview-3.1.3", err: true},
		{in: "3.5.0-rc1", err: true},
		{in: "prerelease-3.1.2", pre: true, want: "prerelease-3.1.2"},
		{in: "preview-3.1.3", pre: true, want: "preview-3.1.3"},
		{in: "3.5.0-rc1", pre: true, want: "release-3.5.0-rc1"},
		{in: "", err: true},
		{in: "3.4", err: true},
		{in: "main", err: true},
		{in: "release-3.4.x", err: true},
		{in: "../3.4.18", err: true},
	}
	for _, tc := range tests {
		got, err := ParseTag(tc.in, tc.pre)
		if tc.err {
			if err == nil {
				t.Errorf("ParseTag(%q, %v) = %q, want error", tc.in, tc.pre, got.Name)
			}
			continue
		}
		if err != nil || got.Name != tc.want {
			t.Errorf("ParseTag(%q, %v) = %q, %v; want %q", tc.in, tc.pre, got.Name, err, tc.want)
		}
	}
	if tag, _ := ParseTag("3.4.18", false); tag.Version() != "3.4.18" {
		t.Errorf("Version() = %q", tag.Version())
	}
}

func TestVersionFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SDL_VERSION")
	tag, _ := ParseTag("3.4.18", false)
	if changed, err := WriteVersionFile(p, tag); !changed || err != nil {
		t.Fatalf("first write: %v %v", changed, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "release-3.4.18\n" {
		t.Fatalf("content %q", b)
	}
	if changed, _ := WriteVersionFile(p, tag); changed {
		t.Error("identical write reported a change")
	}
	got, err := ReadVersionFile(p, false)
	if err != nil || got.Name != tag.Name {
		t.Fatalf("read %v %v", got, err)
	}
	os.WriteFile(p, []byte("a\nb\n"), 0o644)
	if _, err := ReadVersionFile(p, false); err == nil {
		t.Error("two lines accepted")
	}
	os.WriteFile(p, []byte("prerelease-3.1.2\n"), 0o644)
	if _, err := ReadVersionFile(p, false); err == nil {
		t.Error("prerelease pin accepted without allow")
	}
}

type fakeGitHub struct {
	*httptest.Server
	hits      atomic.Int32
	downloads atomic.Int32
	tarballs  map[string][]byte
	gotAgent  atomic.Value
	gotAuth   atomic.Value
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{tarballs: map[string][]byte{}}
	mux := http.NewServeMux()
	api := func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.gotAgent.Store(r.UserAgent())
		f.gotAuth.Store(r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/repos/libsdl-org/SDL/releases/latest":
			fmt.Fprint(w, `{"tag_name":"release-3.4.18","prerelease":false}`)
		case "/repos/libsdl-org/SDL/releases":
			fmt.Fprint(w, `[
{"tag_name":"preview-3.5.0","prerelease":true},
{"tag_name":"release-3.4.18","prerelease":false},
{"tag_name":"release-3.4.16","prerelease":false,"draft":true},
{"tag_name":"prerelease-3.1.2","prerelease":false},
{"tag_name":"release-3.4.14","prerelease":false},
{"tag_name":"release-3.4.12","prerelease":false}]`)
		default:
			http.NotFound(w, r)
		}
	}
	mux.HandleFunc("/repos/", api)
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		f.downloads.Add(1)
		tag := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/dl/"), ".tar.gz")
		b, ok := f.tarballs[tag]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGitHub) client() *Client {
	return &Client{APIURL: f.URL, DownloadURL: f.URL + "/dl", Token: "tok"}
}

func TestLatestAndList(t *testing.T) {
	f := newFakeGitHub(t)
	c := f.client()
	got, err := c.Latest(context.Background())
	if err != nil || got.Name != "release-3.4.18" {
		t.Fatalf("Latest = %v, %v", got, err)
	}
	if ua, _ := f.gotAgent.Load().(string); !strings.Contains(ua, "puresdl") {
		t.Errorf("User-Agent %q", ua)
	}
	if a, _ := f.gotAuth.Load().(string); a != "Bearer tok" {
		t.Errorf("Authorization %q", a)
	}
	tags, err := c.List(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	// preview (flagged), draft and prerelease-named tags are filtered out
	if want := "release-3.4.18 release-3.4.14 release-3.4.12"; strings.Join(names, " ") != want {
		t.Errorf("List = %v, want %s", names, want)
	}
}

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded for 1.2.3.4"}`)
	}))
	defer srv.Close()
	_, err := (&Client{APIURL: srv.URL}).Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "GITHUB_TOKEN") || !strings.Contains(err.Error(), "2023-11-14") {
		t.Fatalf("err = %v", err)
	}
	_, err = (&Client{APIURL: srv.URL, Token: "x"}).List(context.Background(), 1)
	if err == nil || strings.Contains(err.Error(), "set GITHUB_TOKEN") {
		t.Fatalf("with token: %v", err)
	}
	srv429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer srv429.Close()
	if _, err := (&Client{APIURL: srv429.URL}).Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("429: %v", err)
	}
	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	defer srv500.Close()
	if _, err := (&Client{APIURL: srv500.URL}).Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("500: %v", err)
	}
}

func TestLatestRejectsPrereleaseTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"preview-3.5.0"}`)
	}))
	defer srv.Close()
	if _, err := (&Client{APIURL: srv.URL}).Latest(context.Background()); err == nil {
		t.Fatal("preview tag accepted as latest")
	}
}

func TestSyncDownloadAndCache(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)...)
	tag, _ := ParseTag("3.4.18", false)
	cache := t.TempDir()
	s := &Syncer{Client: f.client(), CacheDir: cache}

	got, err := s.Sync(context.Background(), tag)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Downloaded || f.downloads.Load() != 1 {
		t.Fatalf("expected one download: %+v, %d", got, f.downloads.Load())
	}
	// 72 headers extracted, nothing outside include/SDL3
	if got.Files != 72 || got.Version != "3.4.18" || len(got.SHA256) != 64 {
		t.Errorf("meta = %+v", got.Meta)
	}
	if got.Revision != "SDL-release-3.4.18-0-g829a65d76" || got.RevisionSource != "tarball" {
		t.Errorf("revision = %q (%s)", got.Revision, got.RevisionSource)
	}
	if want := filepath.Join(cache, "release-3.4.18", "include"); got.Include != want {
		t.Errorf("Include = %s, want %s", got.Include, want)
	}
	for _, p := range []string{"README.md", "src", "include/build_config", ".sync.json"} {
		_, err := os.Stat(filepath.Join(cache, "release-3.4.18", p))
		if (p == ".sync.json") != (err == nil) {
			t.Errorf("%s: stat err = %v", p, err)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(cache, ".tmp-*")); len(m) != 0 {
		t.Errorf("temp dirs left behind: %v", m)
	}

	// hit: no second download, even with a dead server
	f.Close()
	again, err := s.Sync(context.Background(), tag)
	if err != nil || again.Downloaded || again.SHA256 != got.SHA256 {
		t.Fatalf("cache hit: %+v, %v", again, err)
	}
}

func TestSyncBakedRevision(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL3-3.4.18", bakedRevision)...)
	tag, _ := ParseTag("3.4.18", false)
	got, err := (&Syncer{Client: f.client(), CacheDir: t.TempDir()}).Sync(context.Background(), tag)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != "SDL-release-3.4.18-0-gbaked" || got.RevisionSource != "header" {
		t.Errorf("revision = %q (%s)", got.Revision, got.RevisionSource)
	}
}

func TestSyncCacheMiss(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)...)
	tag, _ := ParseTag("3.4.18", false)
	cache := t.TempDir()
	s := &Syncer{Client: f.client(), CacheDir: cache}
	if _, err := s.Sync(context.Background(), tag); err != nil {
		t.Fatal(err)
	}
	// corrupt entry: a header goes missing -> re-download
	os.Remove(filepath.Join(cache, "release-3.4.18", "include", "SDL3", "SDL_h00.h"))
	got, err := s.Sync(context.Background(), tag)
	if err != nil || !got.Downloaded || f.downloads.Load() != 2 {
		t.Fatalf("corrupt cache not repaired: %+v %v %d", got, err, f.downloads.Load())
	}
	// unknown tag -> 404 mentions the URL
	other, _ := ParseTag("3.9.9", false)
	if _, err := s.Sync(context.Background(), other); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "release-3.9.9")); err == nil {
		t.Error("failed download left a cache entry")
	}
}

func TestSyncOffline(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)...)
	tag, _ := ParseTag("3.4.18", false)
	cache := t.TempDir()
	off := &Syncer{Client: f.client(), CacheDir: cache, Offline: true}
	if _, err := off.Sync(context.Background(), tag); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("offline miss: %v", err)
	}
	if f.downloads.Load() != 0 {
		t.Fatal("offline mode downloaded")
	}
	if _, err := (&Syncer{Client: f.client(), CacheDir: cache}).Sync(context.Background(), tag); err != nil {
		t.Fatal(err)
	}
	if got, err := off.Sync(context.Background(), tag); err != nil || got.Downloaded {
		t.Fatalf("offline hit: %+v %v", got, err)
	}
}

func TestSyncVersionMismatch(t *testing.T) {
	f := newFakeGitHub(t)
	es := fakeTree("SDL-release-3.4.18", plainRevision)
	es[6].body = versionHeader(3, 5, 0)
	f.tarballs["release-3.4.18"] = tarball(t, es...)
	tag, _ := ParseTag("3.4.18", false)
	_, err := (&Syncer{Client: f.client(), CacheDir: t.TempDir()}).Sync(context.Background(), tag)
	if err == nil || !strings.Contains(err.Error(), "3.5.0") {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncTooFewHeaders(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)[:20]...)
	tag, _ := ParseTag("3.4.18", false)
	if _, err := (&Syncer{Client: f.client(), CacheDir: t.TempDir()}).Sync(context.Background(), tag); err == nil {
		t.Fatal("tiny tarball accepted")
	}
}

func TestExtractRejectsMaliciousEntries(t *testing.T) {
	tests := []struct {
		name string
		e    entry
		want string
	}{
		{"traversal", entry{name: "SDL-x/include/SDL3/../../../evil.h", body: "x"}, "unsafe path"},
		{"traversal outside include", entry{name: "../evil", body: "x"}, "unsafe path"},
		{"absolute", entry{name: "/etc/passwd", body: "x"}, "absolute"},
		{"backslash", entry{name: `SDL-x\include\SDL3\a.h`, body: "x"}, "absolute"},
		{"symlink", entry{name: "SDL-x/include/SDL3/SDL_link.h", typeflag: tar.TypeSymlink, link: "/etc/passwd"}, "not a regular file"},
		{"hardlink", entry{name: "SDL-x/include/SDL3/SDL_link.h", typeflag: tar.TypeLink, link: "SDL-x/README"}, "not a regular file"},
		{"huge file", entry{name: "SDL-x/include/SDL3/SDL_big.h", size: maxFileSize + 1}, "size limits"},
		{"nested", entry{name: "SDL-x/include/SDL3/sub/a.h", body: "x"}, "nested"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := t.TempDir()
			data := tarball(t, entry{name: "SDL-x/include/SDL3/SDL_ok.h", body: "ok"}, tc.e)
			_, err := Extract(bytes.NewReader(data), dst)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			// nothing escaped the destination
			parent := filepath.Dir(dst)
			for _, p := range []string{"evil.h", "evil", "passwd"} {
				if _, err := os.Stat(filepath.Join(parent, p)); err == nil {
					t.Errorf("%s escaped", p)
				}
			}
		})
	}
}

func TestExtractIgnoresLinksOutsideInclude(t *testing.T) {
	data := tarball(t,
		entry{name: "SDL-x/docs/link", typeflag: tar.TypeSymlink, link: "README.md"},
		entry{name: "SDL-x/include/SDL3/SDL_ok.h", body: "ok"},
	)
	dst := t.TempDir()
	m, err := Extract(bytes.NewReader(data), dst)
	if err != nil || m.Files != 1 || m.Commit != fakeCommit {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestExtractNotGzip(t *testing.T) {
	if _, err := Extract(strings.NewReader("<html>rate limited</html>"), t.TempDir()); err == nil {
		t.Fatal("html accepted")
	}
}

func TestMainCLI(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)...)
	root := t.TempDir()
	cache := t.TempDir()
	env := func(k string) string {
		if k == "GITHUB_TOKEN" {
			return "tok"
		}
		return ""
	}
	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		args = append([]string{"-root", root, "-cache-dir", cache, "-github-api-url", f.URL, "-download-url", f.URL + "/dl"}, args...)
		code := Main(context.Background(), args, env, &out, &errb)
		return out.String(), errb.String(), code
	}

	// no SDL_VERSION and no flag
	if _, _, code := run(); code == 0 {
		t.Fatal("missing SDL_VERSION accepted")
	}
	if out, _, code := run("-list", "-n", "2"); code != 0 || out != "release-3.4.18\nrelease-3.4.14\n" {
		t.Fatalf("-list: %q %d", out, code)
	}
	// -latest without -write leaves SDL_VERSION alone
	if _, e, code := run("-latest"); code != 0 {
		t.Fatalf("-latest: %s", e)
	}
	if _, err := os.Stat(filepath.Join(root, "SDL_VERSION")); err == nil {
		t.Fatal("-latest wrote SDL_VERSION without -write")
	}
	out, e, code := run("-latest", "-write", "-print-include")
	if code != 0 || strings.TrimSpace(out) != filepath.Join(cache, "release-3.4.18", "include") {
		t.Fatalf("-write: %q %s", out, e)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "SDL_VERSION")); string(b) != "release-3.4.18\n" {
		t.Fatalf("SDL_VERSION = %q", b)
	}
	if !strings.Contains(e, "SDL_VERSION: (none) -> release-3.4.18") || !strings.Contains(e, "SDL_REVISION") {
		t.Errorf("stderr = %s", e)
	}
	// second write is a no-op, default version comes from the file
	if _, e, code := run("-write", "-offline"); code != 0 || !strings.Contains(e, "already") {
		t.Fatalf("rewrite: %s", e)
	}
	// prerelease needs the flag
	if _, _, code := run("-version", "preview-3.1.3"); code == 0 {
		t.Fatal("preview accepted")
	}
	if _, _, code := run("-version", "3.4.18", "-latest"); code == 0 {
		t.Fatal("-version with -latest accepted")
	}
	if _, _, code := run("-offline", "-latest"); code == 0 {
		t.Fatal("-offline with -latest accepted")
	}
	if _, _, code := run("-version", "3.4.17", "-offline"); code == 0 {
		t.Fatal("offline without cache accepted")
	}
}

// stubGenerate stands in for the real pipeline and writes deterministic
// files derived from the tag.
func stubGenerate(content func(gensdl.Config) map[string]string) func(gensdl.Config) error {
	return func(c gensdl.Config) error {
		old, _ := filepath.Glob(filepath.Join(c.Out, "*_gen*.go"))
		for _, o := range old {
			os.Remove(o)
		}
		for n, b := range content(c) {
			if err := os.WriteFile(filepath.Join(c.Out, n), []byte(b), 0o644); err != nil {
				return err
			}
		}
		if c.Skipped != "" {
			return os.WriteFile(c.Skipped, []byte("SDL_memcpy\n"), 0o644)
		}
		return nil
	}
}

func TestGenerateAndCheck(t *testing.T) {
	f := newFakeGitHub(t)
	f.tarballs["release-3.4.18"] = tarball(t, fakeTree("SDL-release-3.4.18", plainRevision)...)
	root := t.TempDir()
	cache := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sdl"), 0o755)
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "SDL_VERSION"), []byte("release-3.4.18\n"), 0o644)
	os.WriteFile(filepath.Join(root, "sdl", "hand.go"), []byte("package sdl\n"), 0o644)

	body := "A"
	var seen gensdl.Config
	old := Generate
	t.Cleanup(func() { Generate = old })
	Generate = func(c gensdl.Config) error {
		seen = c
		return stubGenerate(func(c gensdl.Config) map[string]string {
			return map[string]string{
				"a_gen.go":           "package sdl // " + body + " " + c.Tag + "\n",
				"layout_gen_test.go": "package sdl\n",
			}
		})(c)
	}
	run := func(args ...string) (string, int) {
		var out, errb bytes.Buffer
		args = append([]string{"-root", root, "-cache-dir", cache, "-github-api-url", f.URL, "-download-url", f.URL + "/dl"}, args...)
		code := Main(context.Background(), args, func(string) string { return "" }, &out, &errb)
		return errb.String(), code
	}

	// check before generating: the repository has no generated files
	if e, code := run("-check"); code == 0 || !strings.Contains(e, "missing from the repository: a_gen.go") {
		t.Fatalf("check on empty repo: %d %s", code, e)
	}
	if e, code := run("-generate"); code != 0 {
		t.Fatalf("-generate: %s", e)
	}
	if seen.Tag != "release-3.4.18" || seen.Revision != "SDL-release-3.4.18-0-g829a65d76" || seen.HandDir != filepath.Join(root, "sdl") || seen.Out != filepath.Join(root, "sdl") {
		t.Errorf("config = %+v", seen)
	}
	if e, code := run("-check", "-offline"); code != 0 {
		t.Fatalf("clean check failed: %s", e)
	}
	// the check must not touch the repository
	if b, _ := os.ReadFile(filepath.Join(root, "sdl", "a_gen.go")); !strings.Contains(string(b), "A release-3.4.18") {
		t.Fatalf("a_gen.go = %q", b)
	}

	// drift 1: generator output changes
	body = "B"
	e, code := run("-check", "-offline")
	if code == 0 || !strings.Contains(e, "changed: a_gen.go") || !strings.Contains(e, "line 1") {
		t.Fatalf("drift not detected: %d %s", code, e)
	}
	body = "A"

	// drift 2: skipped list edited by hand
	os.WriteFile(filepath.Join(root, "docs", "skipped.txt"), []byte("SDL_memcpy\nextra\n"), 0o644)
	if e, code := run("-check", "-offline"); code == 0 || !strings.Contains(e, "changed: skipped:skipped.txt") {
		t.Fatalf("skipped drift: %d %s", code, e)
	}
	os.WriteFile(filepath.Join(root, "docs", "skipped.txt"), []byte("SDL_memcpy\n"), 0o644)

	// drift 3: stale generated file in the repository
	os.WriteFile(filepath.Join(root, "sdl", "stale_gen.go"), []byte("package sdl\n"), 0o644)
	if e, code := run("-check", "-offline"); code == 0 || !strings.Contains(e, "no longer generated: stale_gen.go") {
		t.Fatalf("stale file: %d %s", code, e)
	}
	os.Remove(filepath.Join(root, "sdl", "stale_gen.go"))

	// hand-written files are never compared
	os.WriteFile(filepath.Join(root, "sdl", "hand.go"), []byte("package sdl // edited\n"), 0o644)
	if e, code := run("-check", "-offline"); code != 0 {
		t.Fatalf("hand-written change flagged: %s", e)
	}
	if _, code := run("-check", "-generate"); code == 0 {
		t.Fatal("-check with -generate accepted")
	}
}
