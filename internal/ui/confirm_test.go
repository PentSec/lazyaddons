package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

// confirmModel returns a model on the addon list with one addon
// whose status is StatusUpdate, ready for update/remove flow tests.
func confirmModel(t *testing.T, status AddonStatus) *Model {
	t.Helper()
	m := newModelWithProfiles(t, config.Profile{
		Name:    "Retail",
		WoWPath: "/tmp/wow/Interface/AddOns",
		Addons:  []config.Addon{{Name: "Atlas", Version: "v1.0.0"}},
	})
	m.Statuses = map[string]AddonStatus{"Atlas": status}
	return m
}

// TestUpdate_EnterStartsProgress verifies that enter on an addon
// with a pending update updates it directly (progress screen).
func TestUpdate_EnterStartsProgress(t *testing.T) {
	t.Parallel()
	m := confirmModel(t, StatusUpdate)
	mm, _ := updateList(m, tea.KeyMsg{Type: tea.KeyEnter})
	if mm.(Model).Screen != screenProgress {
		t.Errorf("Screen = %v, want progress", mm.(Model).Screen)
	}
	if got := mm.(Model).ProgressLabel; got != "Updating Atlas..." {
		t.Errorf("ProgressLabel = %q, want %q", got, "Updating Atlas...")
	}
}

// TestUpdate_UStartsProgress verifies the u shortcut updates the
// selected addon directly when it has a pending update.
func TestUpdate_UStartsProgress(t *testing.T) {
	t.Parallel()
	m := confirmModel(t, StatusUpdate)
	mm, _ := updateList(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if mm.(Model).Screen != screenProgress {
		t.Errorf("Screen = %v, want progress", mm.(Model).Screen)
	}
	if got := mm.(Model).ProgressLabel; got != "Updating Atlas..." {
		t.Errorf("ProgressLabel = %q, want %q", got, "Updating Atlas...")
	}
}

// TestUpdate_UWithoutUpdateChecksAll verifies that u with no pending
// update on the selection runs the check-all flow.
func TestUpdate_UWithoutUpdateChecksAll(t *testing.T) {
	t.Parallel()
	m := confirmModel(t, StatusOK)
	mm, _ := updateList(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if mm.(Model).Screen != screenProgress {
		t.Errorf("Screen = %v, want progress", mm.(Model).Screen)
	}
	if got := mm.(Model).ProgressLabel; got != "Checking for updates..." {
		t.Errorf("ProgressLabel = %q, want %q", got, "Checking for updates...")
	}
}

// TestRemove_DOpensConfirmModal verifies d opens the remove modal
// with the selected addon pre-set.
func TestRemove_DOpensConfirmModal(t *testing.T) {
	t.Parallel()
	m := confirmModel(t, StatusOK)
	mm, _ := updateList(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if mm.(Model).Screen != screenConfirmRemove {
		t.Errorf("Screen = %v, want confirmRemove", mm.(Model).Screen)
	}
	if got := mm.(Model).PendingRemove; got != "Atlas" {
		t.Errorf("PendingRemove = %q, want Atlas", got)
	}
}
