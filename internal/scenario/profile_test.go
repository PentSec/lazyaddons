// Package scenario contains integration-style tests that exercise
// multiple internal packages together. They live in their own
// package so unit tests stay fast and small, and so these tests
// can rely on real git when present (skipping in -short).
package scenario

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pentsec/lazyaddons/internal/config"
)

// TestProfileScenario_CreateSwitchDelete walks the full
// create-profile → switch → delete flow using the v2 Config
// shape and confirms the active profile pointer is wired
// correctly across all operations.
//
// Acceptance: SPEC profile-management SCENARIO-1 + SCENARIO-3
// + SCENARIO-5 and addon-list SCENARIO-1 + SCENARIO-2.
func TestProfileScenario_CreateSwitchDelete(t *testing.T) {
	t.Parallel()

	// 1. Start with a fresh v2 config: a single Default profile
	//    with one tracked addon.
	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{
				ID:      "p-default",
				Name:    "Default",
				WoWPath: "/tmp/wow/default/Interface/AddOns",
				Addons: []config.Addon{
					{Name: "Details", URL: "https://example.com/Details", TrackMode: "branch", TrackTarget: "main"},
				},
			},
		},
		ActiveProfileID: "p-default",
	}

	// 2. Add a second profile. The addons list on the new
	//    profile must be empty (profile scoping, not shared).
	if err := cfg.AddProfile(config.Profile{
		ID:      "p-private",
		Name:    "PrivateServer",
		WoWPath: "/tmp/wow/private/Interface/AddOns",
		Addons:  []config.Addon{},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("Profiles len = %d, want 2", len(cfg.Profiles))
	}

	// 3. Verify both profiles exist by name and the new one is
	//    empty (addons are isolated per profile, not global).
	def := cfg.FindProfileByName("Default")
	if def == nil {
		t.Fatalf("FindProfileByName(Default) = nil")
	}
	if def.AddonByName("Details") == nil {
		t.Errorf("Default profile missing 'Details' addon after create")
	}
	priv := cfg.FindProfileByName("PrivateServer")
	if priv == nil {
		t.Fatalf("FindProfileByName(PrivateServer) = nil")
	}
	if len(priv.Addons) != 0 {
		t.Errorf("PrivateServer profile has %d addons, want 0 (isolation)", len(priv.Addons))
	}

	// 4. Add an addon to the PrivateServer profile only.
	priv.Addons = append(priv.Addons, config.Addon{
		Name: "ElvUI", URL: "https://example.com/ElvUI",
		TrackMode: "branch", TrackTarget: "main",
	})
	if priv.AddonByName("ElvUI") == nil {
		t.Errorf("ElvUI not found in PrivateServer after add")
	}
	if def.AddonByName("ElvUI") != nil {
		t.Errorf("ElvUI leaked into Default profile (scope leak)")
	}

	// 5. Switch the active profile to PrivateServer.
	cfg.ActiveProfileID = priv.ID
	if priv.ID != cfg.ActiveProfileID {
		t.Errorf("ActiveProfileID = %q, want %q", cfg.ActiveProfileID, priv.ID)
	}

	// 6. The Default profile must still own its original addon
	//    (the switch only changes which is active, not content).
	if def.AddonByName("Details") == nil {
		t.Errorf("Default profile lost 'Details' after switch (state corruption)")
	}

	// 7. Switch back to Default so we can delete PrivateServer
	//    (cannot remove the active profile — REQ-4).
	cfg.ActiveProfileID = def.ID
	if err := cfg.RemoveProfile(priv.ID); err != nil {
		t.Fatalf("RemoveProfile(priv): %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("Profiles len after delete = %d, want 1", len(cfg.Profiles))
	}
	if cfg.FindProfileByName("PrivateServer") != nil {
		t.Errorf("PrivateServer still present after delete")
	}

	// 8. The Default profile is still intact and active.
	if cfg.ActiveProfileID != def.ID {
		t.Errorf("ActiveProfileID after delete = %q, want %q", cfg.ActiveProfileID, def.ID)
	}
	if def.AddonByName("Details") == nil {
		t.Errorf("Default profile lost 'Details' after deleting another profile")
	}
}

// TestProfileScenario_MigrationThenAddonOps simulates the
// realistic user journey: a user with a v1 on-disk config
// (single WoWPath + Addons list) opens lazyaddons, the v1 is
// migrated to v2 with a "Default" profile, and subsequent
// addon operations (upsert, remove) target the active
// profile's addons list, not a phantom global field.
//
// Acceptance: SPEC config-migration SCENARIO-1 + addon-list
// SCENARIO-3 + SCENARIO-4.
func TestProfileScenario_MigrationThenAddonOps(t *testing.T) {
	t.Parallel()

	// 1. Build a v1 on-disk file: {version: 1, wow_path, addons}.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	v1 := []byte(`{
		"version": 1,
		"wow_path": "/wow/legacy/Interface/AddOns",
		"addons": [
			{
				"name": "Bagnon",
				"url": "https://github.com/tuller/Bagnon",
				"track_mode": "branch",
				"track_target": "main"
			}
		]
	}`)
	if err := os.WriteFile(path, v1, 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	// 2. Load via the production path → triggers migration.
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom(v1): %v", err)
	}
	if cfg.Version != 2 {
		t.Errorf("migrated Version = %d, want 2", cfg.Version)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("migrated Profiles len = %d, want 1", len(cfg.Profiles))
	}
	migrated := cfg.Profiles[0]
	if migrated.Name != "Default" {
		t.Errorf("migrated Name = %q, want Default", migrated.Name)
	}
	if migrated.WoWPath != "/wow/legacy/Interface/AddOns" {
		t.Errorf("migrated WoWPath = %q, want legacy path", migrated.WoWPath)
	}
	if cfg.ActiveProfileID != migrated.ID {
		t.Errorf("ActiveProfileID = %q, want migrated profile ID %q", cfg.ActiveProfileID, migrated.ID)
	}
	if migrated.AddonByName("Bagnon") == nil {
		t.Errorf("migrated profile missing Bagnon")
	}

	// 3. v1 backup was written.
	backupPath := path + ".v1-backup"
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("v1 backup file missing: %v", err)
	}

	// 4. Upsert a new addon on the active profile. This is the
	//    pattern every operation uses after migration.
	migrated.UpsertAddon(config.Addon{
		Name: "WeakAuras", URL: "https://example.com/WeakAuras",
		TrackMode: "branch", TrackTarget: "main",
	})
	if migrated.AddonByName("WeakAuras") == nil {
		t.Errorf("UpsertAddon did not add WeakAuras")
	}
	if len(migrated.Addons) != 2 {
		t.Errorf("Addons len after upsert = %d, want 2", len(migrated.Addons))
	}

	// 5. Remove the original addon; the new one must survive.
	if !migrated.RemoveAddon("Bagnon") {
		t.Errorf("RemoveAddon(Bagnon) returned false")
	}
	if migrated.AddonByName("Bagnon") != nil {
		t.Errorf("Bagnon still present after remove")
	}
	if migrated.AddonByName("WeakAuras") == nil {
		t.Errorf("WeakAuras was lost during remove of Bagnon")
	}

	// 6. Persistence round-trip: save and re-load. The active
	//    profile ID must survive the round-trip and remain valid.
	if err := config.SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	reloaded, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (round-trip): %v", err)
	}
	if reloaded.ActiveProfileID != migrated.ID {
		t.Errorf("ActiveProfileID lost in round-trip: got %q, want %q",
			reloaded.ActiveProfileID, migrated.ID)
	}
	if reloaded.FindProfileByID(reloaded.ActiveProfileID) == nil {
		t.Errorf("ActiveProfileID no longer points to a valid profile after round-trip")
	}
}

// TestProfileScenario_DuplicateNameRejected verifies that the
// create flow rejects a name collision (case-insensitive).
//
// Acceptance: SPEC profile-management REQ-7 + EDGE-2.
func TestProfileScenario_DuplicateNameRejected(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{ID: "p1", Name: "Default", WoWPath: "/wow/default/Interface/AddOns"},
		},
		ActiveProfileID: "p1",
	}
	err := cfg.AddProfile(config.Profile{
		ID: "p2", Name: "DEFAULT", // different case, same effective name
		WoWPath: "/wow/x/Interface/AddOns",
	})
	if err == nil {
		t.Errorf("AddProfile(duplicate) = nil, want error")
	}
	if !errors.Is(err, config.ErrDuplicateProfile) {
		t.Errorf("error = %v, want ErrDuplicateProfile", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("Profiles len after rejection = %d, want 1", len(cfg.Profiles))
	}
}

// TestProfileScenario_DeleteActiveRejected verifies that the
// delete flow refuses to remove the active profile and surfaces
// a clear error.
//
// Acceptance: SPEC profile-management REQ-4 + SCENARIO-4.
func TestProfileScenario_DeleteActiveRejected(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{ID: "p1", Name: "Default", WoWPath: "/wow/default/Interface/AddOns"},
		},
		ActiveProfileID: "p1",
	}
	err := cfg.RemoveProfile("p1")
	if err == nil {
		t.Errorf("RemoveProfile(active) = nil, want error")
	}
	if !errors.Is(err, config.ErrActiveProfile) {
		t.Errorf("error = %v, want ErrActiveProfile", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("Profiles len after rejected delete = %d, want 1", len(cfg.Profiles))
	}
}

// TestProfileScenario_RenameActiveProfile confirms that
// renaming the active profile updates the name in place,
// keeps the ID intact, and the active pointer is still valid.
//
// Acceptance: SPEC profile-management SCENARIO-2.
func TestProfileScenario_RenameActiveProfile(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{
			{ID: "p1", Name: "Default", WoWPath: "/wow/default/Interface/AddOns"},
		},
		ActiveProfileID: "p1",
	}
	if err := cfg.RenameProfile("p1", "Retail"); err != nil {
		t.Fatalf("RenameProfile: %v", err)
	}
	if cfg.Profiles[0].Name != "Retail" {
		t.Errorf("Renamed Name = %q, want Retail", cfg.Profiles[0].Name)
	}
	if cfg.Profiles[0].ID != "p1" {
		t.Errorf("ID changed during rename: %q", cfg.Profiles[0].ID)
	}
	if cfg.ActiveProfileID != "p1" {
		t.Errorf("ActiveProfileID = %q, want p1 (unchanged)", cfg.ActiveProfileID)
	}
	if cfg.FindProfileByName("Retail") == nil {
		t.Errorf("FindProfileByName(Retail) = nil after rename")
	}
}
