// Package gitops provides git operations using go-git. The user
// does not need git installed on their system.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/pentsec/lazyaddons/internal/semver"

	"github.com/pentsec/lazyaddons/internal/safepath"
)

// openRepo opens an existing git repository at dir.
func openRepo(dir string) (*git.Repository, *git.Worktree, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("gitops: open %s: %w", dir, err)
	}
	w, err := r.Worktree()
	if err != nil {
		return nil, nil, fmt.Errorf("gitops: worktree %s: %w", dir, err)
	}
	return r, w, nil
}

// Clone clones a remote URL into dest, optionally checking out a
// specific ref. The ref may be a branch name, a tag name, or a
// commit SHA; it is resolved automatically against the remote.
// An empty ref means "whatever the remote HEAD points to".
func Clone(ctx context.Context, url, dest, ref string) error {
	if url == "" {
		return errors.New("gitops: empty url")
	}
	if dest == "" {
		return errors.New("gitops: empty dest")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts := &git.CloneOptions{
		URL:               url,
		SingleBranch:      true,
		RecurseSubmodules: git.NoRecurseSubmodules,
	}
	if ref != "" {
		refName, err := resolveRemoteRef(url, ref)
		if err != nil {
			return fmt.Errorf("gitops: clone %s: %w", url, err)
		}
		opts.ReferenceName = refName
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := git.PlainClone(dest, false, opts)
	if err != nil {
		return fmt.Errorf("gitops: clone %s: %w", url, err)
	}
	return nil
}

// resolveRemoteRef asks the remote which kind of ref `ref` is
// (branch, tag, or commit) and returns the fully-qualified
// plumbing.ReferenceName to use for cloning. This avoids the
// go-git default of always looking under refs/heads/, which
// breaks when cloning a release tag.
func resolveRemoteRef(url, ref string) (plumbing.ReferenceName, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	refs, err := rem.List(&git.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list remote: %w", err)
	}
	// Prefer branch, then tag.
	branchRef := plumbing.NewBranchReferenceName(ref)
	tagRef := plumbing.NewTagReferenceName(ref)
	for _, r := range refs {
		if r.Name() == branchRef {
			return branchRef, nil
		}
	}
	for _, r := range refs {
		if r.Name() == tagRef {
			return tagRef, nil
		}
	}
	// Maybe it's a commit hash; let PlainClone resolve it directly
	// by using it as branch ref name (go-git falls back to hash).
	if h := plumbing.NewHash(ref); h != plumbing.ZeroHash {
		return branchRef, nil
	}
	return "", fmt.Errorf("remote has no branch or tag named %q", ref)
}

// Fetch fetches from origin. Uses an explicit refspec to ensure
// every remote branch is downloaded, even in single-branch clones
// where the local remote config only covers a subset of refs.
func Fetch(dir string) error {
	r, _, err := openRepo(dir)
	if err != nil {
		return err
	}
	err = r.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/tags/*:refs/tags/*"),
		},
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("gitops: fetch: %w", err)
	}
	return nil
}

// Pull does a fast-forward-only pull from origin on the current branch.
func Pull(dir string) error {
	_, w, err := openRepo(dir)
	if err != nil {
		return err
	}
	err = w.Pull(&git.PullOptions{RemoteName: "origin"})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("gitops: pull: %w", err)
	}
	return nil
}

// Checkout switches the working tree to the given ref (branch,
// tag, or commit SHA). It does NOT create a branch.
func Checkout(dir, ref string) error {
	if ref == "" {
		return errors.New("gitops: empty ref")
	}
	_, w, err := openRepo(dir)
	if err != nil {
		return err
	}
	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(ref),
	}); err == nil {
		return nil
	}
	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewTagReferenceName(ref),
	}); err == nil {
		return nil
	}
	if h := plumbing.NewHash(ref); h != plumbing.ZeroHash {
		if err := w.Checkout(&git.CheckoutOptions{Hash: h}); err == nil {
			return nil
		}
	}
	return fmt.Errorf("gitops: checkout %s: not a valid branch, tag, or commit", ref)
}

// HeadSHA returns the full 40-character SHA of the current HEAD.
func HeadSHA(dir string) (string, error) {
	r, _, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	ref, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("gitops: head: %w", err)
	}
	sha := ref.Hash().String()
	if len(sha) != 40 {
		return "", fmt.Errorf("gitops: expected 40-char SHA, got %q", sha)
	}
	return sha, nil
}

// RemoteURL returns the configured URL of the "origin" remote,
// normalised to strip any trailing ".git" suffix.
func RemoteURL(dir string) (string, error) {
	r, _, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	remote, err := r.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("gitops: remote origin: %w", err)
	}
	cfg := remote.Config()
	if len(cfg.URLs) == 0 {
		return "", errors.New("gitops: origin has no URLs")
	}
	url := strings.TrimSpace(cfg.URLs[0])
	url = strings.TrimSuffix(url, ".git")
	return url, nil
}

// DefaultBranch detects the default branch of a cloned repository.
// It reads the remote HEAD symref that git sets up during clone
// (origin/HEAD → origin/<branch>). Returns "" if detection fails.
func DefaultBranch(dir string) string {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return ""
	}
	ref, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), true)
	if err == nil {
		name := ref.Name().String()
		const prefix = "refs/remotes/origin/"
		if strings.HasPrefix(name, prefix) {
			branch := name[len(prefix):]
			if branch != "HEAD" && branch != "" {
				return branch
			}
		}
	}
	if branch := defaultBranchFromPackedRefs(dir); branch != "" {
		return branch
	}
	for _, name := range []string{"main", "master"} {
		_, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/"+name), true)
		if err == nil {
			return name
		}
	}
	return ""
}

// defaultBranchFromPackedRefs reads .git/packed-refs and looks for
// the origin/HEAD entry to determine the default branch.
func defaultBranchFromPackedRefs(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".git", "packed-refs"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[len(fields)-1] != "refs/remotes/origin/HEAD" {
			continue
		}
		if fields[0] == "ref:" && len(fields) >= 3 {
			target := fields[1]
			const originPrefix = "refs/remotes/origin/"
			if strings.HasPrefix(target, originPrefix) {
				return target[len(originPrefix):]
			}
		}
	}
	return ""
}

// ResetWorkingTree discards all local changes and restores the
// working tree to match HEAD. Uses `git reset --hard` which is
// more reliable than `git checkout -- .` when files were moved
// out of the repo (os.Rename) — checkout fails to recreate
// deleted directories, but reset --hard always works.
func ResetWorkingTree(dir string) error {
	_, w, err := openRepo(dir)
	if err != nil {
		return err
	}
	if err := w.Reset(&git.ResetOptions{Mode: git.HardReset}); err != nil {
		return fmt.Errorf("gitops: reset worktree: %w", err)
	}
	return nil
}

// MergeFF performs a fast-forward merge from the given ref into the
// current branch. Returns an error if the merge is not possible.
func MergeFF(dir string, ref string) error {
	r, _, err := openRepo(dir)
	if err != nil {
		return err
	}
	target, err := r.Reference(plumbing.ReferenceName(ref), true)
	if err != nil {
		return fmt.Errorf("gitops: resolve ref %s: %w", ref, err)
	}
	head, err := r.Head()
	if err != nil {
		return fmt.Errorf("gitops: head: %w", err)
	}
	headHash := head.Hash()
	targetHash := target.Hash()
	if headHash == targetHash {
		return nil
	}
	isFF, err := walkAncestors(r, headHash, targetHash)
	if err != nil {
		return fmt.Errorf("gitops: ff check: %w", err)
	}
	if !isFF {
		return fmt.Errorf("gitops: merge --ff-only: not a fast-forward")
	}
	// Move the branch ref to the target commit.
	if err := r.Storer.SetReference(
		plumbing.NewHashReference(head.Name(), targetHash),
	); err != nil {
		return fmt.Errorf("gitops: update ref: %w", err)
	}
	// Re-open the repo to clear go-git's in-memory reference cache,
	// then hard-reset the worktree to match the updated HEAD.
	_, w2, err := openRepo(dir)
	if err != nil {
		return fmt.Errorf("gitops: reopen for reset: %w", err)
	}
	if err := w2.Reset(&git.ResetOptions{Mode: git.HardReset}); err != nil {
		return fmt.Errorf("gitops: reset after merge: %w", err)
	}
	return nil
}

// walkAncestors checks whether needleHash is an ancestor of startHash
// by recursively walking parent commits. Returns true if found.
// The base case (needleHash == startHash) returns true because a
// commit is considered an ancestor of itself.
func walkAncestors(r *git.Repository, needleHash, startHash plumbing.Hash) (bool, error) {
	if needleHash == startHash {
		return true, nil
	}
	commit, err := r.CommitObject(startHash)
	if err != nil {
		return false, err
	}
	iter := commit.Parents()
	defer iter.Close()
	for {
		parent, err := iter.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Skip objects that can't be resolved (shallow repos).
			continue
		}
		found, err := walkAncestors(r, needleHash, parent.Hash)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// IsBehind returns true if HEAD is behind its upstream tracking
// branch. Resolves the upstream by reading git config.
func IsBehind(dir string) (bool, error) {
	r, _, err := openRepo(dir)
	if err != nil {
		return false, err
	}
	head, err := r.Head()
	if err != nil {
		return false, nil
	}
	headName := head.Name().String()
	if !strings.HasPrefix(headName, "refs/heads/") {
		return isBehindRef(r, head.Hash(), "main") || isBehindRef(r, head.Hash(), "master"), nil
	}
	branchName := strings.TrimPrefix(headName, "refs/heads/")
	cfg, err := r.Config()
	if err != nil {
		return false, nil
	}
	bcfg, ok := cfg.Branches[branchName]
	if !ok || bcfg.Merge == "" {
		return isBehindRef(r, head.Hash(), "main") || isBehindRef(r, head.Hash(), "master"), nil
	}
	remoteRef := strings.Replace(
		string(bcfg.Merge),
		"refs/heads/",
		"refs/remotes/"+bcfg.Remote+"/",
		1,
	)
	ref, err := r.Reference(plumbing.ReferenceName(remoteRef), true)
	if err != nil {
		return false, nil
	}
	if head.Hash() == ref.Hash() {
		return false, nil
	}
	return walkAncestors(r, head.Hash(), ref.Hash())
}

func isBehindRef(r *git.Repository, headHash plumbing.Hash, branch string) bool {
	ref, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/"+branch), true)
	if err != nil {
		return false
	}
	if headHash == ref.Hash() {
		return false
	}
	behind, _ := walkAncestors(r, headHash, ref.Hash())
	return behind
}

// LastCommitDate returns the date of the latest commit in
// YYYY-MM-DD format.
func LastCommitDate(dir string) string {
	r, _, err := openRepo(dir)
	if err != nil {
		return ""
	}
	ref, err := r.Head()
	if err != nil {
		return ""
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return ""
	}
	return commit.Committer.When.Format("2006-01-02")
}

// LatestNewTag returns the newest tag (by semver) in the repo that
// is strictly newer than currentTag. Returns ("", nil) when the
// addon is already on the latest tag.
func LatestNewTag(dir, currentTag string) (string, error) {
	r, _, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	iter, err := r.Tags()
	if err != nil {
		return "", fmt.Errorf("gitops: list tags: %w", err)
	}
	defer iter.Close()

	type tagInfo struct {
		name    string
		segments [3]int
	}
	var tags []tagInfo
	cur, _ := semver.Parse(currentTag)

	iter.ForEach(func(ref *plumbing.Reference) error {
		name := strings.TrimPrefix(string(ref.Name()), "refs/tags/")
		seg, _ := semver.Parse(name)
		// Only include tags that are strictly newer than current.
		if semver.Compare(seg, cur) > 0 {
			tags = append(tags, tagInfo{name: name, segments: seg})
		}
		return nil
	})
	if len(tags) == 0 {
		return "", nil
	}
	sort.Slice(tags, func(i, j int) bool {
		return semver.Compare(tags[i].segments, tags[j].segments) > 0
	})
	return tags[0].name, nil
}

// CheckoutTag checks out a tag in detached HEAD mode and resets
// the working tree to match. Unlike w.Checkout, this works even
// when the worktree has been modified (e.g. after UnpackUpdate
// moved files out). It sets HEAD directly and hard-resets.
func CheckoutTag(dir, tag string) error {
	if tag == "" {
		return errors.New("gitops: empty tag")
	}
	r, w, err := openRepo(dir)
	if err != nil {
		return err
	}
	tagRef := plumbing.NewTagReferenceName(tag)
	ref, err := r.Reference(tagRef, true)
	if err != nil {
		return fmt.Errorf("gitops: resolve tag %s: %w", tag, err)
	}
	// Resolve annotated tag objects to the underlying commit.
	h, err := r.ResolveRevision(plumbing.Revision(ref.Hash().String()))
	if err != nil {
		return fmt.Errorf("gitops: resolve revision for tag %s: %w", tag, err)
	}
	// Detached HEAD: write HEAD directly to the commit hash,
	// bypassing w.Checkout which fails on modified worktrees.
	if err := r.Storer.SetReference(
		plumbing.NewHashReference(plumbing.HEAD, *h),
	); err != nil {
		return fmt.Errorf("gitops: set HEAD for tag %s: %w", tag, err)
	}
	if err := w.Reset(&git.ResetOptions{Mode: git.HardReset}); err != nil {
		return fmt.Errorf("gitops: reset after tag %s: %w", tag, err)
	}
	return nil
}

// SanitizeSegment strips path-traversal attempts and null bytes
// from a user-supplied segment.
func SanitizeSegment(s string) (string, error) {
	return safepath.Validate(s)
}