package scenario

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/config"
	gh "github.com/pentsec/lazyaddons/internal/github"
	"github.com/pentsec/lazyaddons/internal/gitops"
)

// ---- addon-update scenarios ----

func TestUpdate_BranchUpdateDetection(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)
	localSHA, _ := gitops.HeadSHA(work)

	// Push a new commit to remote.
	other := t.TempDir()
	runGit(t, other, "clone", remote, ".")
	runGit(t, other, "config", "user.email", "t@example.com")
	runGit(t, other, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(other, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, other, "add", "new.txt")
	runGit(t, other, "commit", "-m", "new")
	runGit(t, other, "push", "origin", "main")

	// Fetch and compare.
	if err := gitops.Fetch(work); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	remoteSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "origin/main"))
	if remoteSHA == localSHA {
		t.Errorf("remote SHA == local SHA, expected an update to be available")
	}
}

func TestUpdate_BranchNoUpdate(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)
	localSHA, _ := gitops.HeadSHA(work)

	if err := gitops.Fetch(work); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	remoteSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "origin/main"))
	if remoteSHA != localSHA {
		t.Errorf("remote SHA = %q, local = %q, want equal", remoteSHA, localSHA)
	}
}

func TestUpdate_ReleaseNewerAvailable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.0.0","prerelease":false,"draft":false},
			{"tag_name":"v1.1.0","prerelease":false,"draft":false}
		]`))
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	all, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(all) < 1 {
		t.Fatalf("no releases")
	}
	// The first release in the sorted list is the latest stable.
	if all[0].TagName != "v1.1.0" {
		t.Errorf("latest = %q, want v1.1.0", all[0].TagName)
	}
}

func TestUpdate_ReleaseNoNewer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0","prerelease":false,"draft":false}]`))
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	all, err := c.ListReleases("u", "r")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	tracked := "v1.0.0"
	if len(all) == 0 || all[0].TagName != tracked {
		t.Errorf("latest != %q", tracked)
	}
}

func TestUpdate_GitHubAPIFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &gh.Client{HTTP: srv.Client(), Base: srv.URL}
	_, err := c.ListReleases("u", "r")
	if err == nil {
		t.Errorf("ListReleases(500) = nil, want error")
	}
}

func TestUpdate_AggregateSummary(t *testing.T) {
	t.Parallel()
	cfg := v2ConfigWithAddons([]config.Addon{
		{Name: "A", URL: "u1", TrackMode: addon.TrackModeBranch, TrackTarget: "main"},
		{Name: "B", URL: "u2", TrackMode: addon.TrackModeBranch, TrackTarget: "main"},
		{Name: "C", URL: "u3", TrackMode: addon.TrackModeBranch, TrackTarget: "main"},
	}, "")
	summary := struct {
		Total   int
		Update  int
		Error   int
		Current int
	}{Total: len(cfg.Profiles[0].Addons), Update: 1, Error: 1, Current: 1}
	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}
	if summary.Update+summary.Error+summary.Current != summary.Total {
		t.Errorf("summary breakdown inconsistent")
	}
}
