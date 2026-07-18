package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

func TestNewModel_DefaultsToListScreen(t *testing.T) {
	t.Parallel()
	m := NewModel()
	if m.Screen != screenList {
		t.Errorf("default Screen = %d, want %d", m.Screen, screenList)
	}
	if m.Statuses == nil {
		t.Errorf("default Statuses = nil, want empty map")
	}
}

func TestUpdate_WindowSizeUpdatesDimensions(t *testing.T) {
	t.Parallel()
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := updated.(Model)
	if mm.Width != 120 || mm.Height != 40 {
		t.Errorf("WindowSize not stored: got %dx%d", mm.Width, mm.Height)
	}
}

func TestUpdate_CtrlCQuits(t *testing.T) {
	t.Parallel()
	m := NewModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Errorf("ctrl+c produced no command, want tea.Quit")
	}
}

func TestUpdate_QOnListQuits(t *testing.T) {
	t.Parallel()
	m := NewModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Errorf("q on list produced no command, want tea.Quit")
	}
}

func TestUpdate_DownArrowAdvancesSelection(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = &config.Config{
		Version: 1,
		Addons: []config.Addon{
			{Name: "Atlas"},
			{Name: "Bagnon"},
			{Name: "Dbm"},
		},
	}
	m.Selection = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm := updated.(Model)
	if mm.Selection != 1 {
		t.Errorf("after down: Selection = %d, want 1", mm.Selection)
	}
}

func TestUpdate_DownArrowClampsAtEnd(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = &config.Config{
		Version: 1,
		Addons:  []config.Addon{{Name: "A"}, {Name: "B"}},
	}
	m.Selection = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm := updated.(Model)
	if mm.Selection != 1 {
		t.Errorf("Selection = %d, want 1 (clamped at end)", mm.Selection)
	}
}

func TestUpdate_UpArrowClampsAtZero(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = &config.Config{
		Version: 1,
		Addons:  []config.Addon{{Name: "A"}},
	}
	m.Selection = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm := updated.(Model)
	if mm.Selection != 0 {
		t.Errorf("Selection = %d, want 0 (clamped)", mm.Selection)
	}
}

func TestUpdate_APressesSwitchToAddForm(t *testing.T) {
	t.Parallel()
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := updated.(Model)
	if mm.Screen != screenAddForm {
		t.Errorf("after a: Screen = %d, want screenAddForm", mm.Screen)
	}
}

func TestUpdate_AddFormAccumulatesInput(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	mm := updated.(Model)
	if mm.AddInput != "h" {
		t.Errorf("AddInput = %q, want %q", mm.AddInput, "h")
	}
}

func TestUpdate_AddFormBackspaceDeletes(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "abc"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	mm := updated.(Model)
	if mm.AddInput != "ab" {
		t.Errorf("AddInput after backspace = %q, want ab", mm.AddInput)
	}
}

func TestUpdate_AddFormEnterRejectsBadURL(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "not-a-url"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.AddError == "" {
		t.Errorf("AddError empty, want validation error")
	}
	if mm.Screen != screenAddForm {
		t.Errorf("Screen = %d, want screenAddForm (stays on form)", mm.Screen)
	}
}

func TestUpdate_AddFormEnterRejectsDuplicate(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "https://github.com/u/Atlas"
	m.Config.Addons = []config.Addon{{Name: "Atlas", URL: "https://github.com/u/Atlas"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.AddError == "" {
		t.Errorf("AddError empty, want duplicate error")
	}
	if !strings.Contains(mm.AddError, "already tracked") {
		t.Errorf("AddError = %q, want duplicate error", mm.AddError)
	}
}

func TestUpdate_AddFormEnterNonGitHubGoesToProgress(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "https://example.com/u/MyAddon.git"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProgress {
		t.Errorf("Screen = %d, want screenProgress for non-GitHub URL", mm.Screen)
	}
	if mm.PendingAddon.Name != "MyAddon" {
		t.Errorf("PendingAddon.Name = %q, want MyAddon", mm.PendingAddon.Name)
	}
}

func TestUpdate_AddFormEnterGitHubGoesToPicker(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "https://github.com/u/Atlas"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenReleasePicker {
		t.Errorf("Screen = %d, want screenReleasePicker", mm.Screen)
	}
}

func TestUpdate_ReleasePickerBranchReturnsToListWithAddon(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenReleasePicker
	m.AddInput = "https://github.com/u/Atlas"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	mm := updated.(Model)
	if mm.Screen != screenProgress {
		t.Errorf("after '1': Screen = %d, want screenProgress", mm.Screen)
	}
}

func TestUpdate_ReleasePickerRelease(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenReleasePicker
	m.AddInput = "https://github.com/u/Atlas"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	mm := updated.(Model)
	if mm.Screen != screenProgress {
		t.Errorf("after '2': Screen = %d, want screenProgress", mm.Screen)
	}
}

func TestUpdate_ErrorScreenReturnsToList(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenError
	m.ErrMessage = "boom"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenList {
		t.Errorf("after dismiss: Screen = %d, want screenList", mm.Screen)
	}
	if mm.ErrMessage != "" {
		t.Errorf("ErrMessage = %q, want empty", mm.ErrMessage)
	}
}

func TestView_ListWithAddons(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Width = 80
	m.Config.Addons = []config.Addon{
		{Name: "Atlas", TrackMode: "branch", TrackTarget: "main"},
		{Name: "Bagnon", TrackMode: "release", TrackTarget: "v1.0.0"},
	}
	m.Statuses = map[string]AddonStatus{
		"Atlas":  StatusOK,
		"Bagnon": StatusUpdate,
	}
	view := m.View()
	if !strings.Contains(view, "Atlas") {
		t.Errorf("view missing Atlas: %q", view)
	}
	if !strings.Contains(view, "Bagnon") {
		t.Errorf("view missing Bagnon: %q", view)
	}
	if !strings.Contains(view, "✓") {
		t.Errorf("view missing OK badge")
	}
	if !strings.Contains(view, "↑") {
		t.Errorf("view missing update badge")
	}
}

func TestView_EmptyListShowsPrompt(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Width = 80
	view := m.View()
	if !strings.Contains(view, "Press a to add") {
		t.Errorf("empty list view missing prompt: %q", view)
	}
}

func TestView_AddFormShowsInput(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenAddForm
	m.AddInput = "https://github.com/u/Atlas"
	view := m.View()
	if !strings.Contains(view, "https://github.com/u/Atlas") {
		t.Errorf("add form view missing input: %q", view)
	}
}

func TestView_ReleasePicker(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenReleasePicker
	m.AddInput = "https://github.com/u/Atlas"
	view := m.View()
	if !strings.Contains(view, "Track main") {
		t.Errorf("picker missing branch option: %q", view)
	}
	if !strings.Contains(view, "Track latest release") {
		t.Errorf("picker missing release option: %q", view)
	}
}

func TestView_Progress(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenProgress
	m.ProgressLabel = "Cloning Atlas..."
	view := m.View()
	if !strings.Contains(view, "Cloning Atlas") {
		t.Errorf("progress view missing label: %q", view)
	}
}

func TestView_Error(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Screen = screenError
	m.ErrMessage = "boom"
	view := m.View()
	if !strings.Contains(view, "boom") {
		t.Errorf("error view missing message: %q", view)
	}
}

func TestScreen_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		screen screen
		want   string
	}{
		{screenList, "list"},
		{screenAddForm, "addForm"},
		{screenReleasePicker, "releasePicker"},
		{screenProgress, "progress"},
		{screenError, "error"},
		{screenWowPath, "wowPath"},
		{screenWowBrowse, "wowBrowse"},
		{screen(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.screen.String(); got != tc.want {
			t.Errorf("screen.String() = %q, want %q", got, tc.want)
		}
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s AddonStatus
		w string
	}{
		{StatusOK, "✓"},
		{StatusUpdate, "↑"},
		{StatusError, "✗"},
		{StatusInstalling, "⟳"},
		{StatusUnknown, "?"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.w {
			t.Errorf("Status.String() = %q, want %q", got, tc.w)
		}
	}
}
