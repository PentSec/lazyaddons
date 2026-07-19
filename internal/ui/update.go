package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/gitops"
)

// updatesCheckedMsg is posted after checking all tracked addons.
type updatesCheckedMsg struct {
	Statuses map[string]AddonStatus
	Err      error
}

// updateAppliedMsg is posted after applying updates to addons.
type updateAppliedMsg struct {
	Updated []string
	Err     error
}

// checkAllUpdatesCmd checks every tracked addon for updates.
// Branch-tracked addons use IsBehind; release-tracked addons use
// LatestNewTag to detect new version tags.
func checkAllUpdatesCmd(addonsRoot string, addons []config.Addon) tea.Cmd {
	return func() tea.Msg {
		statuses := make(map[string]AddonStatus, len(addons))
		for _, a := range addons {
			repoDir := findRepoDir(addonsRoot, a.Name)
			if repoDir == "" {
				statuses[a.Name] = StatusOK // no repo yet, treat as OK
				continue
			}
			// Fetch may fail if offline — don't mark as error.
			_ = gitops.Fetch(repoDir)

			if a.TrackMode == addon.TrackModeRelease {
				newTag, _ := gitops.LatestNewTag(repoDir, a.TrackTarget)
				if newTag != "" {
					statuses[a.Name] = StatusUpdate
				} else {
					statuses[a.Name] = StatusOK
				}
			} else {
				behind, _ := isBehind(repoDir)
				if behind {
					statuses[a.Name] = StatusUpdate
				} else {
					statuses[a.Name] = StatusOK
				}
			}
		}
		return updatesCheckedMsg{Statuses: statuses}
	}
}

// applyAddonCmd applies the update for a single addon.
func applyAddonCmd(addonsRoot string, a config.Addon) tea.Cmd {
	return applyUpdatesCmd(addonsRoot, []config.Addon{a})
}

// applyUpdatesCmd applies updates for each addon. Branch-tracked
// addons use Pull; release-tracked addons use CheckoutTag. All
// addons get ResetWorkingTree + UnpackUpdate.
func applyUpdatesCmd(addonsRoot string, addons []config.Addon) tea.Cmd {
	return func() tea.Msg {
		var updated []string
		for _, a := range addons {
			repoDir := findRepoDir(addonsRoot, a.Name)
			if repoDir == "" {
				continue
			}

			// Restore working tree before any remote operation.
			_ = gitops.ResetWorkingTree(repoDir)
			_ = gitops.Fetch(repoDir)

			if a.TrackMode == addon.TrackModeRelease {
				// Release mode: find the new tag and checkout.
				newTag, err := gitops.LatestNewTag(repoDir, a.TrackTarget)
				if err != nil || newTag == "" {
					continue
				}
				_ = gitops.CheckoutTag(repoDir, newTag)
			} else {
				// Branch mode: fast-forward to latest.
				if err := gitops.Pull(repoDir); err != nil {
					if branch := gitops.DefaultBranch(repoDir); branch != "" {
						_ = gitops.MergeFF(repoDir, "refs/remotes/origin/"+branch)
					}
				}
			}

			// Sync working tree to the new HEAD.
			_ = gitops.ResetWorkingTree(repoDir)

			// Re-unpack: delete old dirs + move fresh copies to AddOns.
			addon.UnpackUpdate(addonsRoot, repoDir, a.SubModules)
			updated = append(updated, a.Name)
		}
		return updateAppliedMsg{Updated: updated}
	}
}

// isBehind returns true if the repo's HEAD is behind its upstream
// or remote tracking branch.
func isBehind(dir string) (bool, error) {
	return gitops.IsBehind(dir)
}

// findRepoDir returns the git repo directory for an addon.
// Checks the new .lazyaddons/<name> location first, then falls
// back to the legacy <name>.repo location.
func findRepoDir(addonsRoot, name string) string {
	// New location: .lazyaddons/<name>
	repoDir := filepath.Join(addonsRoot, ".lazyaddons", name)
	if st, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil && st.IsDir() {
		return repoDir
	}
	// Legacy location: <name>.repo
	oldDir := filepath.Join(addonsRoot, name+".repo")
	if st, err := os.Stat(filepath.Join(oldDir, ".git")); err == nil && st.IsDir() {
		return oldDir
	}
	// Flat clone: <name>/.git
	dir := filepath.Join(addonsRoot, name)
	if st, err := os.Stat(filepath.Join(dir, ".git")); err == nil && st.IsDir() {
		return dir
	}
	return ""
}

// lastCommitDate returns the date of the latest commit in YYYY-MM-DD
// format.
func lastCommitDate(repoDir string) string {
	return gitops.LastCommitDate(repoDir)
}

// readTOCVersion reads the ## Version field from the addon's .toc file.
func readTOCVersion(addonsRoot, name string) string {
	tocPath := filepath.Join(addonsRoot, name, name+".toc")
	data, err := os.ReadFile(tocPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## Version:") || strings.HasPrefix(line, "## Version ") {
			v := strings.TrimPrefix(line, "## Version:")
			v = strings.TrimPrefix(v, "## Version ")
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// refreshAddonMeta updates the Version and LastUpdated fields for an
// addon in the config.
func (m *Model) refreshAddonMeta(name string) {
	repoDir := findRepoDir(string(m.WowPath), name)
	if repoDir == "" {
		return
	}
	a := m.Config.AddonByName(name)
	if a == nil {
		return
	}
	a.Version = readTOCVersion(string(m.WowPath), name)
	a.LastUpdated = lastCommitDate(repoDir)
}

// handleUpdatesChecked processes update check results. It only
// updates the status badges — it does NOT auto-apply updates.
func (m *Model) handleUpdatesChecked(msg updatesCheckedMsg) tea.Cmd {
	if msg.Err != nil {
		m.ErrMessage = fmt.Sprintf("Update check failed: %v", msg.Err)
		m.Screen = screenError
		return nil
	}
	for name, status := range msg.Statuses {
		m.Statuses[name] = status
		m.refreshAddonMeta(name)
	}
	m.Screen = screenList
	return nil
}

// handleUpdateApplied processes the result of applying updates.
func (m *Model) handleUpdateApplied(msg updateAppliedMsg) {
	if msg.Err != nil {
		m.ErrMessage = fmt.Sprintf("Update failed: %v", msg.Err)
		m.Screen = screenError
		return
	}
	for _, name := range msg.Updated {
		m.Statuses[name] = StatusOK
		m.refreshAddonMeta(name)
	}
	m.Screen = screenList
}
