// Package gitops wraps the system `git` binary. We use os/exec
// instead of go-git so we inherit the user's SSH keys, credential
// helpers, GPG config, and platform-specific behaviours.
package gitops

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// run executes a git command in dir and returns its trimmed
// stdout. It panics on non-zero exit, which is what we want in
// production code — every caller treats a non-zero exit as fatal
// and surfaces the error to the user.
// Run executes an arbitrary git command in dir and returns its
// trimmed stdout. Exported so callers can run infrequent git
// operations (rev-list, status) without adding a dedicated
// wrapper for each one.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

// Clone clones a remote URL into dest, optionally checking out a
// specific branch. An empty branch string is allowed and means
// "whatever the remote HEAD points to".
func Clone(url, dest, branch string) error {
	if url == "" {
		return errors.New("gitops: empty url")
	}
	if dest == "" {
		return errors.New("gitops: empty dest")
	}
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, url, dest)
	_, err := Run("", args...)
	return err
}

// Pull runs `git pull` in dir. It does not take a branch because
// the current branch is implicit; if you need to switch branches
// first, use Checkout.
func Pull(dir string) error {
	if _, err := Run(dir, "pull", "--ff-only"); err != nil {
		return err
	}
	return nil
}

// Fetch runs `git fetch` against origin.
func Fetch(dir string) error {
	if _, err := Run(dir, "fetch", "origin"); err != nil {
		return err
	}
	return nil
}

// Checkout switches the working tree to the given ref (branch,
// tag, or commit SHA). It does NOT create a branch.
func Checkout(dir, ref string) error {
	if ref == "" {
		return errors.New("gitops: empty ref")
	}
	if _, err := Run(dir, "checkout", ref); err != nil {
		return err
	}
	return nil
}

// HeadSHA returns the full 40-character SHA of the current HEAD.
// It works on branch, tag, and detached HEAD alike because it
// always reads `git rev-parse HEAD`.
func HeadSHA(dir string) (string, error) {
	out, err := Run(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(out)
	if len(sha) != 40 {
		return "", fmt.Errorf("gitops: expected 40-char SHA, got %q", sha)
	}
	return sha, nil
}

// RemoteURL returns the configured URL of the "origin" remote,
// normalised to strip any trailing ".git" suffix. HTTPS URLs have
// the suffix stripped per the design; SSH and local paths are
// preserved as-is.
func RemoteURL(dir string) (string, error) {
	out, err := Run(dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(out)
	url = strings.TrimSuffix(url, ".git")
	return url, nil
}

// DefaultBranch detects the default branch of a cloned repository.
// It reads the remote HEAD symref that git sets up during clone
// (origin/HEAD → origin/<branch>). Returns "" if detection fails.
func DefaultBranch(dir string) string {
	out, err := Run(dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	out = strings.TrimSpace(out)
	// Output is "refs/remotes/origin/<branch>"
	const prefix = "refs/remotes/origin/"
	if strings.HasPrefix(out, prefix) {
		return out[len(prefix):]
	}
	return ""
}

// SanitizeSegment strips path-traversal attempts and null bytes
// from a user-supplied segment. It is exported because higher
// layers (installer, UI) need the same defence when constructing
// paths for git commands that take file arguments.
//
// Whitespace and Unicode are allowed: WoW addon folder names can
// legitimately contain spaces and non-ASCII characters.
func SanitizeSegment(s string) (string, error) {
	if s == "" {
		return "", errors.New("gitops: empty segment")
	}
	if strings.ContainsRune(s, 0) {
		return "", errors.New("gitops: null byte in segment")
	}
	cleaned := filepath.Clean(s)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("gitops: path traversal detected")
	}
	return cleaned, nil
}
