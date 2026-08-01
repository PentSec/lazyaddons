package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

var errTest = errors.New("test error")

// TestHandleSelfUpdateDone_SuccessSetsPendingQuit verifies that a
// successful self-update flags the error screen so the next key
// quits the app (a restart is required).
func TestHandleSelfUpdateDone_SuccessSetsPendingQuit(t *testing.T) {
	t.Parallel()
	m := &Model{}
	handleSelfUpdateDone(m, selfUpdateDoneMsg{NewVersion: "v1.2.3"})
	if !m.PendingQuit {
		t.Error("PendingQuit = false, want true")
	}
	if m.Screen != screenError {
		t.Errorf("Screen = %v, want screenError", m.Screen)
	}
}

// TestHandleSelfUpdateDone_FailureDoesNotQuit verifies a failed
// self-update stays on the regular error flow (any key returns to
// the list).
func TestHandleSelfUpdateDone_FailureDoesNotQuit(t *testing.T) {
	t.Parallel()
	m := &Model{}
	handleSelfUpdateDone(m, selfUpdateDoneMsg{Err: errTest})
	if m.PendingQuit {
		t.Error("PendingQuit = true, want false")
	}
	if m.Screen != screenError {
		t.Errorf("Screen = %v, want screenError", m.Screen)
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
