// Package addon — zip archive extraction for release downloads.
package addon

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractReleaseZip extracts a zip archive into destDir and returns
// the path. The caller is responsible for removing destDir when done.
func ExtractReleaseZip(destDir string, r io.ReaderAt, size int64) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("addon: open zip: %w", err)
	}

	cleaned := filepath.Clean(destDir) + string(filepath.Separator)

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := f.Name

		if !filepath.IsLocal(rel) {
			continue
		}

		// Strip the first path component if every entry shares
		// the same prefix (common for WoW addon zips).
		dst := filepath.Join(destDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(dst)+string(filepath.Separator), cleaned) {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("addon: mkdir %s: %w", filepath.Dir(dst), err)
		}
		out, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("addon: create %s: %w", dst, err)
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return fmt.Errorf("addon: open %s in zip: %w", rel, err)
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return fmt.Errorf("addon: extract %s: %w", rel, err)
		}
	}
	return nil
}

// UnpackReleaseZip extracts a release zip and moves addon dirs
// from the extracted content into addonsRoot, overwriting old copies.
// knownSubDirs lists sub-module names from the config that should
// always be cleaned. Returns the list of addon names now present in
// addonsRoot.
func UnpackReleaseZip(addonsRoot string, r io.ReaderAt, size int64, knownSubDirs []string) ([]string, error) {
	tmpDir, err := os.MkdirTemp("", "lazyaddons-release-*")
	if err != nil {
		return nil, fmt.Errorf("addon: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := ExtractReleaseZip(tmpDir, r, size); err != nil {
		return nil, err
	}

	// Find all addon directories (dirs with a matching .toc file).
	addonDirs := ScanTOCSubdirs(tmpDir)

	// Also check if the root itself is a flat addon.
	if repoDirHasTOC(tmpDir) {
		name := tocAddonName(tmpDir)
		if name == "" {
			name = filepath.Base(tmpDir)
		}
		addonDirs = append(addonDirs, name)
	}

	// Merge with known sub-dirs for cleanup.
	merged := make(map[string]bool)
	for _, s := range addonDirs {
		merged[s] = true
	}
	for _, s := range knownSubDirs {
		merged[s] = true
	}
	// Also always clean the main addon name derived from the URL.
	// The caller should pass it via knownSubDirs.

	// Delete old unpacked dirs from AddOns root.
	for s := range merged {
		_ = os.RemoveAll(filepath.Join(addonsRoot, s))
	}

	// Move addon dirs from temp to AddOns root.
	var promoted []string
	for _, s := range addonDirs {
		src := filepath.Join(tmpDir, s)
		dst := filepath.Join(addonsRoot, s)
		_ = os.RemoveAll(dst)
		if err := moveDir(src, dst); err != nil {
			continue
		}
		promoted = append(promoted, s)
	}

	// Flat addon: move root-level files.
	if len(addonDirs) == 0 && repoDirHasTOC(tmpDir) {
		name := tocAddonName(tmpDir)
		if name == "" {
			name = filepath.Base(tmpDir)
		}
		entries, _ := os.ReadDir(tmpDir)
		destDir := filepath.Join(addonsRoot, name)
		_ = os.MkdirAll(destDir, 0o755)
		for _, e := range entries {
			src := filepath.Join(tmpDir, e.Name())
			dst := filepath.Join(destDir, e.Name())
			_ = moveFile(src, dst)
		}
		promoted = append(promoted, name)
	}

	return promoted, nil
}
