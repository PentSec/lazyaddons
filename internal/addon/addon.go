// Package addon defines the on-disk entity managed by the tool: an
// addon folder with a `.toc` file. It is the bridge between the git
// world (URLs, refs, SHAs) and the WoW world (folders, .toc files).
package addon

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// TrackMode is the value of the track_mode field on an Addon. Only
// "branch" and "release" are valid; Validate rejects other values.
const (
	TrackModeBranch  = "branch"
	TrackModeRelease = "release"
)

// ErrInvalidURL is returned by ValidateURL for malformed inputs.
var ErrInvalidURL = errors.New("addon: invalid git URL")

// ErrTOCMismatch is returned by ValidateTOC when the .toc file name
// does not match the addon folder name.
var ErrTOCMismatch = errors.New("addon: .toc file name does not match folder")

// ErrNoTOC is returned by ValidateTOC when the folder has no .toc
// file at all.
var ErrNoTOC = errors.New("addon: no .toc file found")

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

// ParseTOC reads a .toc file and returns the parsed header fields.
// Only the fields we currently use are returned; the rest of the
// file is ignored.
type TOC struct {
	Title       string
	Interface   string
	Author      string
	Notes       string
	Version     string
	Dependencies []string
	Path        string
}

// ParseTOCFile parses a .toc file at the given path.
func ParseTOCFile(path string) (TOC, error) {
	f, err := os.Open(path)
	if err != nil {
		return TOC{}, fmt.Errorf("addon: open %s: %w", path, err)
	}
	defer f.Close()

	toc := TOC{Path: path}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Strip the standard "## " prefix used by Blizzard TOC
		// files. Lines that are just "#" without the second
		// hash are treated as comments and skipped.
		if strings.HasPrefix(line, "##") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "##"))
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Title":
			toc.Title = value
		case "Interface":
			toc.Interface = value
		case "Author":
			toc.Author = value
		case "Notes":
			toc.Notes = value
		case "Version":
			toc.Version = value
		case "Dependencies", "RequiredDeps", "OptionalDeps":
			for _, d := range strings.Fields(value) {
				toc.Dependencies = append(toc.Dependencies, d)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return toc, fmt.Errorf("addon: read %s: %w", path, err)
	}
	return toc, nil
}

// ValidateTOC asserts that the given directory contains exactly one
// .toc file (or one matching the folder name) and that the file's
// basename matches the directory's basename.
//
// Behaviour:
//   - If the folder is missing -> error.
//   - If the folder has no .toc file -> ErrNoTOC.
//   - If multiple .toc files exist -> still validated, but
//     prefers the one whose basename equals the folder name.
//   - If the chosen .toc basename does not match the folder name ->
//     ErrTOCMismatch.
func ValidateTOC(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("addon: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("addon: %s is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("addon: readdir %s: %w", dir, err)
	}

	folderName := filepath.Base(dir)

	var tocs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			tocs = append(tocs, e.Name())
		}
	}
	if len(tocs) == 0 {
		return fmt.Errorf("%w: in %s", ErrNoTOC, dir)
	}

	// Prefer the .toc whose basename matches the folder name.
	var chosen string
	expected := folderName + ".toc"
	for _, t := range tocs {
		if strings.EqualFold(t, expected) {
			chosen = t
			break
		}
	}
	if chosen == "" {
		// No matching basename — pick the first one and report
		// a mismatch error so the user sees both names.
		chosen = tocs[0]
		base := strings.TrimSuffix(chosen, filepath.Ext(chosen))
		return fmt.Errorf("%w: folder %q vs .toc %q", ErrTOCMismatch, folderName, base)
	}
	return nil
}

// PromoteAddonDirs unpacks a cloned addon repository following
// the GitAddonsManager strategy:
//
//  1. The clone is renamed to .lazyaddons/<name> with .git inside.
//  2. Subdirectories whose name matches a .toc file inside them
//     are moved to the AddOns root so WoW can discover them.
//  3. The repo directory keeps ALL original files; on update we
//     git checkout/pull there and re-unpack.
//  4. Repository junk is cleaned from the repo.
//
// Returns all addon names now present directly in addonsRoot.
func PromoteAddonDirs(addonsRoot, cloneDir string) ([]string, error) {
	cloneBase := filepath.Base(cloneDir)

	// Scan for subdirectories with matching .toc files.
	subAddons := ScanTOCSubdirs(cloneDir)
	mainIsNested := hasSubAddon(subAddons, cloneBase)

	// Always rename the clone to .lazyaddons/<name> so .git has a
	// permanent home independent of the unpacked addon dirs.
	lazyDir := filepath.Join(addonsRoot, ".lazyaddons")
	_ = os.MkdirAll(lazyDir, 0o755)
	repoDir := filepath.Join(lazyDir, cloneBase)
	if mainIsNested {
		if err := os.Rename(cloneDir, repoDir); err != nil {
			return nil, fmt.Errorf("addon: rename %s -> %s: %w", cloneDir, repoDir, err)
		}
	} else {
		// Flat addon: rename clone to .repo too, then move the
		// addon folder back out. This keeps .git in .repo/.
		if err := os.Rename(cloneDir, repoDir); err != nil {
			return nil, fmt.Errorf("addon: rename %s -> %s: %w", cloneDir, repoDir, err)
		}
	}

	// Move all .toc subdirectories from .repo/ to AddOns root.
	// The originals stay in .repo/; git checkout will restore
	// them on next update so we can re-unpack.
	var promoted []string
	for _, name := range subAddons {
		src := filepath.Join(repoDir, name)
		dst := filepath.Join(addonsRoot, name)
		if _, err := os.Stat(dst); err == nil {
			continue // already exists, don't overwrite
		}
		if err := os.Rename(src, dst); err != nil {
			continue // best-effort, the copy in .repo is the source of truth
		}
		promoted = append(promoted, name)
	}

	// If the main addon wasn't in subAddons (flat structure where
	// the clone dir itself is the addon, no nesting), move it
	// from .repo to AddOns root now.
	//
	// Only run the flat flow when the repo root actually contains
	// a .toc file. Otherwise the clone dir is just a container
	// (e.g. Asc_Gathermate2) and should stay as .repo/ only.
	if !mainIsNested && repoDirHasTOC(repoDir) {
		// The clone dir WAS the addon. After rename to .repo,
		// we need to move everything except .git out.
		//
		// Determine the actual addon name from the .toc file. The
		// repo may have a different basename (e.g. CleanerChat-WotLK
		// repo contains CleanerChat.toc), and WoW requires the
		// folder name to match the .toc basename.
		actualName := cloneBase
		if tocName := tocAddonName(repoDir); tocName != "" {
			actualName = tocName
		}
		// Rename the repo dir if it doesn't match the real name.
		if !strings.EqualFold(actualName, cloneBase) {
			newRepoDir := filepath.Join(lazyDir, actualName)
			_ = os.Rename(repoDir, newRepoDir)
			repoDir = newRepoDir
		}
		entries, _ := os.ReadDir(repoDir)
		destDir := filepath.Join(addonsRoot, actualName)
		_ = os.MkdirAll(destDir, 0o755)
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			src := filepath.Join(repoDir, e.Name())
			dst := filepath.Join(destDir, e.Name())
			_ = os.Rename(src, dst)
		}
		promoted = append(promoted, actualName)
	}

	// Clean junk from .repo/.
	cleanRepoJunk(repoDir)

	return promoted, nil
}

// UnpackUpdate moves addon directories from a git repo (already at
// .lazyaddons/<name>) to the AddOns root. It is the update-path
// counterpart to PromoteAddonDirs: it skips the install-only rename
// step and force-overwrites old unpacked directories so the freshly
// pulled files always land in the right place.
func UnpackUpdate(addonsRoot, repoDir string) {
	cloneBase := filepath.Base(repoDir)
	subAddons := ScanTOCSubdirs(repoDir)
	mainIsNested := hasSubAddon(subAddons, cloneBase)

	// Delete old unpacked dirs from AddOns root.
	for _, s := range subAddons {
		_ = os.RemoveAll(filepath.Join(addonsRoot, s))
	}
	_ = os.RemoveAll(filepath.Join(addonsRoot, cloneBase))

	// Move subdirectories from repo to AddOns root.
	for _, s := range subAddons {
		src := filepath.Join(repoDir, s)
		dst := filepath.Join(addonsRoot, s)
		_ = os.RemoveAll(dst) // belt and suspenders
		_ = os.Rename(src, dst)
	}

	// Flat addon: the repo root IS the addon.
	if !mainIsNested && repoDirHasTOC(repoDir) {
		actualName := cloneBase
		if tocName := tocAddonName(repoDir); tocName != "" {
			actualName = tocName
		}
		entries, _ := os.ReadDir(repoDir)
		destDir := filepath.Join(addonsRoot, actualName)
		_ = os.MkdirAll(destDir, 0o755)
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			src := filepath.Join(repoDir, e.Name())
			dst := filepath.Join(destDir, e.Name())
			_ = os.Rename(src, dst)
		}
	}

	cleanRepoJunk(repoDir)
}

// ScanTOCSubdirs returns the original names of immediate
// subdirectories that contain a matching .toc file.
func ScanTOCSubdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == ".git" {
			continue
		}
		expectedTOC := e.Name() + ".toc"
		subEntries, err := os.ReadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			if strings.EqualFold(se.Name(), expectedTOC) {
				result = append(result, e.Name())
				break
			}
		}
	}
	return result
}

// hasSubAddon reports whether the named subdirectory exists in
// the subAddons list (case-insensitive match).
func hasSubAddon(subAddons []string, name string) bool {
	for _, s := range subAddons {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

// cleanRepoJunk removes well-known non-addon files and
// directories from a repository checkout. .git and .toc files
// are always preserved.
func cleanRepoJunk(dir string) {
	junkNames := map[string]bool{
		".github": true, ".gitignore": true, ".gitattributes": true,
		".gitmodules": true, ".ignore": true,
		"README.md": true, "README": true,
		"LICENSE": true, "LICENSE.md": true, "LICENCE": true,
		"LICENCE.md": true, "LICENSES": true,
		"CHANGELOG.md": true, "CHANGELOG": true,
		"CONTRIBUTING.md": true, "CONTRIBUTING": true,
		"CODE_OF_CONDUCT.md": true,
		"THIRD_PARTY_NOTICES.md": true,
	}
	junkDirPrefixes := []string{
		".github", ".vscode", ".idea",
		"img", "image", "images",
		"screen", "screenshot", "screenshots",
		"doc", "docs",
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		name := strings.ToLower(e.Name())
		if e.Name() == ".git" {
			continue
		}
		if e.IsDir() && hasAnyTOC(path) {
			continue
		}
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			continue
		}
		if junkNames[e.Name()] {
			_ = os.RemoveAll(path)
			continue
		}
		if e.IsDir() {
			for _, pfx := range junkDirPrefixes {
				if strings.HasPrefix(name, pfx) {
					_ = os.RemoveAll(path)
					break
				}
			}
		}
	}
}

// hasAnyTOC reports whether dir or any of its immediate children
// contains a .toc file.
func hasAnyTOC(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			return true
		}
		if e.IsDir() {
			sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			for _, s := range sub {
				if strings.EqualFold(filepath.Ext(s.Name()), ".toc") {
					return true
				}
			}
		}
	}
	return false
}

// repoDirHasTOC reports whether the repo root (after sub-addon
// promotion) contains at least one .toc file at the top level.
// Used to decide whether the clone base itself is a valid addon.
func repoDirHasTOC(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			return true
		}
	}
	return false
}

// tocAddonName returns the basename (without .toc extension) of the
// first .toc file found at the top level of dir. WoW requires the
// addon folder name to match the .toc filename; when a repo's
// basename differs from the .toc name (e.g. "CleanerChat-WotLK"
// repo containing "CleanerChat.toc"), this function provides the
// correct folder name.
func tocAddonName(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			return strings.TrimSuffix(e.Name(), ".toc")
		}
	}
	return ""
}
