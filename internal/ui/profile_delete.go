package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// viewProfileDelete renders the delete confirmation screen.
// If the targeted profile is the active one, a warning is
// shown above the prompt to pre-empt the rejection on enter.
func viewProfileDelete(m *Model) string {
	var b strings.Builder
	target := m.Config.FindProfileByID(m.PendingProfileID)
	targetName := "<unknown>"
	if target != nil {
		targetName = target.Name
	}
	b.WriteString(titleStyle.Render(" Delete Profile "))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf(
		"Are you sure you want to delete the profile %q?\nThis removes its tracked addons list. Your addon folders on disk are not touched.",
		targetName,
	))
	b.WriteString("\n\n")

	if m.ActiveProfile != nil && m.ActiveProfile.ID == m.PendingProfileID {
		b.WriteString(errorStyle.Render(
			"Cannot delete the active profile. Switch to another profile first."))
		b.WriteString("\n\n")
	}

	if m.ProfileError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.ProfileError))
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("y/enter delete • n/esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// updateProfileDelete handles key events on the delete
// confirmation screen. y/enter confirms (and is rejected if
// the target is the active profile); n/esc returns to the
// picker.
func updateProfileDelete(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "Y", "enter":
		return submitProfileDelete(m), nil
	case "n", "N", "esc":
		m.PendingProfileID = ""
		m.ProfileError = ""
		m.Screen = screenProfilePicker
		return *m, nil
	}
	return *m, nil
}

// submitProfileDelete removes the profile with id
// m.PendingProfileID via m.Config.RemoveProfile. The active
// profile is the only deletion that fails; the error is
// surfaced via m.ProfileError and the screen stays put.
func submitProfileDelete(m *Model) Model {
	if m.PendingProfileID == "" {
		m.ProfileError = "internal error: no profile selected for delete"
		return *m
	}
	// Belt-and-braces guard: the spec says deleting the
	// active profile is forbidden, so we surface a friendly
	// error even if a future caller forgets the guard.
	if m.ActiveProfile != nil && m.ActiveProfile.ID == m.PendingProfileID {
		m.ProfileError = "Cannot delete the active profile. Switch to another profile first."
		return *m
	}
	target := m.Config.FindProfileByID(m.PendingProfileID)
	if target == nil {
		m.ProfileError = "Profile not found"
		return *m
	}
	if err := m.Config.RemoveProfile(m.PendingProfileID); err != nil {
		m.ProfileError = fmt.Sprintf("Delete failed: %v", err)
		return *m
	}
	// CRITICAL: RemoveProfile compacts Profiles in place via
	// `append(c.Profiles[:i], c.Profiles[i+1:]...)`, which
	// shifts every element at index > i down by one. If the
	// deleted profile's index was lower than the active
	// profile's, the active profile's bytes are now at
	// index-1, and m.ActiveProfile (a *config.Profile
	// pointer into the backing array) is left pointing at
	// either the WRONG profile's struct or at memory the new
	// slice doesn't include. Subsequent mutations through
	// m.ActiveProfile (UpsertAddon, RemoveAddon, status
	// updates) would then silently corrupt the wrong profile.
	// Always rewire by looking up the active ID again.
	if m.ActiveProfile != nil {
		m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
	}
	m.PendingProfileID = ""
	m.ProfileError = ""
	// Clamp the cursor to the new list length.
	if m.ProfileCursor >= len(m.Config.Profiles) {
		m.ProfileCursor = len(m.Config.Profiles) - 1
	}
	if m.ProfileCursor < 0 {
		m.ProfileCursor = 0
	}
	// Keep the screen on the picker (spec) so the user can
	// perform another action without going back through the
	// list.
	m.Screen = screenProfilePicker
	return *m
}
