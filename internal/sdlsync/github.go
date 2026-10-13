package sdlsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAPIURL is the GitHub REST API root.
	DefaultAPIURL = "https://api.github.com"
	// DefaultDownloadURL is the directory that serves <tag>.tar.gz archives.
	DefaultDownloadURL = "https://github.com/libsdl-org/SDL/archive/refs/tags"
	// UserAgent is sent with every request (GitHub rejects requests without one).
	UserAgent = "puresdl-sdlsync (+https://github.com/opengoui/puresdl)"

	repoPath = "/repos/libsdl-org/SDL"
)

// Client talks to the GitHub API and the tarball host.
type Client struct {
	APIURL      string       // default DefaultAPIURL
	DownloadURL string       // default DefaultDownloadURL
	Token       string       // optional GitHub token, sent to the API only
	HTTP        *http.Client // default: 2 minute timeout
}

func (c *Client) apiURL() string {
	if c.APIURL != "" {
		return strings.TrimRight(c.APIURL, "/")
	}
	return DefaultAPIURL
}

func (c *Client) downloadURL() string {
	if c.DownloadURL != "" {
		return strings.TrimRight(c.DownloadURL, "/")
	}
	return DefaultDownloadURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("sdlsync: GitHub API: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return apiError(resp, body, c.Token != "")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("sdlsync: GitHub API: decoding %s: %w", path, err)
	}
	return nil
}

func apiError(resp *http.Response, body []byte, haveToken bool) error {
	limited := resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && (resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(body)), "rate limit")))
	if limited {
		msg := "sdlsync: GitHub API rate limit exceeded"
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			msg += " (resets at " + time.Unix(reset, 0).UTC().Format(time.RFC3339) + ")"
		}
		if !haveToken {
			msg += "; set GITHUB_TOKEN to raise the limit"
		}
		return fmt.Errorf("%s", msg)
	}
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	return fmt.Errorf("sdlsync: GitHub API: %s: %s", resp.Status, snippet)
}

// Latest returns the tag of the latest official (non-prerelease) release.
func (c *Client) Latest(ctx context.Context) (Tag, error) {
	var r release
	if err := c.getJSON(ctx, repoPath+"/releases/latest", &r); err != nil {
		return Tag{}, err
	}
	t, err := ParseTag(r.TagName, false)
	if err != nil {
		return Tag{}, fmt.Errorf("sdlsync: latest release %q is not a plain release tag: %w", r.TagName, err)
	}
	return t, nil
}

// List returns up to n recent non-prerelease release tags, newest first.
func (c *Client) List(ctx context.Context, n int) ([]Tag, error) {
	var rs []release
	if err := c.getJSON(ctx, fmt.Sprintf("%s/releases?per_page=%d", repoPath, 100), &rs); err != nil {
		return nil, err
	}
	var out []Tag
	for _, r := range rs {
		if r.Prerelease || r.Draft {
			continue
		}
		t, err := ParseTag(r.TagName, false)
		if err != nil {
			continue
		}
		out = append(out, t)
		if n > 0 && len(out) == n {
			break
		}
	}
	return out, nil
}
