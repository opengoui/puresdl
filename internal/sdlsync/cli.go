package sdlsync

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenConfig describes one run of the generation pipeline; it mirrors
// gensdl.Config so that this package does not depend on the generator (the
// generator's tests depend on this package for the header cache).
type GenConfig struct {
	Include, Out, HandDir, Skipped string
	Tag, Revision                  string
	Stdout, Stderr                 io.Writer
}

// Generate is the pipeline run by -generate and -check. cmd/sdlsync sets it
// to gensdl.Run; tests replace it.
var Generate func(GenConfig) error

// Main implements cmd/sdlsync. Progress goes to stderr; stdout carries only
// the output of -list and -print-include so that it can be used from shell
// scripts. It returns the process exit code.
func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if err := run(ctx, args, getenv, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, flag.ErrHelp) {
			return 2
		}
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sdlsync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		version = fs.String("version", "", "release to use: 3.4.18, v3.4.18 or release-3.4.18 (default: contents of SDL_VERSION)")
		latest  = fs.Bool("latest", false, "resolve the latest official release through the GitHub API")
		write   = fs.Bool("write", false, "update SDL_VERSION when the resolved tag differs")
		list    = fs.Bool("list", false, "print recent non-prerelease release tags and exit")
		listN   = fs.Int("n", 10, "number of tags printed by -list")
		cache   = fs.String("cache-dir", "", "header cache (default os.UserCacheDir()/puresdl/sdl)")
		gen     = fs.Bool("generate", false, "regenerate sdl/*_gen.go and docs/skipped.txt after syncing")
		check   = fs.Bool("check", false, "regenerate into a temp dir and fail if the checked-in generated files differ")
		offline = fs.Bool("offline", false, "use the cache only, no network")
		allow   = fs.Bool("allow-prerelease", false, "accept prerelease/preview tags")
		printIn = fs.Bool("print-include", false, "print the include directory (the value for SDL_INCLUDE) on stdout")
		root    = fs.String("root", ".", "repository root")
		apiURL  = fs.String("github-api-url", getenv("SDLSYNC_GITHUB_API_URL"), "GitHub API root (env SDLSYNC_GITHUB_API_URL)")
		dlURL   = fs.String("download-url", getenv("SDLSYNC_DOWNLOAD_URL"), "directory serving <tag>.tar.gz (env SDLSYNC_DOWNLOAD_URL)")
		timeout = fs.Duration("timeout", 5*time.Minute, "overall network timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version != "" && *latest {
		return errors.New("sdlsync: -version and -latest are mutually exclusive")
	}
	if *offline && (*latest || *list) {
		return errors.New("sdlsync: -offline cannot be combined with -latest or -list")
	}
	if *gen && *check {
		return errors.New("sdlsync: -generate and -check are mutually exclusive")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	client := &Client{APIURL: *apiURL, DownloadURL: *dlURL, Token: getenv("GITHUB_TOKEN")}

	if *list {
		tags, err := client.List(ctx, *listN)
		if err != nil {
			return err
		}
		for _, t := range tags {
			fmt.Fprintln(stdout, t.Name)
		}
		return nil
	}

	versionFile := filepath.Join(*root, DefaultVersionFile)
	var tag Tag
	var err error
	switch {
	case *latest:
		if tag, err = client.Latest(ctx); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "latest release: %s\n", tag.Name)
	case *version != "":
		tag, err = ParseTag(*version, *allow)
	default:
		tag, err = ReadVersionFile(versionFile, *allow)
	}
	if err != nil {
		return err
	}

	cacheDir := *cache
	if cacheDir == "" {
		if cacheDir, err = DefaultCacheDir(); err != nil {
			return err
		}
	}
	s := &Syncer{Client: client, CacheDir: cacheDir, Offline: *offline}
	res, err := s.Sync(ctx, tag)
	if err != nil {
		return err
	}
	how := "cached"
	if res.Downloaded {
		how = "downloaded"
	}
	fmt.Fprintf(stderr, "%s: %s headers (%d files, sha256 %s)\n  version %s, SDL_REVISION %q (%s)\n  include %s\n",
		tag.Name, how, res.Files, res.SHA256, res.Version, res.Revision, res.RevisionSource, res.Include)

	sdlDir := filepath.Join(*root, "sdl")
	skipped := filepath.Join(*root, "docs", "skipped.txt")
	cfg := GenConfig{Include: res.Include, HandDir: sdlDir, Tag: tag.Name, Revision: res.Revision, Stdout: stderr, Stderr: stderr}
	if (*gen || *check) && Generate == nil {
		return errors.New("sdlsync: no generator configured")
	}
	switch {
	case *gen:
		cfg.Out, cfg.Skipped = sdlDir, skipped
		if err := Generate(cfg); err != nil {
			return err
		}
	case *check:
		tmp, err := os.MkdirTemp("", "sdlsync-check-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		cfg.Out, cfg.Skipped = tmp, filepath.Join(tmp, "skipped.txt")
		if err := Generate(cfg); err != nil {
			return err
		}
		diffs, err := Compare(tmp, cfg.Skipped, sdlDir, skipped)
		if err != nil {
			return err
		}
		if len(diffs) > 0 {
			return fmt.Errorf("sdlsync: generated code is out of date for %s (run `go generate ./sdl`):\n  %s", tag.Name, strings.Join(diffs, "\n  "))
		}
		fmt.Fprintf(stderr, "generated code is up to date for %s\n", tag.Name)
	}
	if *write {
		pinned, perr := ReadVersionFile(versionFile, true)
		changed, err := WriteVersionFile(versionFile, tag)
		if err != nil {
			return err
		}
		if changed {
			from := "(none)"
			if perr == nil {
				from = pinned.Name
			}
			fmt.Fprintf(stderr, "SDL_VERSION: %s -> %s\n", from, tag.Name)
		} else {
			fmt.Fprintf(stderr, "SDL_VERSION already %s\n", tag.Name)
		}
	}

	if *printIn {
		fmt.Fprintln(stdout, res.Include)
	}
	return nil
}
