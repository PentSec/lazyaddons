package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompareVersions_newer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "2.0.0", true},
		{"1.2.3", "1.2.4", true},
		{"1.1.9", "1.2.0", true},
		{"0.9.0", "1.0.0", true},
		// v-prefix stripped automatically
		{"v1.0.0", "v2.0.0", true},
		{"v1.0.0", "2.0.0", true},
		{"1.0.0", "v2.0.0", true},
	}
	for _, tc := range tests {
		got := CompareVersions(tc.current, tc.latest)
		if got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %v, want %v",
				tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestCompareVersions_sameOrOlder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"2.0.0", "1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"v2.0.0", "v1.0.0", false},
	}
	for _, tc := range tests {
		got := CompareVersions(tc.current, tc.latest)
		if got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %v, want %v",
				tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestCheckLatest_devBuild(t *testing.T) {
	t.Parallel()
	result := CheckLatest("dev")
	if result != nil {
		t.Errorf("CheckLatest(dev) = %+v, want nil", result)
	}
}

func TestCheckLatest_updateAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			TagName:     "v2.0.0",
			Name:        "Release 2.0.0",
			PublishedAt: time.Now(),
			HTMLURL:     "https://github.com/pentsec/lazyaddons/releases/v2.0.0",
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer server.Close()

	// Override APIBase to point at the test server.
	origAPI := APIBase
	APIBase = server.URL
	defer func() { APIBase = origAPI }()

	result := CheckLatest("1.0.0")
	if result == nil {
		t.Fatal("CheckLatest returned nil")
	}
	if !result.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true")
	}
	if result.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want 2.0.0", result.LatestVersion)
	}
	if result.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want 1.0.0", result.CurrentVersion)
	}
}

func TestCheckLatest_upToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			TagName:     "v1.0.0",
			Name:        "Release 1.0.0",
			PublishedAt: time.Now(),
			HTMLURL:     "https://github.com/pentsec/lazyaddons/releases/v1.0.0",
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer server.Close()

	origAPI := APIBase
	APIBase = server.URL
	defer func() { APIBase = origAPI }()

	result := CheckLatest("1.0.0")
	if result == nil {
		t.Fatal("CheckLatest returned nil")
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false (same version)")
	}
}

func TestCheckLatest_noReleasesYet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	origAPI := APIBase
	APIBase = server.URL
	defer func() { APIBase = origAPI }()

	result := CheckLatest("1.0.0")
	if result != nil {
		t.Errorf("CheckLatest with 404 = %+v, want nil (no releases)", result)
	}
}

func TestCheckLatest_serverError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origAPI := APIBase
	APIBase = server.URL
	defer func() { APIBase = origAPI }()

	result := CheckLatest("1.0.0")
	if result == nil {
		t.Fatal("CheckLatest returned nil")
	}
	if result.Err == nil {
		t.Error("Expected error from 500 response, got nil")
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable = true with server error, want false")
	}
}
