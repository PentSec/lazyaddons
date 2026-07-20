package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/config"
)

// viewProfileAdd renders the name-input stage of the
// profile-add flow. After the user submits a valid name, the
// model hands off to screenWowPath for path input.
func viewProfileAdd(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" New Profile "))
	b.WriteString("\n\n")
	b.WriteString("Profile name (1-64 chars, must be unique):\n\n")
	b.WriteString("> " + m.PendingProfileName)
	if m.PendingProfileName == "" {
		b.WriteString(dimStyle.Render("Retail, Classic, PTR, ..."))
	}
	b.WriteString("\n\n")

	if m.ProfileNameError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.ProfileNameError))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("enter next: WoW path • esc back"))
	b.WriteString("\n")
	return b.String()
}

// updateProfileAdd handles the name-input stage of the add
// flow. The path stage reuses screenWowPath (see wowpath.go
// and the createProfileFromPending helper).
func updateProfileAdd(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.PendingProfileName = ""
		m.ProfileNameError = ""
		// Return to picker if there are existing profiles, or
		// to list if there are none (first-run case — though
		// the spec actually says we stay on add until a
		// profile is created). Going to list keeps the user
		// from being trapped in an empty first-run.
		if len(m.Config.Profiles) > 0 {
			m.Screen = screenProfilePicker
		} else {
			m.Screen = screenList
		}
		return *m, nil
	case "backspace":
		if len(m.PendingProfileName) > 0 {
			m.PendingProfileName = m.PendingProfileName[:len(m.PendingProfileName)-1]
		}
		m.ProfileNameError = ""
		return *m, nil
	case "enter":
		return submitProfileName(m), nil
	}

	for _, r := range key.Runes {
		if r >= 32 && r < 127 {
			m.PendingProfileName += string(r)
			m.ProfileNameError = ""
		}
	}
	return *m, nil
}

// submitProfileName validates the typed name and, on success,
// transitions the model to screenWowPath so the user can pick
// or type the AddOns folder. The path stage owns the actual
// profile creation (see createProfileFromPending). Returns
// Model (not *Model) so the caller can pass it through the
// tea.Model interface cleanly.
func submitProfileName(m *Model) Model {
	name := strings.TrimSpace(m.PendingProfileName)
	if err := validateProfileName(name); err != nil {
		m.ProfileNameError = err.Error()
		return *m
	}
	if m.Config == nil {
		m.ProfileNameError = "internal error: nil config"
		return *m
	}
	if m.Config.FindProfileByName(name) != nil {
		m.ProfileNameError = fmt.Sprintf("Profile name %q already exists", name)
		return *m
	}
	if len(m.Config.Profiles) >= config.MaxProfiles {
		m.ProfileNameError = fmt.Sprintf(
			"Maximum number of profiles (%d) reached",
			config.MaxProfiles,
		)
		return *m
	}
	// Valid: keep the trimmed name and move to the path screen.
	m.PendingProfileName = name
	m.ProfileNameError = ""
	m.WowPathInput = ""
	m.WowPathError = ""
	m.WowCandSel = -1
	m.Screen = screenWowPath
	return *m
}

// validateProfileName enforces the spec rules for a profile
// name (non-empty, <= 64 chars after trimming).
func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("Profile name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("Profile name must be 64 characters or fewer (got %d)", len(name))
	}
	return nil
}
