package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if the system has no `git` binary in
// PATH. Every test in this file shells out to git so a missing
// binary is not a test failure — it is an environmental gap.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not in PATH: %v", err)
	}
}

// newBareRemote creates a local bare git repo to act as a remote
// for the tests. The repo is initialised with HEAD pointing to
// `main` but contains no commits yet — callers must push at
// least one commit before the remote is cloneable.
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare", "--initial-branch=main", ".")
	return dir
}

// seedRemote creates an initial commit on `main` and pushes it to
// the bare remote. Returns the commit SHA.
func seedRemote(t *testing.T, remote string) string {
	t.Helper()
	work := t.TempDir()
	runGit(t, work, "init", "--initial-branch=main", ".")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-m", "initial")
	runGit(t, work, "remote", "add", "origin", remote)
	runGit(t, work, "push", "origin", "main")
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	return sha
}

// newClone clones a bare repo into a working directory and returns
// the working directory.
func newClone(t *testing.T, remote string) string {
	t.Helper()
	work := t.TempDir()
	runGit(t, work, "clone", remote, ".")
	// Configure committer identity so commits succeed.
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	return work
}

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

func TestHeadSHA_OnBranch(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)

	sha, err := HeadSHA(work)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("HeadSHA = %q (len %d), want 40-char SHA", sha, len(sha))
	}
}

func TestHeadSHA_OnTag(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)

	// Add a second commit and tag it.
	if err := os.WriteFile(filepath.Join(work, "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, work, "add", "v2.txt")
	runGit(t, work, "commit", "-m", "v2")
	tagSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	runGit(t, work, "tag", "v1.0.0")

	// Check out the tag (detached HEAD at the tag's commit).
	runGit(t, work, "checkout", "v1.0.0")

	got, err := HeadSHA(work)
	if err != nil {
		t.Fatalf("HeadSHA on tag: %v", err)
	}
	if got != tagSHA {
		t.Errorf("HeadSHA on tag = %q, want %q", got, tagSHA)
	}
}

func TestHeadSHA_OnDetachedCommit(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)
	// Add a second commit.
	if err := os.WriteFile(filepath.Join(work, "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, work, "add", "v2.txt")
	runGit(t, work, "commit", "-m", "v2")
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	// Checkout the first commit (detached).
	first := strings.TrimSpace(runGit(t, work, "rev-list", "--max-parents=0", "HEAD"))
	runGit(t, work, "checkout", first)

	got, err := HeadSHA(work)
	if err != nil {
		t.Fatalf("HeadSHA detached: %v", err)
	}
	if got != first {
		t.Errorf("HeadSHA detached = %q, want %q", got, first)
	}
	if got == sha {
		t.Errorf("HeadSHA detached == second commit SHA, want first commit SHA")
	}
}

func TestRemoteURL_Normalises(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)

	got, err := RemoteURL(work)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	// Local file paths should round-trip via filepath.Abs.
	want, _ := filepath.Abs(remote)
	if got != want {
		t.Errorf("RemoteURL = %q, want %q", got, want)
	}
}

func TestRemoteURL_PreservesHTTPSPrefix(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	work := t.TempDir()
	runGit(t, work, "init", ".")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	runGit(t, work, "remote", "add", "origin", "https://github.com/user/Atlas.git")
	runGit(t, work, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(work, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-m", "x")

	got, err := RemoteURL(work)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if !strings.HasPrefix(got, "https://") {
		t.Errorf("RemoteURL = %q, want https://...", got)
	}
	if strings.HasSuffix(got, ".git") {
		// Design says: strip .git suffix for HTTPS URLs.
		t.Errorf("RemoteURL = %q still has .git suffix", got)
	}
}

func TestPull_FastForward(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)
	// Push from another clone so the remote has new commits.
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

	// Now pull in the original work.
	if err := Pull(work); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "new.txt")); err != nil {
		t.Errorf("expected new.txt after pull: %v", err)
	}
}

func TestFetch_DoesNotMutateWorkdir(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)

	if err := Fetch(work); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Fetch should not add a new file to the workdir.
	if _, err := os.Stat(filepath.Join(work, "new.txt")); err == nil {
		t.Errorf("fetch added files to workdir, want read-only")
	}
}

func TestCheckout_Ref(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	work := newClone(t, remote)
	if err := os.WriteFile(filepath.Join(work, "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, work, "add", "v2.txt")
	runGit(t, work, "commit", "-m", "v2")
	runGit(t, work, "tag", "v1.0.0")

	if err := Checkout(work, "v1.0.0"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	// On a tag checkout, the workdir is detached; confirm HEAD
	// points to the tag's commit.
	want := strings.TrimSpace(runGit(t, work, "rev-parse", "v1.0.0"))
	got := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	if got != want {
		t.Errorf("after Checkout HEAD = %q, want %q", got, want)
	}
}

func TestClone_ProducesWorkingRepo(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	dest := filepath.Join(t.TempDir(), "MyAddon")
	if err := Clone(remote, dest, "main"); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Errorf("cloned repo missing README: %v", err)
	}
}

// TestClone_TagRef reproduces the release-clone bug: when a user
// picks a GitHub release tag (e.g. "v3.0.6"), Clone must resolve
// it against refs/tags/, not refs/heads/.
func TestClone_TagRef(t *testing.T) {
	if testing.Short() {
		t.Skipf("requires git binary")
	}
	t.Parallel()
	requireGit(t)

	remote := newBareRemote(t)
	seedRemote(t, remote)
	// Create a tag on the remote via a working clone.
	work := newClone(t, remote)
	runGit(t, work, "tag", "v3.0.6")
	runGit(t, work, "push", "origin", "v3.0.6")

	dest := filepath.Join(t.TempDir(), "DragonUI")
	if err := Clone(remote, dest, "v3.0.6"); err != nil {
		t.Fatalf("Clone by tag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Errorf("cloned repo missing README: %v", err)
	}
	// HEAD should point at the tag's commit (detached).
	got := strings.TrimSpace(runGit(t, dest, "rev-parse", "HEAD"))
	want := strings.TrimSpace(runGit(t, dest, "rev-parse", "v3.0.6"))
	if got != want {
		t.Errorf("after tag-clone HEAD = %q, want %q", got, want)
	}
}
