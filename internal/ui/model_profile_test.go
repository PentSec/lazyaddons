package ui

import (
	"testing"

	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

// helper: build a Model with a v2 Config that has the given profiles
// and the first one marked active. Returns a model ready to query
// state from.
func newModelWithProfiles(t *testing.T, profiles ...config.Profile) *Model {
	t.Helper()
	ids := make([]string, 0, len(profiles))
	for i := range profiles {
		if profiles[i].ID == "" {
			profiles[i].ID = "test-id-" + profiles[i].Name
		}
		ids = append(ids, profiles[i].ID)
	}
	cfg := &config.Config{
		Version:  config.CurrentSchemaVersion,
		Profiles: profiles,
	}
	if len(ids) > 0 {
		cfg.ActiveProfileID = ids[0]
	}
	m := NewModel()
	m.Config = cfg
	// Wire the active profile via setActiveProfile.
	if active := cfg.FindProfileByID(cfg.ActiveProfileID); active != nil {
		m.SetActiveProfile(active)
	}
	return m
}

// TestStartScreen_WithProfilesReturnsList verifies that when at
// least one profile exists, StartScreen returns screenList.
func TestStartScreen_WithProfilesReturnsList(t *testing.T) {
	t.Parallel()
	m := newModelWithProfiles(t, config.Profile{
		Name:    "Retail",
		WoWPath: "/tmp/wow",
		Addons:  []config.Addon{{Name: "Atlas"}},
	})
	if got := m.StartScreen(); got != screenList {
		t.Errorf("StartScreen() = %d, want screenList (%d)", got, screenList)
	}
}

// TestStartScreen_NoProfilesReturnsProfileAdd verifies that on a
// fresh install (zero profiles) the model lands on screenProfileAdd
// so the user must create a profile before anything else.
func TestStartScreen_NoProfilesReturnsProfileAdd(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = config.Default() // zero profiles
	if got := m.StartScreen(); got != screenProfileAdd {
		t.Errorf("StartScreen() = %d, want screenProfileAdd (%d)", got, screenProfileAdd)
	}
}

// TestSetActiveProfile_SyncsWowPath verifies that setActiveProfile
// both stores the profile pointer AND syncs the convenience
// m.WowPath field so downstream code can read it without a nil
// dereference.
func TestSetActiveProfile_SyncsWowPath(t *testing.T) {
	t.Parallel()
	m := NewModel()
	p := &config.Profile{
		ID:      "abc",
		Name:    "Retail",
		WoWPath: "/home/user/wow/Interface/AddOns",
	}
	m.SetActiveProfile(p)
	if m.ActiveProfile != p {
		t.Errorf("ActiveProfile = %v, want %v", m.ActiveProfile, p)
	}
	if m.ActiveProfile.Name != "Retail" {
		t.Errorf("ActiveProfile.Name = %q, want Retail", m.ActiveProfile.Name)
	}
	if string(m.WowPath) != p.WoWPath {
		t.Errorf("WowPath = %q, want %q", string(m.WowPath), p.WoWPath)
	}
	if _, ok := any(m.WowPath).(wowpath.Path); !ok {
		t.Errorf("WowPath is not a wowpath.Path")
	}
}

// TestSelectedAddon_UsesActiveProfile verifies that selectedAddon
// reads from m.ActiveProfile.Addons (not m.Config.Addons), so each
// profile's addon list is properly isolated.
func TestSelectedAddon_UsesActiveProfile(t *testing.T) {
	t.Parallel()
	// Two profiles, each with their own addons. Active is the
	// first profile (Retail).
	m := newModelWithProfiles(t,
		config.Profile{
			Name: "Retail",
			Addons: []config.Addon{
				{Name: "Atlas"},
				{Name: "Bagnon"},
			},
		},
		config.Profile{
			Name: "Classic",
			Addons: []config.Addon{
				{Name: "Details"},
			},
		},
	)
	// Switch to the second profile and verify selectedAddon reads
	// from it.
	classic := m.Config.FindProfileByName("Classic")
	if classic == nil {
		t.Fatalf("Classic profile not found")
	}
	m.SetActiveProfile(classic)
	m.Selection = 0
	got := m.selectedAddon()
	if got == nil {
		t.Fatalf("selectedAddon() = nil, want addon from Classic profile")
	}
	if got.Name != "Details" {
		t.Errorf("selectedAddon().Name = %q, want Details (Classic's addon)", got.Name)
	}

	// Switch back to Retail and verify Bagnon is the second addon.
	retail := m.Config.FindProfileByName("Retail")
	m.SetActiveProfile(retail)
	m.Selection = 1
	got = m.selectedAddon()
	if got == nil {
		t.Fatalf("selectedAddon() = nil, want addon from Retail profile")
	}
	if got.Name != "Bagnon" {
		t.Errorf("selectedAddon().Name = %q, want Bagnon (Retail's addon)", got.Name)
	}
}
