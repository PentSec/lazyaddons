package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"
)

// keybindLine is a single renderable line in the keybinds modal.
// Section headers have Key="" so they render differently.
type keybindLine struct {
	Section string // non-empty for section headers
	Key     string
	Desc    string
}

// allKeybindLines returns the flattened list of keybinding lines.
func allKeybindLines() []keybindLine {
	var lines []keybindLine

	groups := []struct {
		Title string
		Items []struct{ Key, Desc string }
	}{
		{
			Title: "Addon List",
			Items: []struct{ Key, Desc string }{
				{"↑/↓ j/k", "Navigate addons"},
				{"/", "Search addons"},
				{"a", "Add addon"},
				{"d", "Remove addon"},
				{"enter", "Update selected addon"},
				{"U", "Update all / self-update"},
				{"p", "Open profiles"},
				{"?", "Show this help"},
				{"q", "Quit"},
			},
		},
		{
			Title: "Profiles",
			Items: []struct{ Key, Desc string }{
				{"↑/↓ j/k", "Navigate profiles"},
				{"enter", "Switch active profile"},
				{"a", "Add profile"},
				{"r", "Rename profile"},
				{"d", "Delete profile"},
				{"esc", "Back to list"},
			},
		},
		{
			Title: "Add Addon",
			Items: []struct{ Key, Desc string }{
				{"enter", "Submit URL"},
				{"esc", "Cancel"},
			},
		},
		{
			Title: "Track Mode",
			Items: []struct{ Key, Desc string }{
				{"1", "Track branch"},
				{"2", "Track latest release"},
				{"esc", "Cancel"},
			},
		},
		{
			Title: "WoW Path",
			Items: []struct{ Key, Desc string }{
				{"enter", "Submit path"},
				{"↑/↓ j/k", "Navigate candidates"},
				{"b", "Browse for folder"},
				{"esc", "Back / Quit"},
			},
		},
		{
			Title: "Global",
			Items: []struct{ Key, Desc string }{
				{"ctrl+c", "Force quit"},
			},
		},
	}

	for _, g := range groups {
		lines = append(lines, keybindLine{Section: g.Title})
		for _, it := range g.Items {
			lines = append(lines, keybindLine{Key: it.Key, Desc: it.Desc})
		}
		lines = append(lines, keybindLine{}) // blank separator
	}
	return lines
}

// viewKeybindsModal renders the keybinds help as a scrollable
// modal overlay on top of the addon list.
func viewKeybindsModal(m *Model) string {
	inner := max(m.Width-2, minInner)
	avail := max(m.Height-11, 1)
	base := modalBase(m)

	// Limit modal height to 60% of available space, at least 8 lines.
	maxHeight := avail * 6 / 10
	if maxHeight < 8 {
		maxHeight = 8
	}

	lines := allKeybindLines()
	modalW := min(inner-4, 62)

	// Render all lines to measure total height.
	content := renderKeybindLines(lines, modalW)
	allRendered := strings.Split(content, "\n")
	totalLines := len(allRendered)

	// Clamp scroll offset.
	scrollMax := totalLines - maxHeight
	if scrollMax < 0 {
		scrollMax = 0
	}
	if m.KeybindScroll > scrollMax {
		m.KeybindScroll = scrollMax
	}
	if m.KeybindScroll < 0 {
		m.KeybindScroll = 0
	}

	// Slice the visible window.
	visible := allRendered
	if totalLines > maxHeight {
		end := m.KeybindScroll + maxHeight
		if end > totalLines {
			end = totalLines
		}
		visible = allRendered[m.KeybindScroll:end]
	}

	// Add scroll indicators.
	header := ""
	footer := ""
	if m.KeybindScroll > 0 {
		header = dimStyle.Render("  ↑ more above") + "\n"
	}
	if m.KeybindScroll < scrollMax {
		footer = "\n" + dimStyle.Render("  ↓ more below")
	}

 modalContent := header + strings.Join(visible, "\n") + footer
	modal := dialogBoxStyle.
		Width(modalW).
		Render(modalContent)

	x := max((inner-lipgloss.Width(modal))/2, 0)
	y := max((avail-lipgloss.Height(modal))/2, 0)

	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(modal).X(x).Y(y),
	).Render()
}

// renderKeybindLines renders all lines with styling.
func renderKeybindLines(lines []keybindLine, width int) string {
	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		MarginBottom(0)

	keyColWidth := 14

	for _, l := range lines {
		if l.Section != "" {
			b.WriteString(sectionStyle.Render(l.Section))
			b.WriteString("\n")
			continue
		}
		if l.Key == "" && l.Desc == "" {
			// Blank separator line.
			b.WriteString("\n")
			continue
		}
		key := keyStyle.Render(padRight(l.Key, keyColWidth))
		desc := helpStyle.Render(l.Desc)
		b.WriteString("  " + key + desc + "\n")
	}
	return b.String()
}

// padRight pads s to the right with spaces to reach width n.
func padRight(s string, n int) string {
	v := visibleLen(s)
	if v >= n {
		return s
	}
	return s + repeat(" ", n-v)
}

// updateKeybinds handles key events on the keybinds modal.
func updateKeybinds(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q", "?":
		m.KeybindScroll = 0
		m.Screen = screenList
		return *m, nil
	case "up", "k":
		if m.KeybindScroll > 0 {
			m.KeybindScroll--
		}
		return *m, nil
	case "down", "j":
		m.KeybindScroll++
		return *m, nil
	case "pgup":
		m.KeybindScroll -= 5
		if m.KeybindScroll < 0 {
			m.KeybindScroll = 0
		}
		return *m, nil
	case "pgdown":
		m.KeybindScroll += 5
		return *m, nil
	case "home":
		m.KeybindScroll = 0
		return *m, nil
	case "end":
		m.KeybindScroll = 9999 // clamped in view
		return *m, nil
	}
	// Any other key closes.
	m.KeybindScroll = 0
	m.Screen = screenList
	return *m, nil
}
