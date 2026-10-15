package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

var errTest = errors.New("test error")

// TestHandleSelfUpdateDone_SuccessUsesSuccessScreen verifies that a
// successful self-update lands on the success screen — not the
// error screen — flags the pending restart, and renders the full
// message including the restart hint.
func TestHandleSelfUpdateDone_SuccessUsesSuccessScreen(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Width, m.Height = 120, 40

	handleSelfUpdateDone(m, selfUpdateDoneMsg{NewVersion: "v1.2.3"})
	if m.Screen != screenSuccess {
		t.Errorf("Screen = %v (%s), want screenSuccess", m.Screen, m.Screen)
	}
	if !m.PendingQuit {
		t.Error("PendingQuit = false, want true: the app restarts")
	}
	if m.ErrMessage != "" {
		t.Errorf("ErrMessage = %q, want empty on success", m.ErrMessage)
	}

	view := m.View()
	for _, want := range []string{
		"Success",
		"lazyaddons updated to v1.2.3!",
		"Restart the application to use the new version.",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Error") {
		t.Errorf("success screen renders the Error title:\n%s", view)
	}
}

// TestHandleSelfUpdateDone_FailureDoesNotQuit verifies a failed
// self-update stays on the regular error flow (any key returns to
// the list) and never touches the success message.
func TestHandleSelfUpdateDone_FailureDoesNotQuit(t *testing.T) {
	t.Parallel()
	m := NewModel()
	handleSelfUpdateDone(m, selfUpdateDoneMsg{Err: errTest})
	if m.PendingQuit {
		t.Error("PendingQuit = true, want false")
	}
	if m.Screen != screenError {
		t.Errorf("Screen = %v, want screenError", m.Screen)
	}
	if m.SuccessMessage != "" {
		t.Errorf("SuccessMessage = %q, want empty on failure", m.SuccessMessage)
	}
	if !strings.Contains(m.View(), "Self-update failed") {
		t.Error("view does not report the failure")
	}
}

// TestErrorScreen_SelfUpdateKeyQuits verifies that a key press on
// the error screen after a successful self-update quits the app
// instead of returning to the list.
func TestErrorScreen_SelfUpdateKeyQuits(t *testing.T) {
	t.Parallel()
	m := newModelWithProfiles(t, config.Profile{
		Name:    "Retail",
		WoWPath: "/tmp/wow/Interface/AddOns",
	})
	m.Screen = screenError
	m.PendingQuit = true
	mm, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if mm.(Model).Screen != screenError {
		t.Errorf("Screen = %v, want screenError before quitting", mm.(Model).Screen)
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("command returned %T, want tea.QuitMsg", cmd())
	}
}

// TestSuccessScreen_KeyQuits mirrors the error-screen case for the
// success screen: after an update the user restarts, so any key
// quits instead of returning to the list.
func TestSuccessScreen_KeyQuits(t *testing.T) {
	t.Parallel()
	m := newModelWithProfiles(t, config.Profile{
		Name:    "Retail",
		WoWPath: "/tmp/wow/Interface/AddOns",
	})
	m.Screen = screenSuccess
	m.PendingQuit = true
	m.SuccessMessage = "lazyaddons updated to v1.2.3!"
	mm, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if mm.(Model).Screen != screenSuccess {
		t.Errorf("Screen = %v, want screenSuccess before quitting", mm.(Model).Screen)
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("command returned %T, want tea.QuitMsg", cmd())
	}
}
