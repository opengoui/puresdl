package sdlsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	// AllowTokenAnyHost sends Token to a custom https APIURL as well. By
	// default the token only goes to api.github.com or a loopback server.
	AllowTokenAnyHost bool
}

func (c *Client) apiURL() string {
	if c.APIURL != "" {
		return strings.TrimRight(c.APIURL, "/")
	}
	return DefaultAPIURL
}

// tokenAllowed reports whether the token may be sent to the API URL: only
// https, or plain http to a loopback host. Any other host (including a custom
// https one) gets the request without credentials unless AllowTokenAnyHost.
func (c *Client) tokenAllowed() bool {
	u, err := url.Parse(c.apiURL())
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return c.AllowTokenAnyHost || u.Hostname() == "api.github.com"
	case "http":
		ip := net.ParseIP(u.Hostname())
		return u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	}
	return false
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
	if c.Token != "" && c.tokenAllowed() {
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
		snippet = strings.ToValidUTF8(snippet[:200], "")
	}
	return fmt.Errorf("sdlsync: GitHub API: %s: %q", resp.Status, snippet)
}

// Latest returns the tag of the latest official (non-prerelease) release.
func (c *Client) Latest(ctx context.Context) (Tag, error) {
	var r release
	if err := c.getJSON(ctx, repoPath+"/releases/latest", &r); err != nil {
		return Tag{}, err
	}
	t, err := ParseTag(r.TagName, false)
	if err == nil {
		return t, nil
	}
	// The repository also publishes other majors; fall back to the newest
	// SDL3 release in the list.
	if tags, lerr := c.List(ctx, 1); lerr == nil && len(tags) > 0 {
		return tags[0], nil
	}
	return Tag{}, fmt.Errorf("sdlsync: latest release %q is not a plain SDL3 release tag: %w", r.TagName, err)
}

// List returns up to n recent non-prerelease SDL3 release tags, newest first.
// It follows the API pagination; n <= 0 means one page.
func (c *Client) List(ctx context.Context, n int) ([]Tag, error) {
	var out []Tag
	for page := 1; page <= 10; page++ {
		var rs []release
		if err := c.getJSON(ctx, fmt.Sprintf("%s/releases?per_page=100&page=%d", repoPath, page), &rs); err != nil {
			return nil, err
		}
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
				return out, nil
			}
		}
		if n <= 0 || len(rs) < 100 {
			break
		}
	}
	return out, nil
}
