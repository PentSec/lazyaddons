package scenario

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not in PATH: %v", err)
	}
}

// newBareRemote creates a local bare git repo to act as a remote
// for the tests.
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare", "--initial-branch=main", ".")
	return dir
}

// seedRemote creates an initial commit on `main` and pushes it to
// the bare remote. The commit includes a README.md, a MyAddon.toc
// file, and a MyAddon.lua file so cloned repos pass .toc
// validation. Returns the commit SHA.
func seedRemote(t *testing.T, remote string) string {
	t.Helper()
	work := t.TempDir()
	runGit(t, work, "init", "--initial-branch=main", ".")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "MyAddon.toc"), []byte("## Title: x\n"), 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "MyAddon.lua"), []byte("-- x\n"), 0o644); err != nil {
		t.Fatalf("write lua: %v", err)
	}
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-m", "initial")
	runGit(t, work, "remote", "add", "origin", remote)
	runGit(t, work, "push", "origin", "main")
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	return sha
}

// newClone clones a bare repo into a working directory.
func newClone(t *testing.T, remote string) string {
	t.Helper()
	work := t.TempDir()
	runGit(t, work, "clone", remote, ".")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	return work
}

// runGit runs a git command and returns its combined output.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}
