package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// =============================================================================
// T1 — Config v2 default, migration, validation
// =============================================================================

func TestDefault_HasV2SchemaVersion(t *testing.T) {
	t.Parallel()
	c := Default()
	if c.Version != 2 {
		t.Errorf("Default().Version = %d, want 2", c.Version)
	}
}

func TestDefault_EmptyProfilesAndNoActive(t *testing.T) {
	t.Parallel()
	c := Default()
	if c.Profiles == nil {
		t.Errorf("Default().Profiles = nil, want empty slice")
	}
	if len(c.Profiles) != 0 {
		t.Errorf("Default().Profiles len = %d, want 0", len(c.Profiles))
	}
	if c.ActiveProfileID != "" {
		t.Errorf("Default().ActiveProfileID = %q, want empty", c.ActiveProfileID)
	}
}

func TestSaveAndLoad_RoundTrip_V2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	in := &Config{
		Version: 2,
		Profiles: []Profile{
			{
				ID:      "fixed-uuid-for-test",
				Name:    "Default",
				WoWPath: "/home/user/wow/Interface/AddOns",
				Addons: []Addon{
					{
						Name:        "Atlas",
						URL:         "https://github.com/user/Atlas",
						TrackMode:   "branch",
						TrackTarget: "main",
						CurrentSHA:  "0123456789abcdef0123456789abcdef01234567",
					},
				},
			},
		},
		ActiveProfileID: "fixed-uuid-for-test",
	}

	if err := SaveTo(path, in); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("loaded Version = %d, want 2", got.Version)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("loaded Profiles len = %d, want 1", len(got.Profiles))
	}
	p := got.Profiles[0]
	if p.ID != "fixed-uuid-for-test" {
		t.Errorf("loaded Profiles[0].ID = %q, want fixed-uuid-for-test", p.ID)
	}
	if p.Name != "Default" {
		t.Errorf("loaded Profiles[0].Name = %q, want Default", p.Name)
	}
	if p.WoWPath != in.Profiles[0].WoWPath {
		t.Errorf("loaded WoWPath = %q, want %q", p.WoWPath, in.Profiles[0].WoWPath)
	}
	if len(p.Addons) != 1 {
		t.Fatalf("loaded Addons len = %d, want 1", len(p.Addons))
	}
	if !addonEqual(p.Addons[0], in.Profiles[0].Addons[0]) {
		t.Errorf("loaded Addons[0] = %+v, want %+v", p.Addons[0], in.Profiles[0].Addons[0])
	}
	if got.ActiveProfileID != "fixed-uuid-for-test" {
		t.Errorf("loaded ActiveProfileID = %q, want fixed-uuid-for-test", got.ActiveProfileID)
	}
}

func TestSave_WritesVersionFieldToJSON_V2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"version": 2`) {
		t.Errorf("config file missing version:2, got:\n%s", data)
	}
}

func TestLoad_MissingFileReturnsErrNotFound_V2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	cfg, err := LoadFrom(path)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("LoadFrom(missing) error = %v, want ErrNotFound", err)
	}
	if cfg == nil {
		t.Errorf("LoadFrom(missing) cfg = nil, want Default")
	}
	if cfg.Version != 2 {
		t.Errorf("LoadFrom(missing) Version = %d, want 2", cfg.Version)
	}
}

func TestLoad_CorruptJSONReturnsErrCorrupt_V2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadFrom(path)
	if !errors.Is(err, ErrCorrupt) {
		t.Errorf("LoadFrom(corrupt) error = %v, want ErrCorrupt", err)
	}
	if cfg == nil {
		t.Errorf("LoadFrom(corrupt) cfg = nil, want Default fallback")
	}
}

// TestLoad_LegacyV1FileMigratesToV2 is the headline migration test:
// a v1 on-disk file is detected, wrapped in a Default profile,
// and the file is rewritten as v2.
func TestLoad_LegacyV1FileMigratesToV2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	v1 := map[string]any{
		"version":  1,
		"wow_path": "/wow/legacy/Interface/AddOns",
		"addons": []any{
			map[string]any{
				"name":         "Bagnon",
				"url":          "https://github.com/tuller/Bagnon",
				"track_mode":   "branch",
				"track_target": "main",
				"current_sha":  "deadbeef",
			},
		},
	}
	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom(v1): %v", err)
	}
	if got.Version != 2 {
		t.Errorf("migrated Version = %d, want 2", got.Version)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("migrated Profiles len = %d, want 1", len(got.Profiles))
	}
	p := got.Profiles[0]
	if p.Name != "Default" {
		t.Errorf("migrated Profile.Name = %q, want Default", p.Name)
	}
	if p.WoWPath != "/wow/legacy/Interface/AddOns" {
		t.Errorf("migrated Profile.WoWPath = %q, want /wow/legacy/Interface/AddOns", p.WoWPath)
	}
	if len(p.Addons) != 1 || p.Addons[0].Name != "Bagnon" {
		t.Errorf("migrated addons lost: %+v", p.Addons)
	}
	if p.ID == "" {
		t.Errorf("migrated Profile.ID is empty, want a UUID")
	}
	if len(p.ID) != 36 {
		t.Errorf("migrated Profile.ID = %q (len %d), want a 36-char UUID", p.ID, len(p.ID))
	}
	if got.ActiveProfileID != p.ID {
		t.Errorf("migrated ActiveProfileID = %q, want %q (the new profile ID)", got.ActiveProfileID, p.ID)
	}
}

// TestLoad_V2FilePassesThroughIdempotent confirms that loading a v2
// file does NOT migrate it again. The file should be left untouched
// (no .v1-backup, profiles unchanged).
func TestLoad_V2FilePassesThroughIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "abc", Name: "Retail", WoWPath: "/wow/retail"},
			{ID: "def", Name: "Classic", WoWPath: "/wow/classic"},
		},
		ActiveProfileID: "abc",
	}
	if err := SaveTo(path, original); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom(v2): %v", err)
	}
	if len(got.Profiles) != 2 {
		t.Errorf("Profiles len = %d, want 2 (idempotent)", len(got.Profiles))
	}

	// The file should not have been rewritten.
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(afterBytes) != string(originalBytes) {
		t.Errorf("v2 file was rewritten on load (idempotency violated)")
	}

	// No backup file should exist.
	backupPath := path + ".v1-backup"
	if _, err := os.Stat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("v1-backup file exists for v2 config (unexpected): %s", backupPath)
	}
}

// TestLoad_V1FileWritesBackup confirms that loading a v1 file
// produces a .v1-backup file containing the original v1 bytes
// BEFORE the migration saves the v2 form.
func TestLoad_V1FileWritesBackup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	backupPath := path + ".v1-backup"

	v1 := map[string]any{
		"version":  1,
		"wow_path": "/wow/old",
		"addons":   []any{},
	}
	v1Bytes, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, v1Bytes, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := LoadFrom(path); err != nil {
		t.Fatalf("LoadFrom(v1): %v", err)
	}

	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if string(backupBytes) != string(v1Bytes) {
		t.Errorf("backup bytes differ from original v1 file:\n  got:  %s\n  want: %s", backupBytes, v1Bytes)
	}
}

// TestLoad_V1FilePreservesExistingBackup confirms that if a
// .v1-backup already exists, it is NOT overwritten. The migration
// still proceeds.
func TestLoad_V1FilePreservesExistingBackup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	backupPath := path + ".v1-backup"

	// Pre-existing backup with a sentinel value.
	sentinel := []byte(`{"sentinel":true}`)
	if err := os.WriteFile(backupPath, sentinel, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// v1 file with different content.
	v1 := map[string]any{"version": 1, "wow_path": "/wow/x", "addons": []any{}}
	v1Bytes, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, v1Bytes, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := LoadFrom(path); err != nil {
		t.Fatalf("LoadFrom(v1): %v", err)
	}

	// Backup must be unchanged.
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("existing backup was overwritten:\n  got:  %s\n  want: %s", got, sentinel)
	}
}

// TestLoad_FutureVersionReturnsErrFutureVersion confirms a v3+
// file is rejected with a clear error and is NOT migrated.
func TestLoad_FutureVersionReturnsErrFutureVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	future := map[string]any{"version": 3, "profiles": []any{}}
	data, err := json.Marshal(future)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadFrom(path)
	if !errors.Is(err, ErrFutureVersion) {
		t.Errorf("LoadFrom(v3) error = %v, want ErrFutureVersion", err)
	}
	if cfg != nil {
		t.Errorf("LoadFrom(v3) cfg = %+v, want nil", cfg)
	}
}

// TestLoad_LegacyV1FileMissingVersionMigrates covers the case where
// an old v1 file was written BEFORE versioning existed and has no
// version field at all. Treat it as v1 and migrate.
func TestLoad_LegacyV1FileMissingVersionMigrates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	legacy := map[string]any{
		"wow_path": "/wow/very-old",
		"addons":   []any{},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom(legacy no version): %v", err)
	}
	if got.Version != 2 {
		t.Errorf("migrated Version = %d, want 2", got.Version)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("migrated Profiles len = %d, want 1", len(got.Profiles))
	}
	if got.Profiles[0].WoWPath != "/wow/very-old" {
		t.Errorf("migrated WoWPath = %q, want /wow/very-old", got.Profiles[0].WoWPath)
	}
}

// TestValidate_RejectsDuplicateProfileNames covers REQ-7 from
// the profile-management spec.
func TestValidate_RejectsDuplicateProfileNames(t *testing.T) {
	t.Parallel()
	c := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "a", Name: "Retail"},
			{ID: "b", Name: "RETAIL"},
		},
	}
	if err := c.Validate(); err == nil {
		t.Errorf("Validate(duplicates) = nil, want error")
	}
}

func TestValidate_AllowsDistinctNames(t *testing.T) {
	t.Parallel()
	c := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "a", Name: "Retail"},
			{ID: "b", Name: "Classic"},
		},
		ActiveProfileID: "a",
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate(distinct) = %v, want nil", err)
	}
}

func TestValidate_ResetActiveIfMissing(t *testing.T) {
	t.Parallel()
	c := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "a", Name: "Retail"},
		},
		ActiveProfileID: "ghost-id",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.ActiveProfileID != "" {
		t.Errorf("ActiveProfileID = %q, want empty (reset)", c.ActiveProfileID)
	}
}

func TestValidate_KeepsActiveIfPresent(t *testing.T) {
	t.Parallel()
	c := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "a", Name: "Retail"},
		},
		ActiveProfileID: "a",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.ActiveProfileID != "a" {
		t.Errorf("ActiveProfileID = %q, want a (kept)", c.ActiveProfileID)
	}
}

// TestSave_NilAddonsInProfileCoercedToEmptySlice covers the v2
// "profiles with nil Addons are coerced to empty slice on save" rule.
func TestSave_NilAddonsInProfileCoercedToEmptySlice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: 2,
		Profiles: []Profile{
			{ID: "a", Name: "Retail"}, // Addons is nil
		},
		ActiveProfileID: "a",
	}
	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Profiles[0].Addons == nil {
		t.Errorf("Profile.Addons = nil, want []Addon{}")
	}
	if len(got.Profiles[0].Addons) != 0 {
		t.Errorf("Profile.Addons len = %d, want 0", len(got.Profiles[0].Addons))
	}
}

func TestSave_AtomicRenameNoLeftoverTemp_V2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("second SaveTo: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var tmpCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			tmpCount++
		}
	}
	if tmpCount != 0 {
		t.Errorf("leftover .tmp files = %d, want 0", tmpCount)
	}
}

func TestPath_LinuxHonoursXDGConfigHome_V2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("XDG only applies to non-Windows")
	}
	custom := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", custom)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(custom, AppDirName, FileName)
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestPath_LinuxFallsBackToHomeDotConfig_V2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("XDG fallback only applies to non-Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(home, ".config", AppDirName, FileName)
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestNewUUID_Format(t *testing.T) {
	t.Parallel()
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID: %v", err)
	}
	if len(id) != 36 {
		t.Errorf("UUID len = %d, want 36", len(id))
	}
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("UUID parts = %d, want 5", len(parts))
	}
	wantLens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != wantLens[i] {
			t.Errorf("UUID part %d len = %d, want %d", i, len(p), wantLens[i])
		}
	}
	// Version 4 marker is the first nibble of the 3rd group.
	if parts[2][0] != '4' {
		t.Errorf("UUID version nibble = %q, want 4", parts[2][0])
	}
	// Variant is 8/9/a/b as first char of 4th group.
	if c := parts[3][0]; c != '8' && c != '9' && c != 'a' && c != 'b' {
		t.Errorf("UUID variant nibble = %q, want 8/9/a/b", c)
	}
}

func TestNewUUID_Unique(t *testing.T) {
	t.Parallel()
	a, _ := newUUID()
	b, _ := newUUID()
	if a == b {
		t.Errorf("newUUID returned same value twice: %q", a)
	}
}

// =============================================================================
// T2 — Profile CRUD operations
// =============================================================================

// --- Profile.AddonByName / UpsertAddon / RemoveAddon ---

func TestProfile_AddonByName_Found(t *testing.T) {
	t.Parallel()
	p := &Profile{Addons: []Addon{
		{Name: "Atlas", URL: "u1"},
		{Name: "Bagnon", URL: "u2"},
	}}
	got := p.AddonByName("Bagnon")
	if got == nil {
		t.Fatalf("AddonByName(Bagnon) = nil")
	}
	if got.URL != "u2" {
		t.Errorf("AddonByName(Bagnon).URL = %q, want u2", got.URL)
	}
}

func TestProfile_AddonByName_NotFound(t *testing.T) {
	t.Parallel()
	p := &Profile{Addons: []Addon{{Name: "Atlas"}}}
	if got := p.AddonByName("Missing"); got != nil {
		t.Errorf("AddonByName(Missing) = %+v, want nil", got)
	}
}

func TestProfile_UpsertAddon_Inserts(t *testing.T) {
	t.Parallel()
	p := &Profile{}
	n := p.UpsertAddon(Addon{Name: "Atlas", URL: "u1"})
	if n != 1 {
		t.Errorf("UpsertAddon insert returned %d, want 1", n)
	}
	if len(p.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1", len(p.Addons))
	}
}

func TestProfile_UpsertAddon_UpdatesInPlace(t *testing.T) {
	t.Parallel()
	p := &Profile{Addons: []Addon{
		{Name: "Atlas", URL: "old", TrackMode: "branch", TrackTarget: "main"},
	}}
	p.UpsertAddon(Addon{Name: "Atlas", URL: "new", TrackMode: "release", TrackTarget: "v1"})

	if len(p.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1 (update should not append)", len(p.Addons))
	}
	if p.Addons[0].URL != "new" {
		t.Errorf("Addons[0].URL = %q, want new", p.Addons[0].URL)
	}
	if p.Addons[0].TrackMode != "release" {
		t.Errorf("Addons[0].TrackMode = %q, want release", p.Addons[0].TrackMode)
	}
}

func TestProfile_RemoveAddon(t *testing.T) {
	t.Parallel()
	p := &Profile{Addons: []Addon{{Name: "Atlas"}, {Name: "Bagnon"}}}
	if !p.RemoveAddon("Atlas") {
		t.Errorf("RemoveAddon(Atlas) = false, want true")
	}
	if len(p.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1", len(p.Addons))
	}
	if p.Addons[0].Name != "Bagnon" {
		t.Errorf("remaining Addon name = %q, want Bagnon", p.Addons[0].Name)
	}
	if p.RemoveAddon("Missing") {
		t.Errorf("RemoveAddon(Missing) = true, want false")
	}
}

// --- Config.FindProfileByID / FindProfileByName / ProfileNames ---

func TestConfig_FindProfileByID_Found(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{
		{ID: "id-1", Name: "Retail"},
		{ID: "id-2", Name: "Classic"},
	}}
	p := c.FindProfileByID("id-2")
	if p == nil {
		t.Fatalf("FindProfileByID(id-2) = nil")
	}
	if p.Name != "Classic" {
		t.Errorf("FindProfileByID(id-2).Name = %q, want Classic", p.Name)
	}
}

func TestConfig_FindProfileByID_NotFound(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{{ID: "id-1", Name: "Retail"}}}
	if p := c.FindProfileByID("missing"); p != nil {
		t.Errorf("FindProfileByID(missing) = %+v, want nil", p)
	}
}

func TestConfig_FindProfileByName_CaseInsensitive(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{
		{ID: "id-1", Name: "Retail"},
		{ID: "id-2", Name: "Classic"},
	}}
	p := c.FindProfileByName("RETAIL")
	if p == nil {
		t.Fatalf("FindProfileByName(RETAIL) = nil")
	}
	if p.ID != "id-1" {
		t.Errorf("FindProfileByName(RETAIL).ID = %q, want id-1", p.ID)
	}
}

func TestConfig_FindProfileByName_NotFound(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{{ID: "id-1", Name: "Retail"}}}
	if p := c.FindProfileByName("PrivateServer"); p != nil {
		t.Errorf("FindProfileByName(missing) = %+v, want nil", p)
	}
}

func TestConfig_ProfileNames(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{
		{Name: "Retail"},
		{Name: "Classic"},
		{Name: "Private"},
	}}
	got := c.ProfileNames()
	want := []string{"Retail", "Classic", "Private"}
	if len(got) != len(want) {
		t.Fatalf("ProfileNames len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ProfileNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestConfig_ProfileNames_Empty(t *testing.T) {
	t.Parallel()
	c := Default()
	got := c.ProfileNames()
	if len(got) != 0 {
		t.Errorf("ProfileNames on default = %v, want empty", got)
	}
}

// --- Config.RemoveProfile ---

func TestConfig_RemoveProfile_Success(t *testing.T) {
	t.Parallel()
	c := &Config{
		Profiles: []Profile{
			{ID: "a", Name: "Retail"},
			{ID: "b", Name: "Classic"},
		},
		ActiveProfileID: "a",
	}
	if err := c.RemoveProfile("b"); err != nil {
		t.Fatalf("RemoveProfile(b): %v", err)
	}
	if len(c.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1", len(c.Profiles))
	}
	if c.Profiles[0].ID != "a" {
		t.Errorf("remaining Profile.ID = %q, want a", c.Profiles[0].ID)
	}
}

func TestConfig_RemoveProfile_RejectsActive(t *testing.T) {
	t.Parallel()
	c := &Config{
		Profiles:        []Profile{{ID: "a", Name: "Retail"}},
		ActiveProfileID: "a",
	}
	err := c.RemoveProfile("a")
	if err == nil {
		t.Errorf("RemoveProfile(active) = nil, want error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "active") {
		t.Errorf("RemoveProfile(active) error = %q, want message mentioning 'active'", err)
	}
	if len(c.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1 (rejected, no removal)", len(c.Profiles))
	}
}

func TestConfig_RemoveProfile_NotFound(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{{ID: "a", Name: "Retail"}}}
	if err := c.RemoveProfile("missing"); err == nil {
		t.Errorf("RemoveProfile(missing) = nil, want error")
	}
}

// --- Config.RenameProfile ---

func TestConfig_RenameProfile_Success(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{
		{ID: "a", Name: "Retail"},
		{ID: "b", Name: "Classic"},
	}}
	if err := c.RenameProfile("a", "Retail New"); err != nil {
		t.Fatalf("RenameProfile: %v", err)
	}
	if c.Profiles[0].Name != "Retail New" {
		t.Errorf("Renamed name = %q, want Retail New", c.Profiles[0].Name)
	}
	if c.Profiles[0].ID != "a" {
		t.Errorf("ID changed during rename: %q", c.Profiles[0].ID)
	}
}

func TestConfig_RenameProfile_RejectsDuplicate(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{
		{ID: "a", Name: "Retail"},
		{ID: "b", Name: "Classic"},
	}}
	err := c.RenameProfile("a", "CLASSIC")
	if err == nil {
		t.Errorf("RenameProfile(duplicate) = nil, want error")
	}
	if c.Profiles[0].Name != "Retail" {
		t.Errorf("Name changed despite rejection: %q", c.Profiles[0].Name)
	}
}

func TestConfig_RenameProfile_NotFound(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{{ID: "a", Name: "Retail"}}}
	if err := c.RenameProfile("missing", "x"); err == nil {
		t.Errorf("RenameProfile(missing) = nil, want error")
	}
}

func TestConfig_RenameProfile_SameNameIsNoop(t *testing.T) {
	t.Parallel()
	c := &Config{Profiles: []Profile{{ID: "a", Name: "Retail"}}}
	// Renaming to the same name (case-insensitive) should not be
	// treated as a duplicate collision.
	if err := c.RenameProfile("a", "retail"); err != nil {
		t.Errorf("RenameProfile(same name, different case) = %v, want nil", err)
	}
}

// --- Max 50 profiles ---

func TestConfig_AddProfile_EnforcesMax50(t *testing.T) {
	t.Parallel()
	c := &Config{}
	for i := 0; i < MaxProfiles; i++ {
		name := fmt.Sprintf("Profile-%d", i)
		p := Profile{ID: fmt.Sprintf("id-%d", i), Name: name}
		if err := c.AddProfile(p); err != nil {
			t.Fatalf("AddProfile at %d: %v", i, err)
		}
	}
	err := c.AddProfile(Profile{ID: "overflow", Name: "Overflow"})
	if err == nil {
		t.Errorf("AddProfile over MaxProfiles = nil, want error")
	}
	if !errors.Is(err, ErrMaxProfiles) {
		t.Errorf("error = %v, want ErrMaxProfiles", err)
	}
	if len(c.Profiles) != MaxProfiles {
		t.Errorf("Profiles len = %d, want %d", len(c.Profiles), MaxProfiles)
	}
}

func TestConfig_AddProfile_RejectsDuplicateName(t *testing.T) {
	t.Parallel()
	c := &Config{}
	if err := c.AddProfile(Profile{ID: "a", Name: "Retail"}); err != nil {
		t.Fatalf("AddProfile first: %v", err)
	}
	err := c.AddProfile(Profile{ID: "b", Name: "RETAIL"})
	if err == nil {
		t.Errorf("AddProfile(duplicate) = nil, want error")
	}
	if len(c.Profiles) != 1 {
		t.Errorf("Profiles len = %d, want 1 (rejected)", len(c.Profiles))
	}
}

// --- helpers ---

// addonEqual compares two Addon values field by field, including
// the SubModules slice.
func addonEqual(a, b Addon) bool {
	if a.Name != b.Name || a.URL != b.URL ||
		a.TrackMode != b.TrackMode || a.TrackTarget != b.TrackTarget ||
		a.CurrentSHA != b.CurrentSHA || a.Version != b.Version ||
		a.LastUpdated != b.LastUpdated {
		return false
	}
	if len(a.SubModules) != len(b.SubModules) {
		return false
	}
	for i := range a.SubModules {
		if a.SubModules[i] != b.SubModules[i] {
			return false
		}
	}
	return true
}
