// Package update handles lazyaddons self-updates. It checks the
// GitHub Releases API for newer versions and, when the user opts
// in, downloads and replaces the running binary.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// RepoOwner is the GitHub owner of this project.
	RepoOwner = "pentsec"
	// RepoName is the GitHub repository name.
	RepoName = "lazyaddons"
)

// APIBase is the base URL for GitHub API. It is a var so
// tests can point it at a mock server.
var APIBase = "https://api.github.com"

// Release represents a GitHub release as returned by the API.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

// CheckResult is the outcome of a self-update check.
type CheckResult struct {
	// UpdateAvailable is true when there is a newer release on GitHub.
	UpdateAvailable bool
	// CurrentVersion is the running version.
	CurrentVersion string
	// LatestVersion is the latest release tag (without "v" prefix).
	LatestVersion string
	// LatestURL is the HTML URL of the latest release.
	LatestURL string
	// Err is non-nil when the check itself failed.
	Err error
}

// httpClient is swappable for testing.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// CompareVersions returns true if latest is newer than current.
// Both are semver strings (e.g. "1.8.3" or "v1.8.3").
// The comparison is lexicographic for simplicity.
func CompareVersions(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	return latest > current
}

// fetchLatestRelease queries the GitHub Releases API for the
// latest release of the lazyaddons repo.
func fetchLatestRelease() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", APIBase, RepoOwner, RepoName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "lazyaddons-update-check")
	if token := resolveGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no releases yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &rel, nil
}

// CheckLatest compares the running version against the latest
// GitHub release. Returns nil if the version is "dev" (dev build).
func CheckLatest(currentVersion string) *CheckResult {
	if currentVersion == "dev" {
		return nil
	}

	rel, err := fetchLatestRelease()
	result := &CheckResult{
		CurrentVersion: currentVersion,
	}
	if err != nil {
		result.Err = err
		return result
	}
	if rel == nil {
		// No releases on GitHub — nothing to compare.
		return nil
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	result.LatestVersion = latest
	result.LatestURL = rel.HTMLURL

	if CompareVersions(currentVersion, latest) {
		result.UpdateAvailable = true
	}
	return result
}
