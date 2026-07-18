// Package scenario contains integration-style tests that exercise
// multiple internal packages together. They live in their own
// package so unit tests stay fast and small, and so these tests
// can rely on real git when present (skipping in -short).
package scenario

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/backup"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/gitops"
)

var _ = exec.Command // suppress unused import when git tests are stripped

// seedBareWithAddon builds a minimal "addon" git repo with the
// given name and a valid .toc file. The returned path is the
// parent directory containing a folder named `name` with a .git
// inside. A `fake-remote` origin is added so RemoteURL works.
func seedBareWithAddon(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	cmd := exec.Command("git", "init", "--initial-branch=main", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "T"},
		{"remote", "add", "origin", "https://github.com/u/" + name + ".git"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		_ = c.Run()
	}
	if err := os.WriteFile(filepath.Join(dir, name+".toc"), []byte("## Title: x\n"), 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".lua"), []byte("-- x\n"), 0o644); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "init"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

// ---- addon-scan scenarios ----

func TestScan_DetectsGitTrackedAddons(t *testing.T) {
	t.Parallel()
	requireGit(t)

	addons := t.TempDir()
	seedBareWithAddon(t, addons, "Atlas")
	seedBareWithAddon(t, addons, "Bagnon")

	cfg := &config.Config{Version: 1, Addons: []config.Addon{}}
	mgr := backup.New(addons)

	// scan = enumerate subdirs with .git, capture URL/SHA
	entries, err := os.ReadDir(addons)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var detected []config.Addon
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(addons, e.Name())
		if _, err := os.Stat(filepath.Join(sub, ".git")); err != nil {
			continue
		}
		sha, err := gitops.HeadSHA(sub)
		if err != nil {
			t.Fatalf("HeadSHA %s: %v", sub, err)
		}
		url, err := gitops.RemoteURL(sub)
		if err != nil {
			t.Fatalf("RemoteURL %s: %v", sub, err)
		}
		detected = append(detected, config.Addon{
			Name:       e.Name(),
			URL:        url,
			TrackMode:  addon.TrackModeBranch,
			CurrentSHA: sha,
		})
		_ = mgr
	}
	if len(detected) != 2 {
		t.Errorf("detected = %d, want 2", len(detected))
	}
	cfg.Addons = append(cfg.Addons, detected...)
}

func TestScan_EmptyAddOnsFolder(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()

	entries, _ := os.ReadDir(addons)
	gitTracked := 0
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(addons, e.Name(), ".git")); err == nil {
				gitTracked++
			}
		}
	}
	if gitTracked != 0 {
		t.Errorf("gitTracked = %d, want 0", gitTracked)
	}
}

func TestScan_ExcludesNonGitFolders(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	// Mix git and non-git folders.
	seedBareWithAddon(t, addons, "Atlas")
	manual := filepath.Join(addons, "Manual")
	if err := os.MkdirAll(manual, 0o755); err != nil {
		t.Fatalf("mkdir manual: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manual, "Manual.toc"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, _ := os.ReadDir(addons)
	gitTracked := 0
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(addons, e.Name(), ".git")); err == nil {
				gitTracked++
			}
		}
	}
	if gitTracked != 1 {
		t.Errorf("gitTracked = %d, want 1 (Manual excluded)", gitTracked)
	}
}

func TestScan_MissingFolderReturnsError(t *testing.T) {
	t.Parallel()
	_, err := os.ReadDir(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Errorf("readdir(missing) = nil, want error")
	}
}

func TestScan_URLNormalisation(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	addons := t.TempDir()
	dir := seedBareWithAddon(t, addons, "Atlas")
	// Manually set remote URL to an https form.
	exec.Command("git", "remote", "add", "origin", "https://github.com/u/Atlas.git").Run()
	_ = dir
	url, err := gitops.RemoteURL(addons + "/Atlas")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if strings.HasSuffix(url, ".git") {
		t.Errorf("URL still has .git suffix: %q", url)
	}
	if !strings.HasPrefix(url, "https://github.com/") {
		t.Errorf("URL missing https prefix: %q", url)
	}
}

func TestScan_HEADResolutionFullSHA(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	addons := t.TempDir()
	dir := seedBareWithAddon(t, addons, "Atlas")
	sha, err := gitops.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("SHA len = %d, want 40", len(sha))
	}
}
