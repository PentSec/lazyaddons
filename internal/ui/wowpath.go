package ui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

func viewWowPath(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" WoW AddOns folder "))
	b.WriteString("\n\n")
	b.WriteString("Enter the path to your WoW AddOns folder,\n")
	b.WriteString("or press b to browse.\n\n")
	b.WriteString("Examples:\n")
	b.WriteString("  Linux:  /home/user/wow/Interface/AddOns\n")
	b.WriteString("  Windows: C:\\Games\\WoW\\Interface\\AddOns\n")
	b.WriteString("\n")
	b.WriteString("> " + m.WowPathInput)
	if m.WowPathInput == "" {
		b.WriteString(dimStyle.Render("/path/to/WoW/Interface/AddOns"))
	}
	b.WriteString("\n\n")
	if m.WowPathError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.WowPathError))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Render("enter confirm • b browse • esc quit"))
	b.WriteString("\n")
	return b.String()
}

func updateWowPath(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		return *m, tea.Quit
	case "b":
		// Open directory browser starting from home.
		m.WowBrowsePath = userHomeDir()
		m.WowBrowseSel = 0
		m.Screen = screenWowBrowse
		return *m, nil
	case "backspace":
		if len(m.WowPathInput) > 0 {
			m.WowPathInput = m.WowPathInput[:len(m.WowPathInput)-1]
		}
		return *m, nil
	case "enter":
		p, err := wowpath.Resolve(m.WowPathInput)
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

	for _, r := range key.Runes {
		if r >= 32 && r < 127 {
			m.WowPathInput += string(r)
		}
	}
	return *m, nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return home
}
