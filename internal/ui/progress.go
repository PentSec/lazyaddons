package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinnerTickMsg struct{}

func spinnerCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// sendDetail returns a Cmd that updates the progress detail line.
func sendDetail(detail string) tea.Cmd {
	return func() tea.Msg {
		return progressDetailMsg{Detail: detail}
	}
}

type progressStepMsg struct {
	Label string
	Step  int
	Total int
}

type progressDetailMsg struct {
	Detail string
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

	if m.ProgressDetail != "" {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  ⤷ " + m.ProgressDetail))
	}

	if m.progressTotal > 1 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("   step %d of %d", m.progressStep, m.progressTotal)))
	}

	b.WriteString("\n\n")
	b.WriteString(renderHelp("ctrl+c to cancel"))
	b.WriteString("\n")
	return b.String()
}

// viewProgressModal renders the progress panel as a centered modal
// floating on top of the addon list, mirroring the remove-confirm
// overlay. Used when a progress operation started from the list
// (addon update, check-all, self-update).
func viewProgressModal(m *Model) string {
	inner := max(m.Width-2, minInner)
	avail := max(m.Height-11, 1)
	base := modalBase(m)

	modal := dialogBoxStyle.Render(strings.TrimRight(viewProgress(m), "\n"))

	x := max((inner-lipgloss.Width(modal))/2, 0)
	y := max((avail-lipgloss.Height(modal))/2, 0)

	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(modal).X(x).Y(y),
	).Render()
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
	help := "any key to return"
	if m.PendingQuit {
		help = "any key to quit"
	}
	b.WriteString(errorStyle.Render(msg))
	b.WriteString("\n\n")
	b.WriteString(renderHelp(help))
	b.WriteString("\n")
	return b.String()
}
