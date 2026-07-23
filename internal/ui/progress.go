package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinnerTickMsg struct{}

func spinnerCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

type progressStepMsg struct {
	Label string
	Step  int
	Total int
}

func viewProgress(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Working "))
	b.WriteString("\n\n")

	frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
	elapsed := time.Since(m.progressStart).Truncate(time.Second)

	b.WriteString(progressStyle.Render(frame + " "))
	b.WriteString(m.ProgressLabel)
	b.WriteString(dimStyle.Render(fmt.Sprintf(" (%v)", elapsed)))

	if m.progressTotal > 1 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("   step %d of %d", m.progressStep, m.progressTotal)))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("press ctrl+c to cancel"))
	b.WriteString("\n")
	return b.String()
}

func updateProgress(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	_ = key
	return *m, spinnerCmd()
}

func viewError(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Error "))
	b.WriteString("\n\n")
	msg := m.ErrMessage
	if idx := strings.Index(msg, "\n"); idx > 0 {
		msg = msg[:idx]
	}
	if len(msg) > 120 {
		msg = msg[:120] + "..."
	}
	b.WriteString(errorStyle.Render(msg))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("press any key to return"))
	b.WriteString("\n")
	return b.String()
}
