// Package addon — directory promotion and update unpacking.
package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pentsec/lazyaddons/internal/backup"
)

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

	// Park the clone under .lazyaddons/ so .git has a permanent
	// home independent of the unpacked addon dirs. Both layouts
	// (nested and flat) need the same rename, so it lives here
	// once. A caller may pass a repo that is already parked there
	// — re-adding after an interrupted install — in which case
	// cloneDir and repoDir are the same path and there is nothing
	// to rename.
	lazyDir := filepath.Join(addonsRoot, ".lazyaddons")
	_ = os.MkdirAll(lazyDir, 0o755)
	repoDir := filepath.Join(lazyDir, cloneBase)
	if filepath.Clean(cloneDir) != filepath.Clean(repoDir) {
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
			// The folder already exists — almost always a manual
			// install that was never tracked. Back it up, then
			// swap in the fresh copy so what is on disk matches
			// the repo just cloned. If the swap fails, keep the
			// folder that is there and still report it: a target
			// silently dropped here makes the whole repo look
			// like it has no .toc, and the add fails.
			_ = backup.New(addonsRoot).Create(name)
			_ = os.RemoveAll(dst)
			if _, err := os.Stat(dst); err == nil {
				promoted = append(promoted, name)
				continue
			}
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

// ExistingTargets reports which addon folders PromoteAddonDirs would
// have to overwrite when unpacking repoDir: every addon the
// repository installs — a sub-directory with a matching .toc, or the
// repository root itself for a flat addon — whose folder already
// exists in addonsRoot. Callers show this list to the user before
// touching those folders.
func ExistingTargets(addonsRoot, repoDir string) []string {
	cloneBase := filepath.Base(repoDir)
	subAddons := ScanTOCSubdirs(repoDir)
	mainIsNested := hasSubAddon(subAddons, cloneBase)

	// A folder that is the clone itself is not an existing install:
	// a nested main addon named after the repository, or a flat
	// addon, unpacks onto the very path the clone sits in.
	isInstalled := func(name string) bool {
		dst := filepath.Join(addonsRoot, name)
		if filepath.Clean(dst) == filepath.Clean(repoDir) {
			return false
		}
		return isDir(dst)
	}

	var targets []string
	for _, name := range subAddons {
		if isInstalled(name) {
			targets = append(targets, name)
		}
	}
	if !mainIsNested && repoDirHasTOC(repoDir) {
		actualName := cloneBase
		if tocName := tocAddonName(repoDir); tocName != "" {
			actualName = tocName
		}
		if isInstalled(actualName) {
			targets = append(targets, actualName)
		}
	}
	return targets
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// UnpackUpdate moves addon directories from a git repo (already at
// .lazyaddons/<name>) to the AddOns root. It is the update-path
// counterpart to PromoteAddonDirs: it skips the install-only rename
// step and force-overwrites old unpacked directories so the freshly
// pulled files always land in the right place.
//
// knownSubDirs lists addon directory names previously unpacked for
// this addon (from the config's sub_modules field). These are
// ALWAYS cleaned from addonsRoot regardless of what the repo
// currently contains, preventing orphaned directories when the
// repo structure changes or ResetWorkingTree fails.
func UnpackUpdate(addonsRoot, repoDir string, knownSubDirs []string) {
	cloneBase := filepath.Base(repoDir)
	subAddons := ScanTOCSubdirs(repoDir)
	mainIsNested := hasSubAddon(subAddons, cloneBase)

	// Merge known sub-dirs from config with what the repo
	// currently contains. This ensures cleanup even when the
	// repo is empty or has changed structure.
	merged := make(map[string]bool)
	for _, s := range subAddons {
		merged[s] = true
	}
	for _, s := range knownSubDirs {
		merged[s] = true
	}
	allSubs := make([]string, 0, len(merged))
	for s := range merged {
		allSubs = append(allSubs, s)
	}

	// Delete old unpacked dirs from AddOns root.
	for _, s := range allSubs {
		_ = os.RemoveAll(filepath.Join(addonsRoot, s))
	}
	_ = os.RemoveAll(filepath.Join(addonsRoot, cloneBase))

	// Move subdirectories from repo to AddOns root.
	for _, s := range allSubs {
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
