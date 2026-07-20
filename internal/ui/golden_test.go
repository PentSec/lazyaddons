package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/config"
)

// goldenTestProfile returns a v2 Config with a single active
// profile "Retail" holding the standard fixture addons. Tests
// that need a v2 fixture (e.g. golden-file comparison) build
// their model on top of this.
func goldenTestProfile() *config.Config {
	return &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:   "golden-profile-id",
				Name: "Retail",
				WoWPath: "/tmp/wow/Interface/AddOns",
				Addons: []config.Addon{
					{Name: "Atlas", TrackMode: "branch", TrackTarget: "main", CurrentSHA: "abc1234"},
					{Name: "Bagnon", TrackMode: "release", TrackTarget: "v1.0.0", CurrentSHA: "def5678"},
					{Name: "Details", TrackMode: "branch", TrackTarget: "main", CurrentSHA: "9990000"},
				},
			},
		},
		ActiveProfileID: "golden-profile-id",
	}
}

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
	m.Config = goldenTestProfile()
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
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
	m.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:      "p1",
				Name:    "Retail",
				WoWPath: "/tmp/wow/Interface/AddOns",
				Addons: []config.Addon{
					{Name: "Atlas"},
					{Name: "Bagnon"},
					{Name: "Details"},
				},
			},
		},
		ActiveProfileID: "p1",
	}
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
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
	m.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:   "p1",
				Name: "Retail",
				Addons: []config.Addon{
					{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"},
				},
			},
		},
		ActiveProfileID: "p1",
	}
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
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
	m.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:   "p1",
				Name: "Retail",
				Addons: []config.Addon{
					{Name: "A"}, {Name: "B"}, {Name: "C"},
				},
			},
		},
		ActiveProfileID: "p1",
	}
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))
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
	m.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:   "p1",
				Name: "Retail",
				Addons: []config.Addon{
					{Name: "A"},
				},
			},
		},
		ActiveProfileID: "p1",
	}
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))

	updated, _ := m.Update(charKey('a'))
	m = updated.(Model)
	if m.Screen != screenAddForm {
		t.Errorf("a: Screen = %d, want screenAddForm", m.Screen)
	}
}

// updateGolden reports whether the user asked to refresh the
// golden file. We look for both the env var and a CLI flag
// because `go test` rejects unknown flags in the test binary's
// argv. The env var is the reliable path:
//
//	UPDATE_GOLDEN=1 go test ./internal/ui -run TestListView_GoldenFile
//
// The -update CLI path is also tried (read from os.Args by the
// readArgs helper) for users who manage to pass it through.
func updateGolden() bool {
	if os.Getenv("UPDATE_GOLDEN") != "" {
		return true
	}
	for _, a := range readArgs() {
		if a == "-update" || a == "-update-golden" {
			return true
		}
	}
	return false
}
