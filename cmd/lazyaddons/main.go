// Command lazyaddons is the TUI for managing World of Warcraft
// addons via git.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/app"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/gitops"
	"github.com/pentsec/lazyaddons/internal/ui"
	"github.com/pentsec/lazyaddons/internal/update"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

// version is set by GoReleaser via ldflags at build time.
// Default "dev" means a local build — self-update is skipped.
var version = "dev"

func main() {
	// Handle --version before anything else.
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		app.Version = app.ResolveVersion(version)
		fmt.Printf("lazyaddons %s\n", app.Version)
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "lazyaddons: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Resolve the application version before anything else.
	app.Version = app.ResolveVersion(version)

	cfg, err := config.Load()
	if err != nil {
		// ErrNotFound is expected on first run — we just start
		// with a Default config. Other errors are fatal.
		if !errors.Is(err, config.ErrNotFound) {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = config.Default()
	}

	// First-run / no-profiles case: hand off to the UI on the
	// profile-add screen so the user must create one before
	// anything else. The save on exit persists the new profile.
	if len(cfg.Profiles) == 0 {
		model := ui.NewModel()
		model.Config = cfg
		model.Screen = model.StartScreen() // screenProfileAdd

		if result := update.CheckLatest(app.Version); result != nil {
			if result.UpdateAvailable {
				model.UpdateBanner = result
			}
		}

		prog := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := prog.Run(); err != nil {
			return fmt.Errorf("run program: %w", err)
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	}

	// Existing profiles: resolve the active one and scope all
	// path-consuming bootstrap work to it. The active profile's
	// path drives prune + scan; m.WoWPath is kept in sync via
	// setActiveProfile so existing code that reads m.WoWPath
	// works unchanged.
	active := cfg.FindProfileByID(cfg.ActiveProfileID)
	if active == nil {
		// Validate() should have reset a stale ActiveProfileID;
		// this is a defensive fallback.
		active = &cfg.Profiles[0]
		cfg.ActiveProfileID = active.ID
	}

	var path wowpath.Path
	if active.WoWPath != "" {
		if p, err := wowpath.Resolve(active.WoWPath); err == nil {
			path = p
		}
	}
	if path == "" {
		// Fall back to auto-detection only when the profile has
		// no stored path (shouldn't normally happen post-T1, but
		// keeps the bootstrap robust for hand-edited configs).
		if p, err := wowpath.Resolve(""); err == nil {
			path = p
		}
	}
	if path != "" && active.WoWPath == "" {
		active.WoWPath = string(path)
	}

	// Remove addons that no longer exist on disk (user deleted
	// them manually outside the TUI) and re-discover any that
	// are on-disk but not tracked. Both are scoped to the active
	// profile's path.
	if path != "" {
		pruneMissingAddons(active, string(path))
		scanExistingRepos(active, string(path))
		scanGitClones(active, string(path))
	}

	model := ui.NewModel()
	model.Config = cfg
	model.SetActiveProfile(active)
	model.Screen = model.StartScreen()

	// Check for self-updates on startup (non-blocking).
	// A nil result means dev build — nothing to compare.
	if result := update.CheckLatest(app.Version); result != nil {
		if result.UpdateAvailable {
			model.UpdateBanner = result
		}
	}

	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("run program: %w", err)
	}

	// On exit, persist any config changes the UI made.
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// pruneMissingAddons removes tracked addons whose folders no
// longer exist on disk (neither the addon dir nor the repo dir).
// Operates on a single profile's addon list.
func pruneMissingAddons(p *config.Profile, addonsRoot string) {
	if p == nil {
		return
	}
	filtered := p.Addons[:0]
	for _, a := range p.Addons {
		addonDir := filepath.Join(addonsRoot, a.Name)
		newRepoDir := filepath.Join(addonsRoot, ".lazyaddons", a.Name)
		oldRepoDir := filepath.Join(addonsRoot, a.Name+".repo")
		_, errAddon := os.Stat(addonDir)
		_, errNewRepo := os.Stat(newRepoDir)
		_, errOldRepo := os.Stat(oldRepoDir)
		if errAddon == nil || errNewRepo == nil || errOldRepo == nil {
			filtered = append(filtered, a)
		}
	}
	p.Addons = filtered
}

// scanExistingRepos detects addons that were previously managed
// by the tool but aren't yet in the profile's tracked list. It
// checks both the legacy <name>.repo pattern and the new
// .lazyaddons/<name> dir.
func scanExistingRepos(p *config.Profile, addonsRoot string) {
	if p == nil {
		return
	}
	// Scan legacy .repo dirs at AddOns root.
	entries, err := os.ReadDir(addonsRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".repo") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".repo")
			if p.AddonByName(name) != nil {
				continue
			}
			addonDir := filepath.Join(addonsRoot, name)
			if _, err := os.Stat(addonDir); os.IsNotExist(err) {
				continue
			}
			repoDir := filepath.Join(addonsRoot, e.Name())
			url := getRemoteURL(repoDir)
			if url == "" {
				continue
			}
			p.Addons = append(p.Addons, config.Addon{
				Name:        name,
				URL:         url,
				TrackMode:   "branch",
				TrackTarget: detectDefaultBranch(filepath.Join(addonsRoot, name)),
			})
		}
	}

	// Scan .lazyaddons/ subfolder (new style).
	lazyDir := filepath.Join(addonsRoot, ".lazyaddons")
	lazyEntries, err := os.ReadDir(lazyDir)
	if err != nil {
		return
	}
	for _, e := range lazyEntries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if p.AddonByName(name) != nil {
			continue
		}
		repoDir := filepath.Join(lazyDir, name)
		url := getRemoteURL(repoDir)
		if url == "" {
			continue
		}
		// Also verify the unpacked addon folder exists.
		addonDir := filepath.Join(addonsRoot, name)
		if _, err := os.Stat(addonDir); os.IsNotExist(err) {
			continue
		}
		p.Addons = append(p.Addons, config.Addon{
			Name:        name,
			URL:         url,
			TrackMode:   "branch",
			TrackTarget: "main",
		})
	}
}

// getRemoteURL returns the origin URL of a git repo, or "" on
// failure.
func getRemoteURL(repoDir string) string {
	url, err := gitops.RemoteURL(repoDir)
	if err != nil {
		return ""
	}
	return url
}

// detectDefaultBranch returns the repo's default branch name by
// reading the remote HEAD symref. Falls back to "main" if
// detection fails.
func detectDefaultBranch(repoDir string) string {
	if branch := gitops.DefaultBranch(repoDir); branch != "" {
		return branch
	}
	return "main"
}

// scanGitClones detects addons that were manually cloned with git
// (have .git/ inside the addon folder, not .repo/).
func scanGitClones(p *config.Profile, addonsRoot string) {
	if p == nil {
		return
	}
	entries, err := os.ReadDir(addonsRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip repo dirs (handled by scanExistingRepos).
		if strings.HasSuffix(name, ".repo") || name == ".lazyaddons" {
			continue
		}
		if p.AddonByName(name) != nil {
			continue // already tracked
		}
		gitDir := filepath.Join(addonsRoot, name, ".git")
		st, err := os.Stat(gitDir)
		if err != nil || !st.IsDir() {
			continue
		}
		// Verify it has a .toc file (it's a WoW addon).
		tocFile := filepath.Join(addonsRoot, name, name+".toc")
		if _, err := os.Stat(tocFile); os.IsNotExist(err) {
			continue
		}
		url := getRemoteURL(filepath.Join(addonsRoot, name))
		if url == "" {
			continue
		}
		p.Addons = append(p.Addons, config.Addon{
			Name:        name,
			URL:         url,
			TrackMode:   "branch",
			TrackTarget: "main",
		})
	}
}
