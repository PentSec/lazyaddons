package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/update"
)

// selfUpdateDoneMsg is posted when the self-update download completes.
type selfUpdateDoneMsg struct {
	Err     error
	NewVersion string
}

// selfUpdateCmd downloads the new binary and replaces the running one.
func selfUpdateCmd(version string) tea.Cmd {
	return func() tea.Msg {
		tag := version
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		return selfUpdateDoneMsg{Err: update.SelfUpdate(tag), NewVersion: version}
	}
}

// handleSelfUpdateDone processes the self-update result.
func handleSelfUpdateDone(m *Model, msg selfUpdateDoneMsg) {
	if msg.Err != nil {
		m.ErrMessage = fmt.Sprintf("Self-update failed: %v", msg.Err)
		m.Screen = screenError
		return
	}
	m.ErrMessage = fmt.Sprintf(
		"lazyaddons updated to %s!\nRestart the application to use the new version.",
		msg.NewVersion,
	)
	m.Screen = screenError
}
