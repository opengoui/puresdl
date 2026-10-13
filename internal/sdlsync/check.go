package sdlsync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Compare reports how the generated files in gotDir (and the skipped list at
// gotSkipped) differ from the checked-in ones in wantDir and wantSkipped. It
// looks at *_gen.go and *_gen_test.go files and the skipped list only. The result is empty when nothing
// differs, otherwise one line per added, removed or changed file.
func Compare(gotDir, gotSkipped, wantDir, wantSkipped string) ([]string, error) {
	list := func(dir string) (map[string]string, error) {
		m := map[string]string{}
		for _, pat := range []string{"*_gen.go", "*_gen_test.go"} {
			fs, err := filepath.Glob(filepath.Join(dir, pat))
			if err != nil {
				return nil, err
			}
			for _, f := range fs {
				m[filepath.Base(f)] = f
			}
		}
		return m, nil
	}
	got, err := list(gotDir)
	if err != nil {
		return nil, err
	}
	want, err := list(wantDir)
	if err != nil {
		return nil, err
	}
	if gotSkipped != "" || wantSkipped != "" {
		got["skipped:"+filepath.Base(wantSkipped)] = gotSkipped
		want["skipped:"+filepath.Base(wantSkipped)] = wantSkipped
	}
	var names []string
	seen := map[string]bool{}
	for n := range got {
		seen[n] = true
		names = append(names, n)
	}
	for n := range want {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		g, gok := got[n]
		w, wok := want[n]
		switch {
		case gok && !wok:
			out = append(out, "missing from the repository: "+n)
		case !gok && wok:
			out = append(out, "no longer generated: "+n)
		default:
			gb, gerr := os.ReadFile(g)
			wb, werr := os.ReadFile(w)
			if gerr != nil || werr != nil {
				if gerr != nil && werr != nil {
					continue // e.g. no skipped list on either side
				}
				out = append(out, fmt.Sprintf("unreadable: %s", n))
				continue
			}
			if !bytes.Equal(gb, wb) {
				out = append(out, fmt.Sprintf("changed: %s (%s)", n, firstDiff(wb, gb)))
			}
		}
	}
	return out, nil
}

// firstDiff describes the first line at which want and got differ, with both
// line counts, for use in Compare's report.
func firstDiff(want, got []byte) string {
	wl := strings.Split(string(want), "\n")
	gl := strings.Split(string(got), "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var a, b string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			b = gl[i]
		}
		if a != b || i >= len(wl) || i >= len(gl) {
			return fmt.Sprintf("line %d: have %q, generated %q; %d vs %d lines", i+1, clip(a), clip(b), len(wl), len(gl))
		}
	}
	return "content differs"
}

// clip truncates s to 80 bytes, appending an ellipsis when it cut anything.
func clip(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
