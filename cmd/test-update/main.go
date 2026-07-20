// Command test-update launches the TUI with a fake update banner
// for visual testing. The real GitHub API is never called.
//
// Usage:
//
//	go run ./cmd/test-update/
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/app"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/ui"
	"github.com/pentsec/lazyaddons/internal/update"
)

func main() {
	// Simulate an older version running.
	app.Version = "1.0.0"
	fmt.Fprintf(os.Stderr, "Simulating: lazyaddons v1.0.0 running, v2.5.0 available on GitHub\n")
	fmt.Fprintf(os.Stderr, "Press U to trigger self-update (will fail offline)\n")

	cfg := config.Default()
	cfg.Profiles = []config.Profile{
		{
			ID:      "demo-profile-id",
			Name:    "Demo",
			WoWPath: "/tmp/wow/Interface/AddOns",
			Addons: []config.Addon{
				{Name: "Atlas", TrackMode: "branch", TrackTarget: "main"},
				{Name: "Bagnon", TrackMode: "release", TrackTarget: "v1.0.0"},
				{Name: "Details", TrackMode: "branch", TrackTarget: "main"},
			},
		},
	}
	cfg.ActiveProfileID = "demo-profile-id"

	model := ui.NewModel()
	model.Config = cfg
	model.SetActiveProfile(cfg.FindProfileByID(cfg.ActiveProfileID))
	model.UpdateBanner = &update.CheckResult{
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.5.0",
		LatestURL:       "https://github.com/pentsec/lazyaddons/releases/v2.5.0",
	}

	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "test-update: %v\n", err)
		os.Exit(1)
	}
}
