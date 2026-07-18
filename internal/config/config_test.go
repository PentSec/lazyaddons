package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefault_HasCurrentSchemaVersion(t *testing.T) {
	t.Parallel()
	c := Default()
	if c.Version != CurrentSchemaVersion {
		t.Errorf("Default().Version = %d, want %d", c.Version, CurrentSchemaVersion)
	}
	if c.Addons == nil {
		t.Errorf("Default().Addons = nil, want empty slice")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	in := &Config{
		Version: CurrentSchemaVersion,
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
	}

	if err := SaveTo(path, in); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Version != CurrentSchemaVersion {
		t.Errorf("loaded Version = %d, want %d", got.Version, CurrentSchemaVersion)
	}
	if got.WoWPath != in.WoWPath {
		t.Errorf("loaded WoWPath = %q, want %q", got.WoWPath, in.WoWPath)
	}
	if len(got.Addons) != 1 {
		t.Fatalf("loaded Addons len = %d, want 1", len(got.Addons))
	}
	if !addonEqual(got.Addons[0], in.Addons[0]) {
		t.Errorf("loaded Addons[0] = %+v, want %+v", got.Addons[0], in.Addons[0])
	}
}

func TestSave_WritesVersionFieldToJSON(t *testing.T) {
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
	if !strings.Contains(string(data), `"version": 1`) {
		t.Errorf("config file missing version:1, got:\n%s", data)
	}
}

func TestLoad_MissingFileReturnsErrNotFound(t *testing.T) {
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
	if cfg.Version != CurrentSchemaVersion {
		t.Errorf("LoadFrom(missing) Version = %d, want %d", cfg.Version, CurrentSchemaVersion)
	}
}

func TestLoad_CorruptJSONReturnsErrCorrupt(t *testing.T) {
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

func TestLoad_LegacyFileWithNoVersionCoercedToCurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// A config without a version field — simulates an old on-disk
	// file written before the schema versioning was added.
	legacy := map[string]any{
		"wow_path": "/wow",
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
		t.Fatalf("LoadFrom(legacy): %v", err)
	}
	if got.Version != CurrentSchemaVersion {
		t.Errorf("legacy cfg Version = %d, want %d", got.Version, CurrentSchemaVersion)
	}
}

func TestSave_AtomicRenameNoLeftoverTemp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("second SaveTo: %v", err)
	}

	// Count leftover .tmp files; the atomic write should leave
	// exactly one .json file and no temps.
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

func TestSave_NilAddonsBecomesEmptySlice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Version: CurrentSchemaVersion, WoWPath: "/x"}
	if err := SaveTo(path, cfg); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Addons == nil {
		t.Errorf("Addons = nil, want []Addon{}")
	}
	if len(got.Addons) != 0 {
		t.Errorf("Addons len = %d, want 0", len(got.Addons))
	}
}

func TestPath_LinuxHonoursXDGConfigHome(t *testing.T) {
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

func TestPath_LinuxFallsBackToHomeDotConfig(t *testing.T) {
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

func TestConfig_AddonByName_Found(t *testing.T) {
	t.Parallel()
	c := &Config{Addons: []Addon{
		{Name: "Atlas", URL: "u1"},
		{Name: "Bagnon", URL: "u2"},
	}}
	got := c.AddonByName("Bagnon")
	if got == nil {
		t.Fatalf("AddonByName(Bagnon) = nil")
	}
	if got.URL != "u2" {
		t.Errorf("AddonByName(Bagnon).URL = %q, want %q", got.URL, "u2")
	}
}

func TestConfig_AddonByName_NotFound(t *testing.T) {
	t.Parallel()
	c := &Config{Addons: []Addon{{Name: "Atlas"}}}
	if got := c.AddonByName("Missing"); got != nil {
		t.Errorf("AddonByName(Missing) = %+v, want nil", got)
	}
}

func TestConfig_UpsertAddon_Inserts(t *testing.T) {
	t.Parallel()
	c := Default()
	n := c.UpsertAddon(Addon{Name: "Atlas", URL: "u1"})
	if n != 1 {
		t.Errorf("UpsertAddon insert returned %d, want 1", n)
	}
	if len(c.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1", len(c.Addons))
	}
}

func TestConfig_UpsertAddon_UpdatesInPlace(t *testing.T) {
	t.Parallel()
	c := &Config{Addons: []Addon{
		{Name: "Atlas", URL: "old", TrackMode: "branch", TrackTarget: "main"},
	}}
	c.UpsertAddon(Addon{Name: "Atlas", URL: "new", TrackMode: "release", TrackTarget: "v1"})

	if len(c.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1 (update should not append)", len(c.Addons))
	}
	if c.Addons[0].URL != "new" {
		t.Errorf("Addons[0].URL = %q, want %q", c.Addons[0].URL, "new")
	}
	if c.Addons[0].TrackMode != "release" {
		t.Errorf("Addons[0].TrackMode = %q, want release", c.Addons[0].TrackMode)
	}
}

func TestConfig_RemoveAddon(t *testing.T) {
	t.Parallel()
	c := &Config{Addons: []Addon{{Name: "Atlas"}, {Name: "Bagnon"}}}
	if !c.RemoveAddon("Atlas") {
		t.Errorf("RemoveAddon(Atlas) = false, want true")
	}
	if len(c.Addons) != 1 {
		t.Errorf("Addons len = %d, want 1", len(c.Addons))
	}
	if c.Addons[0].Name != "Bagnon" {
		t.Errorf("remaining Addon name = %q, want Bagnon", c.Addons[0].Name)
	}
	if c.RemoveAddon("Missing") {
		t.Errorf("RemoveAddon(Missing) = true, want false")
	}
}

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
