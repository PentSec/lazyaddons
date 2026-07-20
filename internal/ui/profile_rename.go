package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// viewProfileRename renders the rename-input screen. The
// pre-filled current name is shown so the user can edit in
// place; errors are shown in red below the input.
func viewProfileRename(m *Model) string {
	var b strings.Builder
	target := m.Config.FindProfileByID(m.PendingProfileID)
	targetName := "<unknown>"
	if target != nil {
		targetName = target.Name
	}
	b.WriteString(titleStyle.Render(" Rename Profile "))
	b.WriteString("\n\n")
	b.WriteString("Current name: " + targetName)
	b.WriteString("\n\n")
	b.WriteString("New name (1-64 chars, must be unique):\n\n")
	b.WriteString("> " + m.PendingProfileName)
	b.WriteString("\n\n")

	if m.ProfileNameError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.ProfileNameError))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("enter save • esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// updateProfileRename handles key events on the rename
// screen. The new name is validated on enter and persisted via
// m.Config.RenameProfile.
func updateProfileRename(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.PendingProfileID = ""
		m.PendingProfileName = ""
		m.ProfileNameError = ""
		m.Screen = screenProfilePicker
		return *m, nil
	case "backspace":
		if len(m.PendingProfileName) > 0 {
			m.PendingProfileName = m.PendingProfileName[:len(m.PendingProfileName)-1]
		}
		m.ProfileNameError = ""
		return *m, nil
	case "enter":
		return submitProfileRename(m), nil
	}

	for _, r := range key.Runes {
		if r >= 32 && r < 127 {
			m.PendingProfileName += string(r)
			m.ProfileNameError = ""
		}
	}
	return *m, nil
}

// submitProfileRename validates the new name and, on success,
// calls m.Config.RenameProfile to persist the change. The
// user is returned to the picker on success; on error the
// screen stays put with the error shown.
func submitProfileRename(m *Model) Model {
	name := strings.TrimSpace(m.PendingProfileName)
	if err := validateProfileName(name); err != nil {
		m.ProfileNameError = err.Error()
		return *m
	}
	if m.PendingProfileID == "" {
		m.ProfileNameError = "internal error: no profile selected for rename"
		return *m
	}
	if err := m.Config.RenameProfile(m.PendingProfileID, name); err != nil {
		m.ProfileNameError = fmt.Sprintf("Rename failed: %v", err)
		return *m
	}
	// Defensive rewire: RenameProfile currently mutates the
	// struct in place, so the existing pointer stays valid.
	// However, any future change that "replaces" the struct
	// (delete-and-reinsert, reorder by name, etc.) would
	// silently invalidate m.ActiveProfile. Re-look up by ID
	// after every profile-list mutation so we never observe
	// a stale pointer.
	if m.ActiveProfile != nil {
		m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
	}
	m.PendingProfileID = ""
	m.PendingProfileName = name
	m.ProfileNameError = ""
	m.Screen = screenProfilePicker
	return *m
}
