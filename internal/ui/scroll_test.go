package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

// TestUpdate_DownArrowScrollsWindow drives the selection to the
// end of a long list and asserts that the scroll window keeps the
// selected addon visible. Height 24 minus the 12 overhead lines
// leaves 12 visible addons.
func TestUpdate_DownArrowScrollsWindow(t *testing.T) {
	t.Parallel()
	addons := make([]config.Addon, 15)
	for i := range addons {
		addons[i] = config.Addon{Name: fmt.Sprintf("Addon%d", i)}
	}
	m := NewModel()
	m.Config = testConfigWithAddons(t, addons)
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
	m.Width = 80
	m.Height = 24
	m.Selection = 0

	updated := *m
	for i := 0; i < len(addons)-1; i++ {
		next, _ := updated.Update(tea.KeyMsg{Type: tea.KeyDown})
		updated = next.(Model)
	}
	if updated.Selection != len(addons)-1 {
		t.Errorf("Selection = %d, want %d", updated.Selection, len(addons)-1)
	}
	// Height 24, overhead 12 (no search) -> 12 visible items.
	if updated.ScrollOffset != 3 {
		t.Errorf("ScrollOffset = %d, want 3", updated.ScrollOffset)
	}

	lines := strings.Split(stripANSI(updated.View()), "\n")
	for _, n := range []string{"Addon3", "Addon4", "Addon5", "Addon6", "Addon7", "Addon8", "Addon9", "Addon10", "Addon11", "Addon12", "Addon13", "Addon14"} {
		if !viewHasLine(lines, n) {
			t.Errorf("view missing %s at bottom of scroll window", n)
		}
	}
	for _, n := range []string{"Addon0", "Addon1", "Addon2"} {
		if viewHasLine(lines, n) {
			t.Errorf("view should not show %s (scrolled past)", n)
		}
	}
}

// viewHasLine reports whether any line of the rendered view starts
// with name followed by a space (the first column of a row).
func viewHasLine(lines []string, name string) bool {
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, "│ \t")
		if strings.HasPrefix(trimmed, name) {
			rest := trimmed[len(name):]
			if rest == "" || rest[0] == ' ' {
				return true
			}
		}
	}
	return false
}
