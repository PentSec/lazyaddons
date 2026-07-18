// Command lazyaddons is the TUI for managing World of Warcraft
// addons via git.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

	// Resolve the WoW path. If the config has one, use it;
	// otherwise fall back to auto-detection.
	var path wowpath.Path
	if cfg.WoWPath != "" {
		p, err := wowpath.Resolve(cfg.WoWPath)
		if err == nil {
			path = p
		}
	}
	if path == "" {
		p, err := wowpath.Resolve("")
		if err == nil {
			path = p
		}
	}

	// Remove addons that no longer exist on disk (user deleted
	// them manually outside the TUI).
		if path != "" {
		pruneMissingAddons(cfg, string(path))
		scanExistingRepos(cfg, string(path))
		scanGitClones(cfg, string(path))
	}

	model := ui.NewModel()
	model.Config = cfg
	model.WowPath = path
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
func pruneMissingAddons(cfg *config.Config, addonsRoot string) {
	filtered := cfg.Addons[:0]
	for _, a := range cfg.Addons {
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
	cfg.Addons = filtered
}

// scanExistingRepos detects addons that were previously managed
// by the tool but aren't yet in the config. It checks both the
// legacy <name>.repo pattern and the new .lazyaddons/<name> dir.
func scanExistingRepos(cfg *config.Config, addonsRoot string) {
	// Scan legacy .repo dirs at AddOns root.
	entries, err := os.ReadDir(addonsRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".repo") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".repo")
			if cfg.AddonByName(name) != nil {
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
		cfg.Addons = append(cfg.Addons, config.Addon{
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
		if cfg.AddonByName(name) != nil {
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
		cfg.Addons = append(cfg.Addons, config.Addon{
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
	cmd := exec.Command("git", "-C", repoDir, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
func scanGitClones(cfg *config.Config, addonsRoot string) {
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
		if cfg.AddonByName(name) != nil {
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
		cfg.Addons = append(cfg.Addons, config.Addon{
			Name:        name,
			URL:         url,
			TrackMode:   "branch",
			TrackTarget: "main",
		})
	}
}
