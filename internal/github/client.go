// Package github provides a tiny GitHub Releases client. It is
// intentionally narrow: just enough to detect releases, list their
// tags sorted by semver, and download assets. Anything else (auth,
// pagination, GraphQL) is out of scope.
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// APIBase is the GitHub REST base URL. Exported so tests can
// override it.
var APIBase = "https://api.github.com"

// ErrRateLimited is returned when GitHub responds with 403 and
// `X-RateLimit-Remaining` is 0. The caller can treat this as
// "no release info available; fall back to branch tracking".
var ErrRateLimited = errors.New("github: rate limit reached")

// ErrNotFound is returned when a repository or release does not
// exist on the server.
var ErrNotFound = errors.New("github: not found")

// Release is a trimmed GitHub release payload. We only model the
// fields we need; the rest of the API response is ignored.
type Release struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets     []Asset   `json:"assets"`
	HTMLURL    string    `json:"html_url"`
}

// Asset describes a downloadable file attached to a release.
type Asset struct {
	Name        string `json:"name"`
	BrowserURL  string `json:"browser_download_url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

// Client is a thin wrapper around net/http with a configurable
// base URL. It is safe for concurrent use because http.Client is.
type Client struct {
	HTTP *http.Client
	Base string
}

// New returns a Client with sensible defaults. The timeout is
// deliberately short to keep the TUI responsive.
func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 15 * time.Second},
		Base: APIBase,
	}
}

// IsGitHubHost reports whether a URL points at a GitHub-like host.
// The check is case-insensitive and accepts both github.com and
// www.github.com.
func IsGitHubHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return host == "github.com" || host == "www.github.com"
}

// ParseOwnerRepo extracts "owner/repo" from a GitHub URL. Returns
// an error if the URL does not look like a GitHub repo URL.
func ParseOwnerRepo(rawURL string) (owner, repo string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("github: parse %s: %w", rawURL, err)
	}
	host := strings.ToLower(u.Host)
	if host != "github.com" && host != "www.github.com" {
		return "", "", fmt.Errorf("github: %s is not a GitHub URL", rawURL)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("github: %s has no owner/repo", rawURL)
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("github: empty owner or repo in %s", rawURL)
	}
	return owner, repo, nil
}

// ListReleases returns all non-draft releases for a repo, sorted
// newest-first by semver. A 403 with `X-RateLimit-Remaining: 0`
// returns ErrRateLimited. A 404 returns ErrNotFound.
//
// Drafts are skipped. Pre-releases are included so users on the
// latest tag picker can see beta versions.
func (c *Client) ListReleases(owner, repo string) ([]Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=50", c.Base, owner, repo)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: list releases: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, ErrRateLimited
		}
		return nil, fmt.Errorf("github: forbidden: %s", resp.Status)
	default:
		return nil, fmt.Errorf("github: list releases: status %d", resp.StatusCode)
	}

	var raw []Release
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decode releases: %w", err)
	}

	// Strip drafts. We do not filter prereleases because the UI
	// lets the user opt in/out via a checkbox.
	out := raw[:0]
	for _, r := range raw {
		if r.Draft {
			continue
		}
		out = append(out, r)
	}
	sortReleasesBySemver(out)
	return out, nil
}

// LatestRelease returns the newest non-draft, non-prerelease
// release, or nil if none exists.
func (c *Client) LatestRelease(owner, repo string) (*Release, error) {
	all, err := c.ListReleases(owner, repo)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if !all[i].Prerelease {
			return &all[i], nil
		}
	}
	if len(all) > 0 {
		return &all[0], nil
	}
	return nil, nil
}

// ReleaseForTag returns the release whose TagName matches tag,
// or nil if no such release exists.
func (c *Client) ReleaseForTag(owner, repo, tag string) (*Release, error) {
	all, err := c.ListReleases(owner, repo)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].TagName == tag {
			return &all[i], nil
		}
	}
	return nil, nil
}

// FindZipAsset returns the first .zip asset in the release, or nil.
func (r *Release) FindZipAsset() *Asset {
	for i := range r.Assets {
		if strings.HasSuffix(strings.ToLower(r.Assets[i].Name), ".zip") {
			return &r.Assets[i]
		}
	}
	return nil
}

// DefaultBranch returns the default branch name of a GitHub
// repository (e.g. "main", "master", "develop"). Returns "" on
// failure so callers can fall back to git-based detection.
func (c *Client) DefaultBranch(owner, repo string) string {
	endpoint := fmt.Sprintf("%s/repos/%s/%s", c.Base, owner, repo)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ""
	}
	return info.DefaultBranch
}

// DownloadAsset streams the asset at assetURL into dest and
// returns the number of bytes written.
func (c *Client) DownloadAsset(assetURL string, dest io.Writer) (int64, error) {
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("github: download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return 0, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("github: download asset: status %d", resp.StatusCode)
	}
	return io.Copy(dest, resp.Body)
}

// sortReleasesBySemver sorts the slice in place, newest semver
// first. Releases whose tag does not look like a semver fall back
// to lexicographic order so the list is still deterministic.
func sortReleasesBySemver(rs []Release) {
	sort.SliceStable(rs, func(i, j int) bool {
		ai, aok := parseSemver(rs[i].TagName)
		bj, bok := parseSemver(rs[j].TagName)
		if aok && bok {
			if ai.major != bj.major {
				return ai.major > bj.major
			}
			if ai.minor != bj.minor {
				return ai.minor > bj.minor
			}
			if ai.patch != bj.patch {
				return ai.patch > bj.patch
			}
			return rs[i].TagName > rs[j].TagName
		}
		return rs[i].TagName > rs[j].TagName
	})
}

type semver struct {
	major, minor, patch int
}

// parseSemver extracts major.minor.patch from a tag. The leading
// "v" is optional. Returns ok=false if the tag does not match the
// expected shape.
func parseSemver(tag string) (semver, bool) {
	t := strings.TrimPrefix(tag, "v")
	parts := strings.SplitN(t, ".", 3)
	if len(parts) != 3 {
		return semver{}, false
	}
	major, ok1 := atoi(parts[0])
	minor, ok2 := atoi(parts[1])
	patch, ok3 := atoi(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return semver{}, false
	}
	return semver{major, minor, patch}, true
}

func atoi(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
