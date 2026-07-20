package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

// viewProfilePicker renders the profile picker screen. The
// layout is intentionally simple: title, list of profiles
// (active one marked with a star, cursor with "> "), and a
// help bar. The empty-profiles case shows a prompt.
func viewProfilePicker(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Profiles "))
	b.WriteString("\n\n")

	if len(m.Config.Profiles) == 0 {
		b.WriteString(dimStyle.Render("No profiles — create one?"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("a add profile • q quit"))
		b.WriteString("\n")
		return b.String()
	}

	for i, p := range m.Config.Profiles {
		marker := "  "
		if i == m.ProfileCursor {
			marker = "> "
		}
		active := "  "
		if m.ActiveProfile != nil && p.ID == m.ActiveProfile.ID {
			active = "* "
		}
		row := fmt.Sprintf("%s%s%s", marker, active, p.Name)
		if i == m.ProfileCursor {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter switch • a add • r rename • d delete • esc back • q quit"))
	b.WriteString("\n")
	return b.String()
}

// updateProfilePicker handles the profile picker key bindings.
// The cursor is clamped to the profile list bounds; enter
// switches the active profile and returns to the list; a/r/d
// jump to the corresponding CRUD screen with the cursor's
// profile pre-selected; esc returns to the list.
func updateProfilePicker(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Empty case: only 'a' and quit are meaningful.
	if len(m.Config.Profiles) == 0 {
		switch key.String() {
		case "a":
			return openProfileAdd(m), nil
		case "q", "ctrl+c":
			return *m, tea.Quit
		}
		return *m, nil
	}

	switch key.String() {
	case "esc", "q":
		m.Screen = screenList
		return *m, nil
	case "up", "k":
		if m.ProfileCursor > 0 {
			m.ProfileCursor--
		}
		return *m, nil
	case "down", "j":
		if m.ProfileCursor < len(m.Config.Profiles)-1 {
			m.ProfileCursor++
		}
		return *m, nil
	case "enter":
		return switchActiveProfile(m), nil
	case "a":
		return openProfileAdd(m), nil
	case "r":
		return openProfileRename(m), nil
	case "d":
		return openProfileDelete(m), nil
	}
	return *m, nil
}

// openProfileAdd routes the user to the profile-add screen
// with a clean pending state. The screen renders as a name
// input; on submit the model transitions to the WoW path
// screen to collect the AddOns path.
func openProfileAdd(m *Model) Model {
	m.PendingProfileName = ""
	m.PendingProfilePath = ""
	m.PendingProfileID = ""
	m.ProfileNameError = ""
	m.ProfileError = ""
	m.WowPathInput = ""
	m.WowPathError = ""
	m.WowCandSel = -1
	m.Screen = screenProfileAdd
	return *m
}

// openProfileRename routes the user to the profile-rename
// screen with the cursor's profile selected. The current name
// is pre-filled so the user can edit in place.
func openProfileRename(m *Model) Model {
	if m.ProfileCursor < 0 || m.ProfileCursor >= len(m.Config.Profiles) {
		return *m
	}
	target := m.Config.Profiles[m.ProfileCursor]
	m.PendingProfileID = target.ID
	m.PendingProfileName = target.Name
	m.ProfileNameError = ""
	m.ProfileError = ""
	m.Screen = screenProfileRename
	return *m
}

// openProfileDelete routes the user to the profile-delete
// confirmation screen with the cursor's profile pre-selected.
func openProfileDelete(m *Model) Model {
	if m.ProfileCursor < 0 || m.ProfileCursor >= len(m.Config.Profiles) {
		return *m
	}
	target := m.Config.Profiles[m.ProfileCursor]
	m.PendingProfileID = target.ID
	m.ProfileError = ""
	m.Screen = screenProfileDelete
	return *m
}

// switchActiveProfile changes the active profile pointer to
// the cursor's selection, syncs m.WowPath, resets the
// per-profile selection/status state, and returns to the
// addon list.
func switchActiveProfile(m *Model) Model {
	if m.ProfileCursor < 0 || m.ProfileCursor >= len(m.Config.Profiles) {
		return *m
	}
	target := &m.Config.Profiles[m.ProfileCursor]
	m.SetActiveProfile(target)
	m.Config.ActiveProfileID = target.ID
	m.Selection = 0
	m.Statuses = map[string]AddonStatus{}
	m.ScrollOffset = 0
	m.SearchQuery = ""
	m.SearchActive = false
	m.WowPath = wowpath.Path(target.WoWPath)
	m.Screen = screenList
	return *m
}
