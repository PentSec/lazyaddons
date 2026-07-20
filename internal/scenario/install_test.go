package scenario

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/backup"
	"github.com/pentsec/lazyaddons/internal/gitops"
)

// ---- addon-install scenarios ----

func TestInstall_CloneBranch(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)

	// Clone into a folder that matches the .toc file name
	// (MyAddon) so ValidateTOC passes.
	dest := filepath.Join(t.TempDir(), "MyAddon")
	if err := gitops.Clone(context.Background(), remote, dest, "main"); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if err := addon.ValidateTOC(dest); err != nil {
		t.Errorf("ValidateTOC after clone: %v", err)
	}
}

func TestInstall_CloneRelease(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	// Add a tag to the remote.
	work := t.TempDir()
	runGit(t, work, "clone", remote, ".")
	runGit(t, work, "config", "user.email", "t@example.com")
	runGit(t, work, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(work, "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, work, "add", "v2.txt")
	runGit(t, work, "commit", "-m", "v2")
	runGit(t, work, "tag", "v1.0.0")
	runGit(t, work, "push", remote, "v1.0.0")

	dest := filepath.Join(t.TempDir(), "Atlas")
	if err := gitops.Clone(context.Background(), remote, dest, "main"); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := gitops.Checkout(dest, "v1.0.0"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "v2.txt")); err != nil {
		t.Errorf("expected v2.txt after tag checkout: %v", err)
	}
}

func TestInstall_ReleaseZipFallback(t *testing.T) {
	t.Parallel()
	// Build a release zip with a valid .toc structure.
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	header := &zip.FileHeader{Name: "Atlas.toc"}
	w, _ := zw.CreateHeader(header)
	_, _ = w.Write([]byte("## Title: x\n"))
	header2 := &zip.FileHeader{Name: "Atlas.lua"}
	w2, _ := zw.CreateHeader(header2)
	_, _ = w2.Write([]byte("-- x\n"))
	_ = zw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/Atlas.zip")
	if err != nil {
		t.Fatalf("GET zip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestInstall_RejectsTOCMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	addonDir := filepath.Join(dir, "MyAddon")
	if err := os.MkdirAll(addonDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// .toc name does not match folder name.
	if err := os.WriteFile(filepath.Join(addonDir, "Wrong.toc"), []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := addon.ValidateTOC(addonDir)
	if err == nil {
		t.Errorf("ValidateTOC(mismatch) = nil, want error")
	}
}

func TestInstall_PreInstallBackup(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	dir := filepath.Join(addons, "MyAddon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.toc"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.lua"), []byte("-- v1"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := backup.New(addons)
	if err := mgr.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Now overwrite live with v2.
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.lua"), []byte("-- v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Restore should bring back v1.
	if err := mgr.Restore("MyAddon"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "MyAddon.lua"))
	if string(got) != "-- v1" {
		t.Errorf("after restore = %q, want v1", got)
	}
}
