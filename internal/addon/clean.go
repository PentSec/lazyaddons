// Package addon — repository junk file cleanup.
package addon

import (
	"os"
	"path/filepath"
	"strings"
)

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
