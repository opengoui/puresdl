package cparse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Result bundles everything extracted from a header set.
type Result struct {
	Unit  *Unit
	Env   *Env
	Order []string // macro names in definition order
}

// ParseHeaders preprocesses and parses the named headers (relative to dir)
// in the given order. Macros are shared across headers.
func ParseHeaders(dir string, names []string) (*Result, error) {
	pp := NewPreprocessor()
	var p Parser
	for _, n := range names {
		src, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		toks := pp.Process(n, string(src))
		p.ParseTokens(n, toks)
	}
	res := &Result{Unit: &p.Unit, Env: pp.Env}
	type nm struct {
		name string
		m    *Macro
	}
	var all []nm
	for k, m := range pp.Env.Macros {
		all = append(all, nm{k, m})
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i].m, all[j].m
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	for _, x := range all {
		res.Order = append(res.Order, x.name)
	}
	if len(res.Unit.Funcs) == 0 {
		return nil, fmt.Errorf("cparse: no functions found in %s", dir)
	}
	return res, nil
}
