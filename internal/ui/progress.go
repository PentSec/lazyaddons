package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// viewProgress renders the spinner + status text used during
// long-running operations. We use a simple ASCII spinner that
// doesn't depend on the bubbles spinner component so the golden
// files stay deterministic across terminals.
func viewProgress(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Working "))
	b.WriteString("\n\n")
	b.WriteString(progressStyle.Render("[...] "))
	b.WriteString(m.ProgressLabel)
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("press ctrl+c to cancel"))
	b.WriteString("\n")
	return b.String()
}

func updateProgress(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The progress screen is read-only except for the global
	// ctrl+c handler bound in the parent.
	_ = key
	return *m, nil
}

func viewError(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Error "))
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render(m.ErrMessage))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("press any key to return"))
	b.WriteString("\n")
	return b.String()
}
