package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/addon"
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
func checkAllUpdatesCmd(addonsRoot string, names []string) tea.Cmd {
	return func() tea.Msg {
		statuses := make(map[string]AddonStatus, len(names))
		for _, name := range names {
			repoDir := findRepoDir(addonsRoot, name)
			if repoDir == "" {
				statuses[name] = StatusOK // no repo yet, treat as OK
				continue
			}
			// Fetch may fail if offline — don't mark as error.
			_ = gitops.Fetch(repoDir)
			behind, _ := isBehind(repoDir)
			if behind {
				statuses[name] = StatusUpdate
			} else {
				statuses[name] = StatusOK
			}
		}
		return updatesCheckedMsg{Statuses: statuses}
	}
}

// applyAddonCmd applies the update for a single addon.
func applyAddonCmd(addonsRoot, name string) tea.Cmd {
	return applyUpdatesCmd(addonsRoot, []string{name})
}

// applyUpdatesCmd applies git pull + re-unpack for each behind
// addon. It pulls in the repo then re-unpacks the addon dirs
// into the AddOns root, overwriting old copies.
func applyUpdatesCmd(addonsRoot string, behind []string) tea.Cmd {
	return func() tea.Msg {
		var updated []string
		for _, name := range behind {
			repoDir := findRepoDir(addonsRoot, name)
			if repoDir == "" {
				continue
			}

			// Restore working tree and fast-forward to latest.
			_, _ = gitops.Run(repoDir, "checkout", "--", ".")
			_ = gitops.Fetch(repoDir)
			// Fast-forward using the upstream tracking branch.
			// @{u} works when the branch has a configured
			// upstream (set automatically by git clone).
			if _, err := gitops.Run(repoDir, "merge", "--ff-only", "@{u}"); err != nil {
				// Detached HEAD or no upstream — try origin/<branch>.
				if branch := gitops.DefaultBranch(repoDir); branch != "" {
					_, _ = gitops.Run(repoDir, "merge", "--ff-only", "origin/"+branch)
				}
			}

			// Re-unpack: delete old dirs + move fresh copies to AddOns.
			addon.UnpackUpdate(addonsRoot, repoDir)
			updated = append(updated, name)
		}
		return updateAppliedMsg{Updated: updated}
	}
}

// isBehind returns true if the repo's HEAD is behind its upstream
// or remote tracking branch. Handles detached HEAD (tag checkout)
// by falling back to detecting the default branch.
func isBehind(dir string) (bool, error) {
	out, err := gitops.Run(dir, "rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		out = strings.TrimSpace(out)
		return out != "0" && out != "", nil
	}
	// @{u} failed (detached HEAD or no upstream).
	// Detect the default branch from the remote.
	if branch := gitops.DefaultBranch(dir); branch != "" {
		out, err = gitops.Run(dir, "rev-list", "--count", "HEAD..origin/"+branch)
		if err == nil {
			out = strings.TrimSpace(out)
			return out != "0" && out != "", nil
		}
	}
	return false, nil
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
// format using `git log -1 --format=%cs`.
func lastCommitDate(repoDir string) string {
	out, err := gitops.Run(repoDir, "log", "-1", "--format=%cs")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
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
