// Package addon defines the on-disk entity managed by the tool: an
// addon folder with a `.toc` file. It is the bridge between the git
// world (URLs, refs, SHAs) and the WoW world (folders, .toc files).
package addon

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// moveFile moves src to dst. If os.Rename fails (e.g. cross-device
// link), it falls back to copying the file then removing the source.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device or same-filesystem rename failed — copy.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	info, _ := in.Stat()
	if info != nil {
		_ = os.Chmod(dst, info.Mode())
	}
	return os.Remove(src)
}

// moveDir moves a directory from src to dst. Uses os.Rename when
// possible, falls back to recursive copy + remove.
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyDir(src, dst)
}

// copyDir recursively copies src into dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err := moveFile(s, d); err != nil {
			return err
		}
	}
	return os.RemoveAll(src)
}

// TrackMode is the value of the track_mode field on an Addon. Only
// "branch" and "release" are valid; Validate rejects other values.
const (
	TrackModeBranch  = "branch"
	TrackModeRelease = "release"
)

// ErrInvalidURL is returned by ValidateURL for malformed inputs.
var ErrInvalidURL = errors.New("addon: invalid git URL")

// Addon is the persistent record of a tracked addon. It mirrors the
// config.Addon struct but lives in this package so callers do not
// have to import config just to read or write one.
type Addon struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	TrackMode   string `json:"track_mode"`
	TrackTarget string `json:"track_target"`
	CurrentSHA  string `json:"current_sha"`
}

// DeriveName returns the addon name from a git URL. The name is the
// last path segment with any ".git" suffix stripped, and any
// trailing slash removed. Examples:
//
//	DeriveName("https://github.com/u/Atlas.git")        -> "Atlas"
//	DeriveName("https://github.com/u/Atlas/")           -> "Atlas"
//	DeriveName("git@github.com:u/Atlas.git")           -> "Atlas"
//	DeriveName("https://gitlab.com/group/sub/MyAdd.git") -> "MyAdd"
func DeriveName(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("%w: empty URL", ErrInvalidURL)
	}

	// SSH-style "user@host:path" — we cannot pass to net/url
	// directly because the colon confuses the parser. Split on
	// the colon first.
	if strings.Contains(rawURL, "@") && !strings.Contains(rawURL, "://") {
		idx := strings.Index(rawURL, ":")
		if idx == -1 {
			return "", fmt.Errorf("%w: malformed SSH URL %q", ErrInvalidURL, rawURL)
		}
		rawURL = rawURL[idx+1:]
	} else {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
		}
		if u.Scheme == "" {
			return "", fmt.Errorf("%w: missing scheme in %q", ErrInvalidURL, rawURL)
		}
		rawURL = u.Path
	}

	// Strip query/fragment, then drop the last segment.
	rawURL = strings.SplitN(rawURL, "?", 2)[0]
	rawURL = strings.SplitN(rawURL, "#", 2)[0]
	rawURL = strings.TrimRight(rawURL, "/")
	if rawURL == "" {
		return "", fmt.Errorf("%w: no path component in %q", ErrInvalidURL, rawURL)
	}

	seg := rawURL
	if i := strings.LastIndex(rawURL, "/"); i != -1 {
		seg = rawURL[i+1:]
	}
	if seg == "" {
		return "", fmt.Errorf("%w: no name segment in %q", ErrInvalidURL, rawURL)
	}
	seg = strings.TrimSuffix(seg, ".git")
	if seg == "" {
		return "", fmt.Errorf("%w: name segment empty after strip in %q", ErrInvalidURL, rawURL)
	}
	return seg, nil
}

// ValidateURL performs a basic syntactic check on a candidate git
// URL. It accepts both HTTPS (https://...) and SSH (user@host:...)
// forms. This is a sanity check, not a deep validation — we still
// rely on git to confirm the repo exists.
func ValidateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	// SSH form
	if strings.HasPrefix(rawURL, "git@") || (strings.Contains(rawURL, "@") && !strings.Contains(rawURL, "://")) {
		if !strings.Contains(rawURL, ":") {
			return fmt.Errorf("%w: SSH URL missing colon", ErrInvalidURL)
		}
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrInvalidURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return nil
}
