package ui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

func viewWowPath(m *Model) string {
	var b strings.Builder

	// Write-protection warning takes over the whole screen.
	if m.WowWriteWarning != "" {
		b.WriteString(titleStyle.Render(" Cannot write to AddOns folder "))
		b.WriteString("\n\n")
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
		b.WriteString(helpStyle.Render("enter launch as admin • esc choose another • i ignore"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(titleStyle.Render(" WoW AddOns Folder "))
	b.WriteString("\n\n")

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
	help.WriteString("enter confirm  •  esc quit")
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
	// Store early so acceptPath can use it.
	m.Config.WoWPath = string(p)
	// Check if the AddOns folder is writable. On Windows,
	// C:\Program Files requires admin privileges.
	if !wowpath.IsWritable(string(p)) {
		m.WowWriteWarning = string(p)
		return *m, nil
	}
	return acceptPath(m)
}

// acceptPath stores the resolved path and advances to the main screen.
// m.Config.WoWPath must already be set by the caller.
func acceptPath(m *Model) (tea.Model, tea.Cmd) {
	m.WowWriteWarning = ""
	m.WowPath = wowpath.Path(m.Config.WoWPath)
	m.WowPathError = ""
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
