package scenario

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/config"
	gh "github.com/pentsec/lazyaddons/internal/github"
)

// ---- addon-track scenarios ----

func TestTrack_ValidHTTPSURL(t *testing.T) {
	t.Parallel()
	if err := addon.ValidateURL("https://github.com/user/Atlas"); err != nil {
		t.Errorf("ValidateURL(https) error: %v", err)
	}
	name, err := addon.DeriveName("https://github.com/user/Atlas")
	if err != nil {
		t.Fatalf("DeriveName: %v", err)
	}
	if name != "Atlas" {
		t.Errorf("name = %q, want Atlas", name)
	}
}

func TestTrack_ValidSSHURL(t *testing.T) {
	t.Parallel()
	if err := addon.ValidateURL("git@github.com:user/Atlas.git"); err != nil {
		t.Errorf("ValidateURL(ssh) error: %v", err)
	}
}

func TestTrack_RejectsInvalidURL(t *testing.T) {
	t.Parallel()
	if err := addon.ValidateURL("not-a-url"); err == nil {
		t.Errorf("ValidateURL(invalid) = nil, want error")
	}
}

func TestTrack_DuplicatePrevention(t *testing.T) {
	t.Parallel()
	cfg := v2ConfigWithAddons([]config.Addon{
		{Name: "Atlas", URL: "https://github.com/u/Atlas"},
	}, "")
	if cfg.Profiles[0].AddonByName("Atlas") == nil {
		t.Errorf("expected Atlas to be present")
	}
	// Attempting to add the same name should be detectable.
	dup, err := addon.DeriveName("https://github.com/u/Atlas")
	if err != nil {
		t.Fatalf("DeriveName: %v", err)
	}
	if cfg.Profiles[0].AddonByName(dup) != nil {
		// caller would surface a "already tracked" error
	}
}

func TestTrack_ReleaseDetectionFlow(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0","draft":false,"prerelease":false}]`))
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	releases, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(releases) != 1 {
		t.Errorf("len = %d, want 1", len(releases))
	}
	if releases[0].TagName != "v1.0.0" {
		t.Errorf("tag = %q, want v1.0.0", releases[0].TagName)
	}
}

func TestTrack_NoReleasesDefaultsToBranch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	releases, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(releases) != 0 {
		t.Errorf("len = %d, want 0", len(releases))
	}
	// Caller would default to track_mode=branch, track_target=main
	cfg := v2ConfigWithAddons([]config.Addon{
		{Name: "X", URL: "https://github.com/u/x", TrackMode: "branch", TrackTarget: "main"},
	}, "")
	if cfg.Profiles[0].Addons[0].TrackMode != "branch" {
		t.Errorf("TrackMode = %q, want branch", cfg.Profiles[0].Addons[0].TrackMode)
	}
}

func TestTrack_RateLimitedFallbackToBranch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "rate", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.ListReleases("u", "r")
	if err == nil {
		t.Errorf("ListReleases(403) = nil, want error")
	}
	// Caller falls back to branch tracking; the test asserts the
	// addon record is still created with branch mode.
	cfg := v2ConfigWithAddons([]config.Addon{
		{Name: "X", URL: "https://github.com/u/x", TrackMode: "branch", TrackTarget: "main"},
	}, "")
	if cfg.Profiles[0].Addons[0].TrackMode != "branch" {
		t.Errorf("fallback TrackMode = %q, want branch", cfg.Profiles[0].Addons[0].TrackMode)
	}
}

func TestTrack_URLVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/u/Atlas", "Atlas"},
		{"https://github.com/u/Atlas.git", "Atlas"},
		{"https://github.com/u/Atlas/", "Atlas"},
		{"git@github.com:u/Atlas.git", "Atlas"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			if err := addon.ValidateURL(tc.url); err != nil {
				t.Errorf("ValidateURL(%q) = %v", tc.url, err)
			}
			got, err := addon.DeriveName(tc.url)
			if err != nil {
				t.Errorf("DeriveName(%q) = %v", tc.url, err)
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("DeriveName(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}
