package ui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

func viewWowPath(m *Model) string {
	var b strings.Builder
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
	m.WowPath = p
	m.Config.WoWPath = p.String()
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
