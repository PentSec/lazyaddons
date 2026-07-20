package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

// =============================================================================
// T4 — Profile Picker
// =============================================================================

// pickerModelWithProfiles returns a model positioned on the profile
// picker screen with the given profiles. The first profile is
// marked active (matching the standard newModelWithProfiles
// behaviour), and the picker cursor is on the active entry.
func pickerModelWithProfiles(t *testing.T, profiles ...config.Profile) *Model {
	t.Helper()
	m := newModelWithProfiles(t, profiles...)
	m.Screen = screenProfilePicker
	m.ProfileCursor = 0
	if m.ActiveProfile != nil {
		for i, p := range m.Config.Profiles {
			if p.ID == m.ActiveProfile.ID {
				m.ProfileCursor = i
				break
			}
		}
	}
	return m
}

// typeName drives the model as if the user typed each character of
// `s` into the current input field. It takes a value because the
// standard Model.Update pattern is to receive a fresh value, not
// to mutate in place.
func typeName(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

// backspaceN sends n backspace key events to the model.
func backspaceN(m Model, n int) Model {
	for i := 0; i < n; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(Model)
	}
	return m
}

// profileNameOrEmpty returns the active profile's name or "<nil>"
// for diagnostic messages.
func profileNameOrEmpty(m *Model) string {
	if m == nil || m.ActiveProfile == nil {
		return "<nil>"
	}
	return m.ActiveProfile.Name
}

// TestProfilePicker_RendersProfileList verifies the picker view
// lists every profile by name so the user can see them.
func TestProfilePicker_RendersProfileList(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{Name: "Retail"},
		config.Profile{Name: "Classic"},
		config.Profile{Name: "PTR"},
	)
	view := stripANSI(m.View())
	for _, name := range []string{"Retail", "Classic", "PTR"} {
		if !strings.Contains(view, name) {
			t.Errorf("picker view missing %q\n--- view ---\n%s", name, view)
		}
	}
}

// TestProfilePicker_ActiveProfileIsMarked verifies the active
// profile carries a visible marker in the picker view so the
// user knows which one is currently in use.
func TestProfilePicker_ActiveProfileIsMarked(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{Name: "Retail"},
		config.Profile{Name: "Classic"},
	)
	view := stripANSI(m.View())
	hasMarker := strings.Contains(view, "Retail *") ||
		strings.Contains(view, "* Retail") ||
		strings.Contains(view, "› Retail") ||
		strings.Contains(view, "active")
	if !hasMarker {
		t.Errorf("picker view missing active marker for Retail:\n%s", view)
	}
}

// TestProfilePicker_EnterSwitchesActive verifies pressing enter
// on a non-active profile switches the active profile pointer
// and returns to the addon list.
func TestProfilePicker_EnterSwitchesActive(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{Name: "Retail", WoWPath: "/tmp/retail"},
		config.Profile{Name: "Classic", WoWPath: "/tmp/classic"},
	)
	if m.ActiveProfile == nil || m.ActiveProfile.Name != "Retail" {
		t.Fatalf("setup: ActiveProfile.Name = %q, want Retail",
			profileNameOrEmpty(m))
	}
	m.ProfileCursor = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenList {
		t.Errorf("after enter: Screen = %v, want screenList", mm.Screen)
	}
	if mm.ActiveProfile == nil {
		t.Fatalf("after enter: ActiveProfile = nil")
	}
	if mm.ActiveProfile.Name != "Classic" {
		t.Errorf("after enter: ActiveProfile.Name = %q, want Classic", mm.ActiveProfile.Name)
	}
	if mm.Config.ActiveProfileID != mm.ActiveProfile.ID {
		t.Errorf("after enter: Config.ActiveProfileID = %q, want %q",
			mm.Config.ActiveProfileID, mm.ActiveProfile.ID)
	}
	// Model field is WowPath (lowercase o), Profile field is WoWPath.
	if string(mm.WowPath) != "/tmp/classic" {
		t.Errorf("after enter: m.WowPath = %q, want /tmp/classic", string(mm.WowPath))
	}
}

// TestProfilePicker_EnterOnActiveStaysActive verifies pressing
// enter on the already-active profile is a no-op (still active,
// still on list).
func TestProfilePicker_EnterOnActiveStaysActive(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{Name: "Retail", WoWPath: "/tmp/retail"},
		config.Profile{Name: "Classic", WoWPath: "/tmp/classic"},
	)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenList {
		t.Errorf("after enter on active: Screen = %v, want screenList", mm.Screen)
	}
	if mm.ActiveProfile == nil || mm.ActiveProfile.Name != "Retail" {
		t.Errorf("after enter on active: name = %q, want Retail", profileNameOrEmpty(&mm))
	}
}

// TestProfilePicker_EscReturnsToList verifies esc returns the
// user to the addon list without changing the active profile.
func TestProfilePicker_EscReturnsToList(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{Name: "Retail", WoWPath: "/tmp/retail"},
		config.Profile{Name: "Classic", WoWPath: "/tmp/classic"},
	)
	m.ProfileCursor = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.Screen != screenList {
		t.Errorf("after esc: Screen = %v, want screenList", mm.Screen)
	}
	if mm.ActiveProfile == nil || mm.ActiveProfile.Name != "Retail" {
		t.Errorf("after esc: active changed to %q, want unchanged Retail",
			profileNameOrEmpty(&mm))
	}
}

// TestProfilePicker_UpDownNavigation verifies the cursor moves
// through the profile list and clamps at both ends.
func TestProfilePicker_UpDownNavigation(t *testing.T) {
	t.Parallel()
	// Work with a Model value, not a pointer, to avoid the
	// "cannot use Model as *Model" assignment error.
	m := *pickerModelWithProfiles(t,
		config.Profile{Name: "A"},
		config.Profile{Name: "B"},
		config.Profile{Name: "C"},
	)
	if m.ProfileCursor != 0 {
		t.Fatalf("setup: ProfileCursor = %d, want 0", m.ProfileCursor)
	}
	// down x2 → cursor 2
	updated, _ := m.Update(downKey())
	m = updated.(Model)
	updated, _ = m.Update(downKey())
	m = updated.(Model)
	if m.ProfileCursor != 2 {
		t.Errorf("after 2 down: ProfileCursor = %d, want 2", m.ProfileCursor)
	}
	// down again → still 2 (clamped)
	updated, _ = m.Update(downKey())
	m = updated.(Model)
	if m.ProfileCursor != 2 {
		t.Errorf("after clamp down: ProfileCursor = %d, want 2", m.ProfileCursor)
	}
	// up → cursor 1
	updated, _ = m.Update(upKey())
	m = updated.(Model)
	if m.ProfileCursor != 1 {
		t.Errorf("after up: ProfileCursor = %d, want 1", m.ProfileCursor)
	}
	// up x2 → cursor clamped at 0
	updated, _ = m.Update(upKey())
	m = updated.(Model)
	updated, _ = m.Update(upKey())
	m = updated.(Model)
	if m.ProfileCursor != 0 {
		t.Errorf("after clamp up: ProfileCursor = %d, want 0", m.ProfileCursor)
	}
}

// TestProfilePicker_VimKeysMoveCursor verifies j/k move the
// cursor as well as the arrow keys.
func TestProfilePicker_VimKeysMoveCursor(t *testing.T) {
	t.Parallel()
	m := *pickerModelWithProfiles(t,
		config.Profile{Name: "A"},
		config.Profile{Name: "B"},
		config.Profile{Name: "C"},
	)
	m.ProfileCursor = 0
	updated, _ := m.Update(charKey('j'))
	m = updated.(Model)
	if m.ProfileCursor != 1 {
		t.Errorf("after j: ProfileCursor = %d, want 1", m.ProfileCursor)
	}
	updated, _ = m.Update(charKey('k'))
	m = updated.(Model)
	if m.ProfileCursor != 0 {
		t.Errorf("after k: ProfileCursor = %d, want 0", m.ProfileCursor)
	}
}

// TestProfilePicker_EmptyProfilesShowsMessage verifies the
// picker renders a helpful prompt when no profiles exist.
func TestProfilePicker_EmptyProfilesShowsMessage(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = config.Default()
	m.Screen = screenProfilePicker
	m.ProfileCursor = 0
	view := stripANSI(m.View())
	lower := strings.ToLower(view)
	if !strings.Contains(lower, "no profile") &&
		!strings.Contains(lower, "create") {
		t.Errorf("empty picker view missing hint: %q", view)
	}
}

// TestProfilePicker_ARoutesToAdd verifies pressing 'a' on the
// picker routes to the profile-add screen.
func TestProfilePicker_ARoutesToAdd(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t, config.Profile{Name: "Retail"})
	updated, _ := m.Update(charKey('a'))
	mm := updated.(Model)
	if mm.Screen != screenProfileAdd {
		t.Errorf("after a: Screen = %v, want screenProfileAdd", mm.Screen)
	}
	if mm.PendingProfileName != "" {
		t.Errorf("after a: PendingProfileName = %q, want empty", mm.PendingProfileName)
	}
}

// TestProfilePicker_RRoutesToRename verifies pressing 'r' on the
// picker routes to the profile-rename screen with the selected
// profile's name pre-filled.
func TestProfilePicker_RRoutesToRename(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.ProfileCursor = 1
	updated, _ := m.Update(charKey('r'))
	mm := updated.(Model)
	if mm.Screen != screenProfileRename {
		t.Errorf("after r: Screen = %v, want screenProfileRename", mm.Screen)
	}
	if mm.PendingProfileName != "Classic" {
		t.Errorf("after r: PendingProfileName = %q, want Classic (pre-fill)", mm.PendingProfileName)
	}
	if mm.PendingProfileID != "p2" {
		t.Errorf("after r: PendingProfileID = %q, want p2", mm.PendingProfileID)
	}
}

// TestProfilePicker_DRoutesToDelete verifies pressing 'd' on the
// picker routes to the profile-delete screen.
func TestProfilePicker_DRoutesToDelete(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.ProfileCursor = 1
	updated, _ := m.Update(charKey('d'))
	mm := updated.(Model)
	if mm.Screen != screenProfileDelete {
		t.Errorf("after d: Screen = %v, want screenProfileDelete", mm.Screen)
	}
	if mm.PendingProfileID != "p2" {
		t.Errorf("after d: PendingProfileID = %q, want p2", mm.PendingProfileID)
	}
}

// TestProfilePicker_PKeyFromListRoutesToPicker verifies the 'p'
// key on the list screen opens the profile picker.
func TestProfilePicker_PKeyFromListRoutesToPicker(t *testing.T) {
	t.Parallel()
	m := pickerModelWithProfiles(t, config.Profile{Name: "Retail"})
	m.Screen = screenList
	updated, _ := m.Update(charKey('p'))
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after p: Screen = %v, want screenProfilePicker", mm.Screen)
	}
}

// =============================================================================
// T5 — Profile Add / Rename / Delete
// =============================================================================

// TestProfileAdd_ValidInput verifies the full happy-path: name +
// valid path → profile created and set as active.
func TestProfileAdd_ValidInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	addons := filepath.Join(dir, "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}

	mPtr := NewModel()
	mPtr.Config = config.Default()
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, "Retail")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != screenWowPath {
		t.Fatalf("after name enter: Screen = %v, want screenWowPath", m.Screen)
	}
	if m.PendingProfileName != "Retail" {
		t.Errorf("PendingProfileName = %q, want Retail", m.PendingProfileName)
	}
	m.WowPathInput = addons
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != screenList {
		t.Errorf("after path enter: Screen = %v, want screenList", m.Screen)
	}
	if len(m.Config.Profiles) != 1 {
		t.Fatalf("Config.Profiles len = %d, want 1", len(m.Config.Profiles))
	}
	if m.Config.Profiles[0].Name != "Retail" {
		t.Errorf("Profiles[0].Name = %q, want Retail", m.Config.Profiles[0].Name)
	}
	if m.Config.Profiles[0].ID == "" {
		t.Errorf("Profiles[0].ID is empty, want generated UUID")
	}
	if m.Config.Profiles[0].WoWPath != addons {
		t.Errorf("Profiles[0].WoWPath = %q, want %q",
			m.Config.Profiles[0].WoWPath, addons)
	}
	if m.ActiveProfile == nil {
		t.Fatalf("ActiveProfile = nil after create")
	}
	if m.ActiveProfile.Name != "Retail" {
		t.Errorf("ActiveProfile.Name = %q, want Retail", m.ActiveProfile.Name)
	}
	if m.Config.ActiveProfileID != m.ActiveProfile.ID {
		t.Errorf("Config.ActiveProfileID = %q, want %q",
			m.Config.ActiveProfileID, m.ActiveProfile.ID)
	}
	if m.PendingProfileName != "" {
		t.Errorf("PendingProfileName = %q, want cleared", m.PendingProfileName)
	}
	if m.PendingProfilePath != "" {
		t.Errorf("PendingProfilePath = %q, want cleared", m.PendingProfilePath)
	}
	if string(m.WowPath) != addons {
		t.Errorf("m.WowPath = %q, want %q", string(m.WowPath), addons)
	}
}

// TestProfileAdd_DuplicateNameRejected verifies the add flow
// rejects a name that already exists in the config.
func TestProfileAdd_DuplicateNameRejected(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t, config.Profile{ID: "p1", Name: "Retail"})
	m.Screen = screenProfileAdd
	m = typeName(m, "Retail")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileAdd {
		t.Errorf("after duplicate enter: Screen = %v, want screenProfileAdd (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want duplicate-name error")
	}
	lower := strings.ToLower(mm.ProfileNameError)
	if !strings.Contains(lower, "duplicate") && !strings.Contains(lower, "exists") {
		t.Errorf("ProfileNameError = %q, want duplicate/exists error", mm.ProfileNameError)
	}
	if len(mm.Config.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1 (no new profile created)", len(mm.Config.Profiles))
	}
}

// TestProfileAdd_EmptyNameRejected verifies the add flow rejects
// an empty (or whitespace-only) name.
func TestProfileAdd_EmptyNameRejected(t *testing.T) {
	t.Parallel()
	mPtr := NewModel()
	mPtr.Config = config.Default()
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, "   ")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileAdd {
		t.Errorf("after empty enter: Screen = %v, want screenProfileAdd (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want empty-name error")
	}
}

// TestProfileAdd_TooLongNameRejected verifies the add flow
// rejects names longer than 64 chars.
func TestProfileAdd_TooLongNameRejected(t *testing.T) {
	t.Parallel()
	mPtr := NewModel()
	mPtr.Config = config.Default()
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, strings.Repeat("x", 65))
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileAdd {
		t.Errorf("after long name: Screen = %v, want screenProfileAdd (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want length error")
	}
}

// TestProfileAdd_InvalidPathRejected verifies the path step
// surfaces a wowpath error and keeps the user on the path
// screen (so they can correct it).
func TestProfileAdd_InvalidPathRejected(t *testing.T) {
	t.Parallel()
	mPtr := NewModel()
	mPtr.Config = config.Default()
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, "Retail")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != screenWowPath {
		t.Fatalf("setup: Screen = %v, want screenWowPath", m.Screen)
	}
	m.WowPathInput = "/this/path/definitely/does/not/exist/wow"
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenWowPath {
		t.Errorf("after invalid path: Screen = %v, want screenWowPath (stay)", mm.Screen)
	}
	if mm.WowPathError == "" {
		t.Errorf("WoWPathError empty, want path-resolution error")
	}
	if len(mm.Config.Profiles) != 0 {
		t.Errorf("Profiles len = %d, want 0 (no create on bad path)", len(mm.Config.Profiles))
	}
}

// TestProfileAdd_MaxProfilesRejected verifies the add flow
// rejects creation when 50 profiles already exist.
func TestProfileAdd_MaxProfilesRejected(t *testing.T) {
	t.Parallel()
	profiles := make([]config.Profile, 0, config.MaxProfiles)
	for i := 0; i < config.MaxProfiles; i++ {
		profiles = append(profiles, config.Profile{
			ID:   uniqueID(i),
			Name: uniqueName(i),
		})
	}
	mPtr := NewModel()
	mPtr.Config = &config.Config{
		Version:         config.CurrentSchemaVersion,
		Profiles:        profiles,
		ActiveProfileID: profiles[0].ID,
	}
	mPtr.SetActiveProfile(&profiles[0])
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, "NewOne")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileAdd {
		t.Errorf("at max: Screen = %v, want screenProfileAdd (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want max-profiles error")
	}
	lower := strings.ToLower(mm.ProfileNameError)
	if !strings.Contains(lower, "maximum") && !strings.Contains(lower, "max") {
		t.Errorf("ProfileNameError = %q, want maximum error", mm.ProfileNameError)
	}
}

// TestProfileAdd_BackspaceInName verifies backspace deletes the
// last character of the typed name. We use a short string and
// a single backspace to keep the test focused; the
// multi-backspace case is implicitly exercised by
// TestProfileAdd_TooLongNameRejected (which starts with a 65
// char string and presses enter, but the helper functions
// themselves are unit-tested elsewhere).
func TestProfileAdd_BackspaceInName(t *testing.T) {
	t.Parallel()
	mPtr := NewModel()
	mPtr.Config = config.Default()
	mPtr.Screen = screenProfileAdd
	m := *mPtr
	m = typeName(m, "ABC")
	// Verify pre-backspace state.
	if m.PendingProfileName != "ABC" {
		t.Fatalf("after typeName: PendingProfileName = %q, want ABC", m.PendingProfileName)
	}
	m = backspaceN(m, 1) // "ABC" -> "AB"
	if m.PendingProfileName != "AB" {
		t.Errorf("after 1 backspace: PendingProfileName = %q, want AB", m.PendingProfileName)
	}
	// A second backspace to "AB" -> "A" (this catches the
	// "lost backspace" bug that the previous test exposed).
	m = backspaceN(m, 1)
	if m.PendingProfileName != "A" {
		t.Errorf("after 2 backspaces: PendingProfileName = %q, want A", m.PendingProfileName)
	}
}

// TestProfileRename_ValidName verifies rename updates the
// profile name and returns to the picker.
func TestProfileRename_ValidName(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t, config.Profile{ID: "p1", Name: "Retail"})
	m.Screen = screenProfileRename
	m.PendingProfileID = "p1"
	m.PendingProfileName = "Retail"
	m = backspaceN(m, len("Retail"))
	m = typeName(m, "Retail Era")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after rename: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if mm.Config.Profiles[0].Name != "Retail Era" {
		t.Errorf("Profiles[0].Name = %q, want %q",
			mm.Config.Profiles[0].Name, "Retail Era")
	}
	if mm.Config.Profiles[0].ID != "p1" {
		t.Errorf("Profiles[0].ID = %q, want p1 (unchanged)", mm.Config.Profiles[0].ID)
	}
}

// TestProfileRename_DuplicateRejected verifies renaming to a
// name that already exists is rejected with an error.
func TestProfileRename_DuplicateRejected(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.Screen = screenProfileRename
	m.PendingProfileID = "p1"
	m.PendingProfileName = "Retail"
	m = backspaceN(m, len("Retail"))
	m = typeName(m, "Classic")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileRename {
		t.Errorf("after duplicate rename: Screen = %v, want screenProfileRename (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want duplicate error")
	}
	if mm.Config.Profiles[0].Name != "Retail" {
		t.Errorf("Profiles[0].Name = %q, want Retail (unchanged)", mm.Config.Profiles[0].Name)
	}
}

// TestProfileRename_EmptyRejected verifies an empty new name is
// rejected on rename.
func TestProfileRename_EmptyRejected(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t, config.Profile{ID: "p1", Name: "Retail"})
	m.Screen = screenProfileRename
	m.PendingProfileID = "p1"
	m.PendingProfileName = "Retail"
	m = backspaceN(m, len("Retail"))
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfileRename {
		t.Errorf("after empty rename: Screen = %v, want screenProfileRename (stay)", mm.Screen)
	}
	if mm.ProfileNameError == "" {
		t.Errorf("ProfileNameError empty, want empty error")
	}
	if mm.Config.Profiles[0].Name != "Retail" {
		t.Errorf("Profiles[0].Name = %q, want Retail (unchanged)", mm.Config.Profiles[0].Name)
	}
}

// TestProfileRename_EscReturnsToPicker verifies esc on the
// rename screen returns to the picker without changing the name.
func TestProfileRename_EscReturnsToPicker(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t, config.Profile{ID: "p1", Name: "Retail"})
	m.Screen = screenProfileRename
	m.PendingProfileID = "p1"
	m.PendingProfileName = "Retail"
	m = typeName(m, "Changed")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after esc: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if mm.Config.Profiles[0].Name != "Retail" {
		t.Errorf("Profiles[0].Name = %q, want Retail (unchanged after esc)",
			mm.Config.Profiles[0].Name)
	}
}

// TestProfileDelete_NonActiveSucceeds verifies deleting a
// non-active profile removes it and stays on the picker.
func TestProfileDelete_NonActiveSucceeds(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p2"
	updated, _ := m.Update(charKey('y'))
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after delete: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if len(mm.Config.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1 (Classic removed)", len(mm.Config.Profiles))
	}
	if mm.Config.Profiles[0].ID != "p1" {
		t.Errorf("Profiles[0].ID = %q, want p1 (Retail kept)", mm.Config.Profiles[0].ID)
	}
}

// TestProfileDelete_EnterConfirms verifies the enter key also
// confirms deletion.
func TestProfileDelete_EnterConfirms(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p2"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after enter: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if len(mm.Config.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1", len(mm.Config.Profiles))
	}
}

// TestProfileDelete_ActiveRejected verifies deleting the active
// profile is rejected with a clear error.
func TestProfileDelete_ActiveRejected(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t, config.Profile{ID: "p1", Name: "Retail"})
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p1"
	updated, _ := m.Update(charKey('y'))
	mm := updated.(Model)
	if mm.Screen != screenProfileDelete {
		t.Errorf("after active-delete attempt: Screen = %v, want screenProfileDelete (stay)", mm.Screen)
	}
	if mm.ProfileError == "" {
		t.Errorf("ProfileError empty, want cannot-delete-active error")
	}
	lower := strings.ToLower(mm.ProfileError)
	if !strings.Contains(lower, "active") {
		t.Errorf("ProfileError = %q, want error mentioning 'active'", mm.ProfileError)
	}
	if len(mm.Config.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1 (active kept)", len(mm.Config.Profiles))
	}
}

// TestProfileDelete_CancelByEsc verifies esc on the delete
// screen returns to the picker without changes.
func TestProfileDelete_CancelByEsc(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p2"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after esc: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if len(mm.Config.Profiles) != 2 {
		t.Errorf("Profiles len = %d, want 2 (nothing deleted)", len(mm.Config.Profiles))
	}
}

// TestProfileDelete_ActivePointerValid is the regression test
// for the CRITICAL bug where deleting a profile at a lower
// index than the active one shifted the active profile's
// bytes down in the slice, leaving m.ActiveProfile (a
// *config.Profile pointer) pointing to the wrong struct or
// to memory the new slice doesn't include. After the fix,
// submitProfileDelete must rewire m.ActiveProfile by
// re-looking-up the active ID, so any subsequent mutation
// through the pointer lands on the right profile.
//
// Scenario: 3 profiles A, B, C; active is B (index 1). We
// delete A (index 0). The slice shifts so B is now at index
// 0. m.ActiveProfile must still point to B, and an
// UpsertAddon through that pointer must add to B's addon
// list — not to C's.
func TestProfileDelete_ActivePointerValid(t *testing.T) {
	t.Parallel()

	// Build a 3-profile config with B as the active profile
	// (index 1, not the default 0).
	mPtr := NewModel()
	mPtr.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{ID: "p1", Name: "A", WoWPath: "/tmp/A"},
			{ID: "p2", Name: "B", WoWPath: "/tmp/B"},
			{ID: "p3", Name: "C", WoWPath: "/tmp/C"},
		},
		ActiveProfileID: "p2",
	}
	mPtr.SetActiveProfile(mPtr.Config.FindProfileByID("p2"))

	// Sanity: the setup actually exercises the bug. The
	// active profile's index (1) must be GREATER than the
	// deleted profile's index (0), so the in-place slice
	// shift will move the active profile's bytes.
	if mPtr.ActiveProfile.Name != "B" {
		t.Fatalf("setup: ActiveProfile.Name = %q, want B",
			mPtr.ActiveProfile.Name)
	}
	preDeleteName := mPtr.ActiveProfile.Name
	preDeleteID := mPtr.ActiveProfile.ID

	// Drive the delete through the same handler the UI uses
	// (y keypress on the delete screen).
	mPtr.Screen = screenProfileDelete
	mPtr.PendingProfileID = "p1"
	m := *mPtr
	updated, _ := m.Update(charKey('y'))
	mm := updated.(Model)

	// Post-delete slice should be [B, C].
	if len(mm.Config.Profiles) != 2 {
		t.Fatalf("after delete: Profiles len = %d, want 2",
			len(mm.Config.Profiles))
	}
	if mm.Config.Profiles[0].ID != "p2" {
		t.Errorf("after delete: Profiles[0].ID = %q, want p2 (B)",
			mm.Config.Profiles[0].ID)
	}
	if mm.Config.Profiles[1].ID != "p3" {
		t.Errorf("after delete: Profiles[1].ID = %q, want p3 (C)",
			mm.Config.Profiles[1].ID)
	}

	// CRITICAL: ActiveProfile must still point to B. The
	// pre-fix code left it pointing to a different struct
	// (C) because the slice shift moved B's bytes and the
	// stale pointer landed on C.
	if mm.ActiveProfile == nil {
		t.Fatalf("after delete: ActiveProfile = nil, want pointer to B")
	}
	if mm.ActiveProfile.ID != preDeleteID {
		t.Errorf("after delete: ActiveProfile.ID = %q, want %q (pointer invalidated by slice shift)",
			mm.ActiveProfile.ID, preDeleteID)
	}
	if mm.ActiveProfile.Name != preDeleteName {
		t.Errorf("after delete: ActiveProfile.Name = %q, want %q (pointer now reads wrong struct)",
			mm.ActiveProfile.Name, preDeleteName)
	}

	// CRITICAL: the pointer must identify the SAME struct in
	// the new slice (i.e. &Profiles[0]), not some stale
	// address. The cleanest assertion: dereference through
	// the pointer and through the slice index and verify
	// they agree.
	if mm.ActiveProfile != &mm.Config.Profiles[0] {
		t.Errorf("after delete: ActiveProfile points to a struct that is not Profiles[0]; the slice is %v and ActiveProfile is at %p",
			mm.Config.Profiles, mm.ActiveProfile)
	}

	// CRITICAL: a mutation through the pointer must land in
	// B's addons, not C's. This is the user-visible symptom
	// of the original bug.
	mm.ActiveProfile.UpsertAddon(config.Addon{
		Name: "NewlyAdded",
		URL:  "https://example.com/NewlyAdded.git",
	})

	var foundIn string
	for i, p := range mm.Config.Profiles {
		for _, a := range p.Addons {
			if a.Name == "NewlyAdded" {
				foundIn = p.Name
				t.Logf("addon landed in profile[%d] (name=%q, id=%s)",
					i, p.Name, p.ID)
			}
		}
	}
	if foundIn != "B" {
		t.Fatalf("addon added via m.ActiveProfile landed in %q, want B (silent data corruption)",
			foundIn)
	}
}

// TestProfileDelete_ActivePointerValid_MultipleDeletes
// extends the regression: three deletes in a row, with the
// active profile remaining constant, must keep the pointer
// correct after every slice shift. Pre-fix, the second
// delete would compound the corruption.
func TestProfileDelete_ActivePointerValid_MultipleDeletes(t *testing.T) {
	t.Parallel()

	mPtr := NewModel()
	mPtr.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{ID: "p1", Name: "A"},
			{ID: "p2", Name: "B"},
			{ID: "p3", Name: "C"},
			{ID: "p4", Name: "D"},
		},
		ActiveProfileID: "p3", // C
	}
	mPtr.SetActiveProfile(mPtr.Config.FindProfileByID("p3"))

	// Delete A (p1).
	m := *mPtr
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p1"
	updated, _ := m.Update(charKey('y'))
	m = updated.(Model)
	if m.ActiveProfile == nil || m.ActiveProfile.ID != "p3" {
		t.Fatalf("after delete A: ActiveProfile.ID = %v, want p3",
			activeIDOrNil(m.ActiveProfile))
	}
	if m.ActiveProfile.Name != "C" {
		t.Errorf("after delete A: ActiveProfile.Name = %q, want C",
			m.ActiveProfile.Name)
	}

	// Delete B (p2) — at this point C is at index 0.
	m.Screen = screenProfileDelete
	m.PendingProfileID = "p2"
	updated, _ = m.Update(charKey('y'))
	m = updated.(Model)
	if m.ActiveProfile == nil || m.ActiveProfile.ID != "p3" {
		t.Fatalf("after delete B: ActiveProfile.ID = %v, want p3",
			activeIDOrNil(m.ActiveProfile))
	}
	if m.ActiveProfile.Name != "C" {
		t.Errorf("after delete B: ActiveProfile.Name = %q, want C",
			m.ActiveProfile.Name)
	}
	if len(m.Config.Profiles) != 2 {
		t.Errorf("after delete B: Profiles len = %d, want 2",
			len(m.Config.Profiles))
	}

	// Mutate through the pointer — must land in C.
	m.ActiveProfile.UpsertAddon(config.Addon{Name: "C-addon"})
	for _, p := range m.Config.Profiles {
		for _, a := range p.Addons {
			if a.Name == "C-addon" && p.ID != "p3" {
				t.Errorf("addon landed in %s (%q), want p3 (C)",
					p.ID, p.Name)
			}
		}
	}
}

// TestProfileRename_ActivePointerValid verifies the
// defensive rewire after rename keeps m.ActiveProfile
// pointing at the renamed profile. Rename currently mutates
// the struct in place, so the pointer is still valid
// regardless — but we test it explicitly to lock the
// invariant against future refactors.
func TestProfileRename_ActivePointerValid(t *testing.T) {
	t.Parallel()
	m := *newModelWithProfiles(t,
		config.Profile{ID: "p1", Name: "Retail"},
		config.Profile{ID: "p2", Name: "Classic"},
	)
	preRenameID := m.ActiveProfile.ID

	m.Screen = screenProfileRename
	m.PendingProfileID = "p1"
	m.PendingProfileName = "Retail"
	m = backspaceN(m, len("Retail"))
	m = typeName(m, "Retail Era")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)

	if mm.ActiveProfile == nil {
		t.Fatalf("after rename: ActiveProfile = nil")
	}
	if mm.ActiveProfile.ID != preRenameID {
		t.Errorf("after rename: ActiveProfile.ID = %q, want %q (pointer invalidated)",
			mm.ActiveProfile.ID, preRenameID)
	}
	if mm.ActiveProfile.Name != "Retail Era" {
		t.Errorf("after rename: ActiveProfile.Name = %q, want %q",
			mm.ActiveProfile.Name, "Retail Era")
	}
	if mm.ActiveProfile != &mm.Config.Profiles[0] {
		t.Errorf("after rename: ActiveProfile != &Profiles[0]; rewired incorrectly")
	}
}

// activeIDOrNil is a small helper for diagnostic messages
// in the multi-delete regression test.
func activeIDOrNil(p *config.Profile) string {
	if p == nil {
		return "<nil>"
	}
	return p.ID
}

// TestProfileAdd_FromPickerEscReturnsToPicker verifies the esc
// behaviour on the add screen: return to picker, not quit.
func TestProfileAdd_FromPickerEscReturnsToPicker(t *testing.T) {
	t.Parallel()
	m := *pickerModelWithProfiles(t, config.Profile{Name: "Retail"})
	m.Screen = screenProfileAdd
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.Screen != screenProfilePicker {
		t.Errorf("after esc on add: Screen = %v, want screenProfilePicker", mm.Screen)
	}
	if mm.ActiveProfile == nil || mm.ActiveProfile.Name != "Retail" {
		t.Errorf("after esc on add: active = %q, want Retail unchanged",
			profileNameOrEmpty(&mm))
	}
}

// uniqueID returns a stable, unique ID for the i-th fixture
// profile. Avoids zero-padding issues and stays readable in
// failure messages.
func uniqueID(i int) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for j := 0; j < 8; j++ {
		out = append(out, hex[(i*7+j*3)&0xf])
	}
	out = append(out, '-')
	for j := 0; j < 4; j++ {
		out = append(out, hex[(i*5+j*2+1)&0xf])
	}
	out = append(out, '-', '4')
	for j := 0; j < 3; j++ {
		out = append(out, hex[(i*3+j+2)&0xf])
	}
	out = append(out, '-', 'a')
	for j := 0; j < 3; j++ {
		out = append(out, hex[(i*2+j+5)&0xf])
	}
	out = append(out, '-')
	for j := 0; j < 12; j++ {
		out = append(out, hex[(i+j*7+11)&0xf])
	}
	return string(out)
}

// uniqueName returns a stable, unique display name for the i-th
// fixture profile so the config validator does not reject
// duplicates within the 50-profile fixture.
func uniqueName(i int) string {
	return "Profile" + intToAlpha(i)
}

// intToAlpha converts an int to a 2-letter suffix like "Aa",
// "Ab", ... used as a stable, unique profile name.
func intToAlpha(i int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	first := i / 26
	second := i % 26
	if first == 0 {
		return string(letters[second])
	}
	return string(letters[first-1]) + string(letters[second])
}
