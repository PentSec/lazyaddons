package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/config"
	gh "github.com/pentsec/lazyaddons/internal/github"
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
func applyAddonCmd(addonsRoot string, a config.Addon, ghClient *gh.Client, cfg *config.Config) tea.Cmd {
	return applyUpdatesCmd(addonsRoot, []config.Addon{a}, ghClient, cfg)
}

// applyUpdatesCmd applies updates for each addon. Branch-tracked
// addons use Pull; release-tracked addons download the release zip
// asset from GitHub. All addons get UnpackUpdate or UnpackReleaseZip.
// On success for release-tracked addons, the config is updated with
// the new version tag and saved.
func applyUpdatesCmd(addonsRoot string, addons []config.Addon, ghClient *gh.Client, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		var updated []string
		for _, a := range addons {
			repoDir := findRepoDir(addonsRoot, a.Name)

			if a.TrackMode == addon.TrackModeRelease {
				// Release mode: download zip asset from GitHub.
				if err := applyReleaseUpdate(addonsRoot, &a, ghClient); err != nil {
					continue
				}
				// Persist the new version in config.
				if cfg != nil {
					for i := range cfg.Addons {
						if cfg.Addons[i].Name == a.Name {
							cfg.Addons[i].TrackTarget = a.TrackTarget
							break
						}
					}
					_ = config.Save(cfg)
				}
			} else {
				// Branch mode: git pull + re-unpack.
				if repoDir == "" {
					continue
				}
				_ = gitops.ResetWorkingTree(repoDir)
				_ = gitops.Fetch(repoDir)
				if err := gitops.Pull(repoDir); err != nil {
					if branch := gitops.DefaultBranch(repoDir); branch != "" {
						_ = gitops.MergeFF(repoDir, "refs/remotes/origin/"+branch)
					}
				}
				_ = gitops.ResetWorkingTree(repoDir)
				addon.UnpackUpdate(addonsRoot, repoDir, a.SubModules)
			}
			updated = append(updated, a.Name)
		}
		return updateAppliedMsg{Updated: updated}
	}
}

func applyOneUpdate(addonsRoot string, a config.Addon, ghClient *gh.Client, cfg *config.Config, step, total int) tea.Cmd {
	return tea.Sequence(
		func() tea.Msg {
			return progressStepMsg{
				Label: fmt.Sprintf("Updating %s...", a.Name),
				Step:  step,
				Total: total,
			}
		},
		func() tea.Msg {
			repoDir := findRepoDir(addonsRoot, a.Name)

			if a.TrackMode == addon.TrackModeRelease {
				if err := applyReleaseUpdate(addonsRoot, &a, ghClient); err != nil {
					return updateAppliedMsg{Err: fmt.Errorf("%s: %w", a.Name, err)}
				}
				if cfg != nil {
					for i := range cfg.Addons {
						if cfg.Addons[i].Name == a.Name {
							cfg.Addons[i].TrackTarget = a.TrackTarget
							break
						}
					}
					_ = config.Save(cfg)
				}
			} else {
				if repoDir == "" {
					return updateAppliedMsg{Err: fmt.Errorf("%s: repo not found", a.Name)}
				}
				_ = gitops.ResetWorkingTree(repoDir)
				_ = gitops.Fetch(repoDir)
				if err := gitops.Pull(repoDir); err != nil {
					if branch := gitops.DefaultBranch(repoDir); branch != "" {
						_ = gitops.MergeFF(repoDir, "refs/remotes/origin/"+branch)
					}
				}
				_ = gitops.ResetWorkingTree(repoDir)
				addon.UnpackUpdate(addonsRoot, repoDir, a.SubModules)
			}
			return updateAppliedMsg{Updated: []string{a.Name}}
		},
	)
}

func applyUpdatesWithProgress(addonsRoot string, addons []config.Addon, ghClient *gh.Client, cfg *config.Config) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(addons))
	for i, a := range addons {
		cmds = append(cmds, applyOneUpdate(addonsRoot, a, ghClient, cfg, i+1, len(addons)))
	}
	return tea.Sequence(cmds...)
}

// applyReleaseUpdate downloads the release zip for an addon and
// replaces the addon dirs in AddOns with the zip contents.
func applyReleaseUpdate(addonsRoot string, a *config.Addon, ghClient *gh.Client) error {
	if ghClient == nil {
		return fmt.Errorf("github client not available")
	}
	owner, repo, err := gh.ParseOwnerRepo(a.URL)
	if err != nil {
		return err
	}
	// Find the latest tag newer than the current version.
	// a.TrackTarget is the installed version (stale); we need the
	// actual target tag to download the right release zip.
	repoDir := findRepoDir(addonsRoot, a.Name)
	targetTag := a.TrackTarget
	if repoDir != "" {
		if latest, _ := gitops.LatestNewTag(repoDir, a.TrackTarget); latest != "" {
			targetTag = latest
		}
	}
	rel, err := ghClient.ReleaseForTag(owner, repo, targetTag)
	if err != nil {
		return err
	}
	if rel == nil {
		return fmt.Errorf("release %s not found", targetTag)
	}
	zipAsset := rel.FindZipAsset()
	if zipAsset == nil {
		return fmt.Errorf("no zip asset in release %s", targetTag)
	}

	// Download zip into memory (WoW addon zips are small).
	var buf bytes.Buffer
	if _, err := ghClient.DownloadAsset(zipAsset.BrowserURL, &buf); err != nil {
		return err
	}

	// Ensure the main addon name is always cleaned.
	known := append([]string{a.Name}, a.SubModules...)
	promoted, err := addon.UnpackReleaseZip(addonsRoot, bytes.NewReader(buf.Bytes()), int64(buf.Len()), known)
	if err != nil {
		return err
	}
	if len(promoted) == 0 {
		return fmt.Errorf("no valid .toc found in release zip")
	}
	// Update the tracked version in config so the next check
	// starts from the newly installed release.
	a.TrackTarget = targetTag
	return nil
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
