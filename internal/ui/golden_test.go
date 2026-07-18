package ui

import (
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/config"
)

// TestListView_GoldenFile renders the list view and compares the
// result to a checked-in golden file. Update the golden with
// `go test -run TestListView_GoldenFile -update` after
// intentional UI changes.
//
// We use direct Model.View() rather than teatest because the
// Bubble Tea program doesn't terminate cleanly inside `go test`
// without signal handling, and the rendered string is what we
// want to assert against anyway.
//
// The comparison is "fuzzy" — it normalises whitespace before
// checking so lipgloss format-string changes don't cascade into
// a golden file churn.
func TestListView_GoldenFile(t *testing.T) {
	m := NewModel()
	m.Width = 80
	m.Height = 24
	m.Config = &config.Config{Version: 1, Addons: []config.Addon{
		{Name: "Atlas", TrackMode: "branch", TrackTarget: "main", CurrentSHA: "abc1234"},
		{Name: "Bagnon", TrackMode: "release", TrackTarget: "v1.0.0", CurrentSHA: "def5678"},
		{Name: "Details", TrackMode: "branch", TrackTarget: "main", CurrentSHA: "9990000"},
	}}
	m.Statuses = map[string]AddonStatus{
		"Atlas":   StatusOK,
		"Bagnon":  StatusUpdate,
		"Details": StatusError,
	}
	m.Selection = 1

	got := stripANSI(m.View())
	got = normaliseWhitespace(got)
	want := normaliseWhitespace(readFile(t, "testdata/list_view.golden"))
	if updateGolden() {
		if err := writeTestData("testdata/list_view.golden", []byte(got)); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Skipf("golden file updated")
	}
	if got != want {
		// Find the first byte that differs.
		for i := 0; i < len(got) && i < len(want); i++ {
			if got[i] != want[i] {
				t.Errorf("list view mismatch at byte %d: got %q want %q\n--- got ---\n%s\n--- want ---\n%s",
					i, got[i], want[i], got, want)
				return
			}
		}
		t.Errorf("list view mismatch (lengths differ): got %d bytes, want %d bytes\n--- got ---\n%s\n--- want ---\n%s",
			len(got), len(want), got, want)
	}
}

// TestListView_ContainsAllAddons is a content-level sanity check
// that doesn't depend on a golden file. It ensures the rendering
// emits the addon names and the correct status characters.
func TestListView_ContainsAllAddons(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Width = 80
	m.Config = &config.Config{Version: 1, Addons: []config.Addon{
		{Name: "Atlas"},
		{Name: "Bagnon"},
		{Name: "Details"},
	}}
	m.Statuses = map[string]AddonStatus{
		"Atlas":   StatusOK,
		"Bagnon":  StatusUpdate,
		"Details": StatusError,
	}
	view := stripANSI(m.View())
	for _, name := range []string{"Atlas", "Bagnon", "Details"} {
		if !strings.Contains(view, name) {
			t.Errorf("view missing %s", name)
		}
	}
	if !strings.Contains(view, "✓") {
		t.Errorf("view missing OK badge")
	}
	if !strings.Contains(view, "↑") {
		t.Errorf("view missing update badge")
	}
	if !strings.Contains(view, "✗") {
		t.Errorf("view missing error badge")
	}
}

// TestKeyboardNavigation_DownArrow moves the selection through
// the addon list using only the model — no program loop.
func TestKeyboardNavigation_DownArrow(t *testing.T) {
	t.Parallel()
	m := *NewModel()
	m.Width = 80
	m.Config = &config.Config{Version: 1, Addons: []config.Addon{
		{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"},
	}}
	m.Selection = 0

	// 3 down presses should move from index 0 to index 3.
	for i := 0; i < 3; i++ {
		updated, _ := m.Update(downKey())
		m = updated.(Model)
	}
	if m.Selection != 3 {
		t.Errorf("after 3 down: Selection = %d, want 3", m.Selection)
	}
}

func TestKeyboardNavigation_UpArrow(t *testing.T) {
	t.Parallel()
	m := *NewModel()
	m.Width = 80
	m.Config = &config.Config{Version: 1, Addons: []config.Addon{
		{Name: "A"}, {Name: "B"}, {Name: "C"},
	}}
	m.Selection = 2

	updated, _ := m.Update(upKey())
	m = updated.(Model)
	if m.Selection != 1 {
		t.Errorf("after up: Selection = %d, want 1", m.Selection)
	}
}

func TestKeyboardNavigation_ShortcutKeys(t *testing.T) {
	t.Parallel()
	m := *NewModel()
	m.Width = 80
	m.Config = &config.Config{Version: 1, Addons: []config.Addon{{Name: "A"}}}

	updated, _ := m.Update(charKey('a'))
	m = updated.(Model)
	if m.Screen != screenAddForm {
		t.Errorf("a: Screen = %d, want screenAddForm", m.Screen)
	}
}

// updateGolden reports whether the -update flag was passed.
func updateGolden() bool {
	for _, arg := range []string{"-update", "-update-golden"} {
		for _, a := range []string{"-test.run", "-args"} {
			_ = a
		}
		_ = arg
	}
	// Go's testing package doesn't expose the flag list
	// portably; we read os.Args ourselves.
	for _, a := range readArgs() {
		if a == "-update" {
			return true
		}
	}
	return false
}
