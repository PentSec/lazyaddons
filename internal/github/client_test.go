package github

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsGitHubHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/u/r", true},
		{"https://www.github.com/u/r", true},
		{"HTTPS://GITHUB.COM/u/r", true},
		{"https://gitlab.com/u/r", false},
		{"https://bitbucket.org/u/r", false},
		{"not-a-url", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			if got := IsGitHubHost(tc.url); got != tc.want {
				t.Errorf("IsGitHubHost(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestParseOwnerRepo(t *testing.T) {
	t.Parallel()
	owner, repo, err := ParseOwnerRepo("https://github.com/jeff/Atlas.git")
	if err != nil {
		t.Fatalf("ParseOwnerRepo: %v", err)
	}
	if owner != "jeff" {
		t.Errorf("owner = %q, want jeff", owner)
	}
	if repo != "Atlas" {
		t.Errorf("repo = %q, want Atlas", repo)
	}
}

func TestParseOwnerRepo_NonGitHub(t *testing.T) {
	t.Parallel()
	_, _, err := ParseOwnerRepo("https://gitlab.com/u/r")
	if err == nil {
		t.Errorf("ParseOwnerRepo(gitlab) = nil, want error")
	}
}

func TestListReleases_SortBySemver(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repos/u/r/releases") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{"tag_name":"v1.0.0","draft":false,"prerelease":false},
			{"tag_name":"v2.0.0","draft":false,"prerelease":false},
			{"tag_name":"v1.5.0","draft":false,"prerelease":false}
		]`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	got, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].TagName != "v2.0.0" {
		t.Errorf("got[0] = %q, want v2.0.0", got[0].TagName)
	}
	if got[1].TagName != "v1.5.0" {
		t.Errorf("got[1] = %q, want v1.5.0", got[1].TagName)
	}
	if got[2].TagName != "v1.0.0" {
		t.Errorf("got[2] = %q, want v1.0.0", got[2].TagName)
	}
}

func TestListReleases_SkipsDrafts(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{"tag_name":"v1.0.0","draft":true},
			{"tag_name":"v0.9.0","draft":false}
		]`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	got, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (drafts must be filtered)", len(got))
	}
	if got[0].TagName != "v0.9.0" {
		t.Errorf("got[0] = %q, want v0.9.0", got[0].TagName)
	}
}

func TestListReleases_EmptyArray(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	got, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestListReleases_NotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.ListReleases("u", "r")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ListReleases(notfound) = %v, want ErrNotFound", err)
	}
}

func TestListReleases_RateLimitFallback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "rate limit", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.ListReleases("u", "r")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("ListReleases(403 ratelimit) = %v, want ErrRateLimited", err)
	}
}

func TestListReleases_ForbiddenNotRateLimit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "5000")
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.ListReleases("u", "r")
	if err == nil || errors.Is(err, ErrRateLimited) {
		t.Errorf("ListReleases(403 non-rate) = %v, want generic error", err)
	}
}

func TestLatestRelease_SkipsPrerelease(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{"tag_name":"v2.0.0","prerelease":true},
			{"tag_name":"v1.5.0","prerelease":false}
		]`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	got, err := c.LatestRelease("u", "r")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got == nil {
		t.Fatalf("LatestRelease = nil, want a release")
	}
	if got.TagName != "v1.5.0" {
		t.Errorf("LatestRelease = %q, want v1.5.0 (prerelease skipped)", got.TagName)
	}
}

func TestLatestRelease_EmptyList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	got, err := c.LatestRelease("u", "r")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got != nil {
		t.Errorf("LatestRelease(empty) = %+v, want nil", got)
	}
}

func TestDownloadAsset_Success(t *testing.T) {
	t.Parallel()
	want := []byte("zip contents")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	var got bytes.Buffer
	n, err := c.DownloadAsset(srv.URL+"/asset.zip", &got)
	if err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if int(n) != len(want) {
		t.Errorf("DownloadAsset wrote %d bytes, want %d", n, len(want))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("DownloadAsset body = %q, want %q", got.String(), want)
	}
}

func TestDownloadAsset_RateLimited(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.DownloadAsset(srv.URL+"/asset.zip", io.Discard)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("DownloadAsset(403) = %v, want ErrRateLimited", err)
	}
}

func TestParseSemver_HandlesVPrefix(t *testing.T) {
	t.Parallel()
	v, ok := parseSemver("v1.2.3")
	if !ok {
		t.Fatal("parseSemver(v1.2.3) = !ok")
	}
	if v.major != 1 || v.minor != 2 || v.patch != 3 {
		t.Errorf("parseSemver = %+v, want {1,2,3}", v)
	}
}

func TestParseSemver_HandlesNoVPrefix(t *testing.T) {
	t.Parallel()
	v, ok := parseSemver("2.0.1")
	if !ok {
		t.Fatal("parseSemver(2.0.1) = !ok")
	}
	if v.major != 2 {
		t.Errorf("major = %d, want 2", v.major)
	}
}

func TestParseSemver_RejectsMalformed(t *testing.T) {
	t.Parallel()
	_, ok := parseSemver("not-a-version")
	if ok {
		t.Errorf("parseSemver(not-a-version) = ok, want !ok")
	}
}
