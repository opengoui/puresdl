package sdlsync

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxFileSize  = 4 << 20  // largest single header accepted
	maxTotalSize = 32 << 20 // all extracted headers together
	maxFiles     = 400
	minHeaders   = 60 // a real release has ~87
	includeDir   = "include/SDL3/"
	metaName     = ".sync.json"
)

// Meta is stored as <cache>/<tag>/.sync.json.
type Meta struct {
	Tag      string `json:"tag"`
	Version  string `json:"version"`
	Revision string `json:"revision"`
	// RevisionSource is "header" when SDL_REVISION is baked into
	// SDL_revision.h and "tarball" when it was derived from the tag and the
	// commit recorded in the tarball.
	RevisionSource string `json:"revision_source"`
	Commit         string `json:"commit,omitempty"`
	SHA256         string `json:"sha256"`
	Files          int    `json:"files"`
}

// Synced describes a validated header tree in the cache.
type Synced struct {
	Meta
	// Include is the directory that contains SDL3/SDL.h.
	Include string
	// Downloaded is false when the cache was reused.
	Downloaded bool
}

// Syncer maintains the header cache.
type Syncer struct {
	Client   *Client
	CacheDir string
	// Offline forbids network access: only a valid cache entry is accepted.
	Offline bool
}

// DefaultCacheDir returns os.UserCacheDir()/puresdl/sdl.
func DefaultCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "puresdl", "sdl"), nil
}

// Sync makes sure <cache>/<tag>/include/SDL3 holds the validated headers of
// tag, downloading them when the cache entry is missing or invalid.
func (s *Syncer) Sync(ctx context.Context, tag Tag) (*Synced, error) {
	if s.CacheDir == "" {
		return nil, errors.New("sdlsync: cache directory is not set")
	}
	dir := filepath.Join(s.CacheDir, tag.Name)
	if m, err := validate(dir, tag); err == nil {
		return &Synced{Meta: *m, Include: filepath.Join(dir, "include")}, nil
	} else if s.Offline {
		return nil, fmt.Errorf("sdlsync: -offline: no valid cache for %s in %s: %w", tag.Name, s.CacheDir, err)
	}
	if s.Client == nil {
		s.Client = &Client{}
	}
	if err := os.MkdirAll(s.CacheDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(s.CacheDir, ".tmp-"+tag.Name+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	meta, err := s.download(ctx, tag, tmp)
	if err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, metaName), append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	meta, err = validate(tmp, tag)
	if err != nil {
		return nil, err
	}
	// store the validated version and revision too
	b, _ = json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, metaName), append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return nil, err
	}
	return &Synced{Meta: *meta, Include: filepath.Join(dir, "include"), Downloaded: true}, nil
}

func (s *Syncer) download(ctx context.Context, tag Tag, dst string) (*Meta, error) {
	url := s.Client.downloadURL() + "/" + tag.Name + ".tar.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := s.Client.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("sdlsync: downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sdlsync: downloading %s: %s", url, resp.Status)
	}
	h := sha256.New()
	meta, err := Extract(io.TeeReader(resp.Body, h), dst)
	if err != nil {
		return nil, fmt.Errorf("sdlsync: %s: %w", url, err)
	}
	io.Copy(io.Discard, resp.Body) // hash the trailer too
	meta.Tag = tag.Name
	meta.SHA256 = hex.EncodeToString(h.Sum(nil))
	return meta, nil
}

// Extract reads a gzip-compressed source tarball from r and writes only the
// regular files below <top>/include/SDL3/ into dst/include/SDL3/. It rejects
// absolute paths, ".." components, links and oversize entries. The returned
// Meta has Files and Commit set (the commit comes from the pax global header
// that git archive writes).
func Extract(r io.Reader, dst string) (*Meta, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a gzip stream: %w", err)
	}
	tr := tar.NewReader(zr)
	meta := &Meta{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			if c := h.PAXRecords["comment"]; c != "" {
				meta.Commit = c
			}
			continue
		}
		name := h.Name
		if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || filepath.IsAbs(name) || strings.Contains(name, `\`) {
			return nil, fmt.Errorf("unsafe absolute path %q in tarball", name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return nil, fmt.Errorf("unsafe path %q in tarball", name)
			}
		}
		// drop the top-level directory (SDL-release-3.4.18/)
		_, rel, ok := strings.Cut(path.Clean(name), "/")
		if !ok || !strings.HasPrefix(rel+"/", includeDir) {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, fmt.Errorf("%q in %s is not a regular file (type %q)", name, includeDir, h.Typeflag)
		}
		if strings.Contains(strings.TrimPrefix(rel, includeDir), "/") {
			return nil, fmt.Errorf("unexpected nested path %q below %s", name, includeDir)
		}
		if h.Size > maxFileSize || total+h.Size > maxTotalSize || meta.Files >= maxFiles {
			return nil, fmt.Errorf("%q exceeds the size limits (file %d bytes, total %d, count %d)", name, h.Size, total, meta.Files)
		}
		out := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, err
		}
		n, err := io.Copy(f, io.LimitReader(tr, maxFileSize+1))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		if n != h.Size {
			return nil, fmt.Errorf("%q: short or oversize read (%d of %d bytes)", name, n, h.Size)
		}
		total += n
		meta.Files++
	}
	return meta, nil
}

var (
	defineRE   = regexp.MustCompile(`(?m)^#define\s+SDL_(MAJOR|MINOR|MICRO)_VERSION\s+(\d+)\s*$`)
	revisionRE = regexp.MustCompile(`(?m)^#define\s+SDL_REVISION\s+"(SDL-[^"]+)"\s*$`)
)

// validate checks a cache directory and returns its (completed) metadata. It
// does not touch the network.
func validate(dir string, tag Tag) (*Meta, error) {
	b, err := os.ReadFile(filepath.Join(dir, metaName))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", metaName, err)
	}
	if m.Tag != tag.Name {
		return nil, fmt.Errorf("%s records tag %q, want %q", metaName, m.Tag, tag.Name)
	}
	hdr := filepath.Join(dir, "include", "SDL3")
	ents, err := os.ReadDir(hdr)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, e := range ents {
		if e.Type().IsRegular() {
			n++
		}
	}
	if n != m.Files {
		return nil, fmt.Errorf("%d headers on disk, %s records %d", n, metaName, m.Files)
	}
	if n < minHeaders {
		return nil, fmt.Errorf("only %d headers; a release has at least %d", n, minHeaders)
	}
	ver, err := os.ReadFile(filepath.Join(hdr, "SDL_version.h"))
	if err != nil {
		return nil, err
	}
	vals := map[string]string{}
	for _, mm := range defineRE.FindAllStringSubmatch(string(ver), -1) {
		vals[mm[1]] = mm[2]
	}
	got := vals["MAJOR"] + "." + vals["MINOR"] + "." + vals["MICRO"]
	if got != tag.Version() {
		return nil, fmt.Errorf("SDL_version.h says %s, tag %s says %s", got, tag.Name, tag.Version())
	}
	m.Version = got
	rev, err := os.ReadFile(filepath.Join(hdr, "SDL_revision.h"))
	if err != nil {
		return nil, err
	}
	if mm := revisionRE.FindSubmatch(rev); mm != nil {
		m.Revision, m.RevisionSource = string(mm[1]), "header"
	} else {
		m.Revision, m.RevisionSource = "SDL-"+tag.Name, "tarball"
		if len(m.Commit) >= 9 {
			m.Revision = fmt.Sprintf("SDL-%s-0-g%s", tag.Name, m.Commit[:9])
		}
	}
	return &m, nil
}
