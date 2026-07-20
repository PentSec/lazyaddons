package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

func viewWowPath(m *Model) string {
	var b strings.Builder

	// When we are mid-create-profile, the title reflects that so
	// the user knows this path input is for a new profile.
	if m.PendingProfileName != "" {
		b.WriteString(titleStyle.Render(" Profile: WoW AddOns Folder "))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Profile name: " + m.PendingProfileName))
		b.WriteString("\n\n")
	} else {
		b.WriteString(titleStyle.Render(" WoW AddOns Folder "))
		b.WriteString("\n\n")
	}

	// Write-protection warning takes over the whole screen.
	if m.WowWriteWarning != "" {
		b.WriteString(errorStyle.Render(m.WowWriteWarning))
		b.WriteString("\n\n")
		b.WriteString("lazyaddons needs to create folders and clone repos here.\n\n")
		b.WriteString(helpStyle.Render("Solutions:"))
		b.WriteString("\n")
		b.WriteString("> ")
		b.WriteString(selectedStyle.Render("Re-launch as administrator (recommended)"))
		b.WriteString("\n  ")
		b.WriteString(dimStyle.Render("Opens a new lazyaddons window with write access."))
		b.WriteString("\n  ")
		b.WriteString("Choose a different folder")
		b.WriteString("\n  ")
		b.WriteString("Ignore and continue anyway")
		b.WriteString("\n\n")
		if m.PendingProfileName != "" {
			b.WriteString(helpStyle.Render("enter launch as admin • esc back to name • i ignore"))
		} else {
			b.WriteString(helpStyle.Render("enter launch as admin • esc choose another • i ignore"))
		}
		b.WriteString("\n")
		return b.String()
	}

	if m.WowSearching {
		b.WriteString(dimStyle.Render("  Searching for WoW installations..."))
		b.WriteString("\n\n")
	} else if len(m.WowCandidates) > 0 {
		b.WriteString(helpStyle.Render("Detected installations:"))
		b.WriteString("\n")
		for i, c := range m.WowCandidates {
			marker := "  "
			if i == m.WowCandSel {
				marker = "> "
			}
			entry := marker + c
			if i == m.WowCandSel {
				b.WriteString(selectedStyle.Render(entry))
			} else {
				b.WriteString(entry)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else if !m.WowSearching {
		b.WriteString(dimStyle.Render("  No installations auto-detected."))
		b.WriteString("\n\n")
	}

	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")
	b.WriteString("Or type a custom path:\n\n")
	b.WriteString("> " + m.WowPathInput)
	if m.WowPathInput == "" {
		b.WriteString(dimStyle.Render("/path/to/WoW/Interface/AddOns"))
	}
	b.WriteString("\n\n")

	if m.WowPathError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.WowPathError))
		b.WriteString("\n")
	}

	var help strings.Builder
	if len(m.WowCandidates) > 0 {
		help.WriteString("↑↓ pick candidate  •  ")
	}
	help.WriteString("type custom path  •  ")
	help.WriteString("b browse filesystem  •  ")
	if m.PendingProfileName != "" {
		help.WriteString("enter confirm  •  esc back")
	} else {
		help.WriteString("enter confirm  •  esc quit")
	}
	b.WriteString(helpStyle.Render(help.String()))
	b.WriteString("\n")
	return b.String()
}

func updateWowPath(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Write-protection warning has its own key bindings.
	if m.WowWriteWarning != "" {
		switch key.String() {
		case "esc":
			m.WowWriteWarning = ""
			m.WowPathError = ""
			// Mid-create-profile: return to the name screen.
			if m.PendingProfileName != "" {
				m.Screen = screenProfileAdd
			}
			return *m, nil
		case "enter":
			// Re-launch as admin and quit current process.
			if err := wowpath.RelaunchAsAdmin(); err != nil {
				m.WowPathError = "Failed to re-launch: " + err.Error()
				m.WowWriteWarning = ""
				return *m, nil
			}
			return *m, tea.Quit
		case "i":
			// Ignore warning, accept the path anyway.
			return acceptPath(m)
		}
		return *m, nil
	}

	switch key.String() {
	case "esc":
		if m.PendingProfileName != "" {
			// Mid-create-profile: return to the name screen so
			// the user can retry the name. Keep PendingProfileName
			// set so they don't lose it.
			m.Screen = screenProfileAdd
			m.WowPathInput = ""
			m.WowPathError = ""
			return *m, nil
		}
		return *m, tea.Quit
	case "b":
		m.WowBrowsePath = userHomeDir()
		m.WowBrowseSel = 0
		m.Screen = screenWowBrowse
		return *m, nil
	case "up", "k":
		if m.WowCandSel > 0 {
			m.WowCandSel--
		}
		return *m, nil
	case "down", "j":
		if m.WowCandSel < len(m.WowCandidates)-1 {
			m.WowCandSel++
		}
		return *m, nil
	case "backspace":
		if len(m.WowPathInput) > 0 {
			m.WowPathInput = m.WowPathInput[:len(m.WowPathInput)-1]
		}
		m.WowCandSel = -1
		return *m, nil
	case "enter":
		if m.WowCandSel >= 0 && m.WowCandSel < len(m.WowCandidates) {
			return confirmPath(m, m.WowCandidates[m.WowCandSel])
		}
		if m.WowPathInput != "" {
			return confirmPath(m, m.WowPathInput)
		}
		return *m, nil
	}

	for _, r := range key.Runes {
		if r >= 32 && r < 127 {
			m.WowPathInput += string(r)
			m.WowCandSel = -1
		}
	}
	return *m, nil
}

func confirmPath(m *Model, input string) (tea.Model, tea.Cmd) {
	p, err := wowpath.Resolve(input)
	if err != nil {
		m.WowPathError = err.Error()
		return *m, nil
	}

	// Profile creation flow: store the resolved path in
	// PendingProfilePath so the accept step can build the
	// Profile struct. We do NOT touch m.ActiveProfile here
	// because the new profile does not exist yet.
	if m.PendingProfileName != "" {
		m.PendingProfilePath = string(p)
		if !wowpath.IsWritable(m.PendingProfilePath) {
			m.WowWriteWarning = m.PendingProfilePath
			return *m, nil
		}
		return acceptPath(m)
	}

	// Standard flow (existing profile): store the resolved path
	// on the active profile.
	if m.ActiveProfile != nil {
		m.ActiveProfile.WoWPath = string(p)
	}
	if !wowpath.IsWritable(string(p)) {
		m.WowWriteWarning = string(p)
		return *m, nil
	}
	return acceptPath(m)
}

// acceptPath finalises whichever path flow is in progress:
//   - pending profile creation → mints the profile, sets it
//     active, returns to the addon list;
//   - normal flow → syncs m.WowPath and returns to the list.
func acceptPath(m *Model) (tea.Model, tea.Cmd) {
	m.WowWriteWarning = ""
	m.WowPathError = ""

	if m.PendingProfileName != "" {
		return createProfileFromPending(m)
	}

	m.WowPath = wowpath.Path(m.ActiveProfile.WoWPath)
	m.Screen = screenList
	return *m, nil
}

// createProfileFromPending materialises the profile the user
// has been assembling across the name + path screens, then
// returns the model to the addon list with the new profile
// marked active.
func createProfileFromPending(m *Model) (tea.Model, tea.Cmd) {
	id, err := config.NewUUID()
	if err != nil {
		m.ErrMessage = fmt.Sprintf("Failed to generate profile ID: %v", err)
		m.Screen = screenError
		m.PendingProfileName = ""
		m.PendingProfilePath = ""
		return *m, nil
	}
	p := config.Profile{
		ID:      id,
		Name:    m.PendingProfileName,
		WoWPath: m.PendingProfilePath,
		Addons:  []config.Addon{},
	}
	if err := m.Config.AddProfile(p); err != nil {
		m.ErrMessage = fmt.Sprintf("Failed to create profile: %v", err)
		m.Screen = screenError
		m.PendingProfileName = ""
		m.PendingProfilePath = ""
		return *m, nil
	}
	// AddProfile appended the new profile to the end of
	// m.Config.Profiles. The append may have reallocated the
	// backing array, which would invalidate any pre-existing
	// *config.Profile pointers into the slice — including the
	// m.ActiveProfile the user was on before this create
	// flow. We rewire to the newly created profile and adopt
	// it as the active one, so the pointer is valid for every
	// subsequent mutation (UpsertAddon, RemoveAddon, etc.).
	last := &m.Config.Profiles[len(m.Config.Profiles)-1]
	m.SetActiveProfile(last)
	m.Config.ActiveProfileID = last.ID

	// Reset the flow state.
	m.PendingProfileName = ""
	m.PendingProfilePath = ""
	m.WowPathInput = ""
	m.WowCandSel = -1
	m.WowPath = wowpath.Path(last.WoWPath)
	m.Statuses = map[string]AddonStatus{}
	m.Selection = 0
	m.Screen = screenList
	return *m, nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return home
}
