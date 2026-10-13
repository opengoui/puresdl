// Package sdlsync pins the SDL release whose headers the bindings are
// generated from and keeps a local cache of those headers: it resolves
// release tags through the GitHub API, downloads the source tarball of a tag,
// extracts only include/SDL3 and checks that the result is consistent.
package sdlsync

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DefaultVersionFile is the file at the repository root that holds the
// pinned release tag.
const DefaultVersionFile = "SDL_VERSION"

var tagRE = regexp.MustCompile(`^(?:(release|prerelease|preview)-)?v?(\d+)\.(\d+)\.(\d+)(?:[-.]?([0-9A-Za-z.\-]+))?$`)

// Tag is a parsed release tag.
type Tag struct {
	Major, Minor, Micro int
	// Prerelease is true for prerelease-/preview- tags and for versions with
	// a suffix such as "3.5.0-rc1".
	Prerelease bool
	// Name is the canonical git tag, for example "release-3.4.18".
	Name string
}

// Version returns "major.minor.micro".
func (t Tag) Version() string { return fmt.Sprintf("%d.%d.%d", t.Major, t.Minor, t.Micro) }

// ParseTag normalises "3.4.18", "v3.4.18" and "release-3.4.18" to the
// canonical tag "release-3.4.18". Prerelease and preview tags
// ("prerelease-3.1.2", "preview-3.1.3", "3.5.0-rc1") are rejected unless
// allowPrerelease is set.
func ParseTag(in string, allowPrerelease bool) (Tag, error) {
	in = strings.TrimSpace(in)
	m := tagRE.FindStringSubmatch(in)
	if m == nil {
		return Tag{}, fmt.Errorf("sdlsync: %q is not a release version (want 3.4.18, v3.4.18 or release-3.4.18)", in)
	}
	var t Tag
	fmt.Sscanf(m[2]+" "+m[3]+" "+m[4], "%d %d %d", &t.Major, &t.Minor, &t.Micro)
	t.Prerelease = (m[1] != "" && m[1] != "release") || m[5] != ""
	switch {
	case m[1] == "prerelease" || m[1] == "preview":
		t.Name = m[1] + "-" + t.Version()
	default:
		t.Name = "release-" + t.Version()
	}
	if m[5] != "" {
		t.Name += "-" + m[5]
	}
	if t.Prerelease && !allowPrerelease {
		return Tag{}, fmt.Errorf("sdlsync: %q is a prerelease tag; pass -allow-prerelease to use it", in)
	}
	return t, nil
}

// ReadVersionFile returns the tag stored in path (a single line).
func ReadVersionFile(path string, allowPrerelease bool) (Tag, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Tag{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" {
		return Tag{}, fmt.Errorf("sdlsync: %s must hold exactly one line with the release tag", path)
	}
	return ParseTag(lines[0], allowPrerelease)
}

// WriteVersionFile stores tag in path, reporting whether the content changed.
func WriteVersionFile(path string, tag Tag) (changed bool, err error) {
	want := tag.Name + "\n"
	if old, err := os.ReadFile(path); err == nil && string(old) == want {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(want), 0o644)
}
