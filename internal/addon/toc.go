// Package addon — TOC parsing, validation, and discovery.
package addon

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrTOCMismatch is returned by ValidateTOC when the .toc file name
// does not match the addon folder name.
var ErrTOCMismatch = errors.New("addon: .toc file name does not match folder")

// ErrNoTOC is returned by ValidateTOC when the folder has no .toc
// file at all.
var ErrNoTOC = errors.New("addon: no .toc file found")

// TOC holds parsed header fields from a .toc file.
type TOC struct {
	Title        string
	Interface    string
	Author       string
	Notes        string
	Version      string
	Dependencies []string
	Path         string
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
