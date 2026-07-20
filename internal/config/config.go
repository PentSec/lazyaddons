// Package config persists the WoW installation path and the list of
// tracked addons per profile. The config is the single source of
// truth for addon state. Writes are atomic (temp file + rename) so
// a crash mid-write cannot corrupt the on-disk file.
package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// CurrentSchemaVersion is the version stored in every written config.
// Bump it whenever the on-disk schema changes incompatibly.
const CurrentSchemaVersion = 2

// MaxProfiles is the upper bound on the number of profiles a user
// can have. Enforced at create/rename time.
const MaxProfiles = 50

// AppDirName is the folder under which the config file lives.
const AppDirName = "lazyaddons"

// FileName is the config file name. JSON keeps it diff-friendly and
// human-inspectable for debugging.
const FileName = "config.json"

// backupSuffix is appended to the config path to produce the v1
// backup file written during migration.
const backupSuffix = ".v1-backup"

// ErrNotFound is returned by Load when the config file does not exist.
// Callers should treat this as a first-run scenario, not an error.
var ErrNotFound = errors.New("config: file not found")

// ErrCorrupt is returned by Load when the file exists but is not
// valid JSON. The error wraps the underlying parse error so the
// caller can inspect the cause.
var ErrCorrupt = errors.New("config: file is corrupt")

// ErrFutureVersion is returned by Load when the on-disk file uses a
// schema version newer than what this build understands. Callers
// should prompt the user to upgrade.
var ErrFutureVersion = errors.New("config: file uses a newer schema version")

// ErrMaxProfiles is returned by AddProfile when the max is hit.
var ErrMaxProfiles = errors.New("config: maximum number of profiles (50) reached")

// ErrDuplicateProfile is returned when a profile with the same name
// (case-insensitive) already exists.
var ErrDuplicateProfile = errors.New("config: duplicate profile name")

// ErrProfileNotFound is returned when a profile lookup by ID fails.
var ErrProfileNotFound = errors.New("config: profile not found")

// ErrActiveProfile is returned when trying to remove the active profile.
var ErrActiveProfile = errors.New("config: cannot remove the active profile")

// Config is the root on-disk structure (v2 schema). Every field is
// exported so the JSON encoder writes a stable, hand-inspectable shape.
type Config struct {
	// Version is the schema version, written to disk and checked on
	// load. A mismatch surfaces as a migration.
	Version int `json:"version"`

	// Profiles is the list of profiles, each with its own WoW
	// AddOns path and tracked addon set.
	Profiles []Profile `json:"profiles"`

	// ActiveProfileID is the UUID of the currently active profile,
	// or "" if no profile is active.
	ActiveProfileID string `json:"active_profile_id"`
}

// Profile is one WoW installation: a path plus its tracked addons.
type Profile struct {
	// ID is a UUID v4 generated at creation time. It is the
	// stable identifier used for active-profile tracking.
	ID string `json:"id"`

	// Name is the human-readable label shown in the picker and
	// footer. Must be unique across profiles (case-insensitive).
	Name string `json:"name"`

	// WoWPath is the absolute path to this profile's WoW AddOns
	// folder (e.g. /home/user/wow/Interface/AddOns).
	WoWPath string `json:"wow_path"`

	// Addons is the list of tracked addons for this profile.
	Addons []Addon `json:"addons"`
}

// Addon records a tracked repo. The shape matches the addon package's
// Addon struct but is duplicated here to keep the config package
// import-light (no addon → config cycle).
type Addon struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	TrackMode   string   `json:"track_mode"`   // "branch" | "release"
	TrackTarget string   `json:"track_target"` // branch name or tag
	CurrentSHA  string   `json:"current_sha"`
	Version     string   `json:"version"`               // from .toc ## Version:
	LastUpdated string   `json:"last_updated"`          // last commit date (YYYY-MM-DD)
	SubModules  []string `json:"sub_modules,omitempty"` // related addon dirs
}

// v1Config is the legacy on-disk shape. Used only by the migration
// path to read out the WoWPath + Addons that the new Profile
// structure wraps.
type v1Config struct {
	Version int     `json:"version"`
	WoWPath string  `json:"wow_path"`
	Addons  []Addon `json:"addons"`
}

// Default returns an empty v2 config.
func Default() *Config {
	return &Config{
		Version:  CurrentSchemaVersion,
		Profiles: []Profile{},
	}
}

// Path returns the absolute path to the config file. The directory
// follows XDG on Linux/macOS and APPDATA on Windows.
//
// The lookup is deterministic and does NOT depend on the
// environment's current working directory.
func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("config: cannot resolve config dir: %w", err)
	}
	return filepath.Join(dir, AppDirName, FileName), nil
}

// configDir returns the OS-appropriate parent directory for our
// config. We mirror the behaviour of os.UserConfigDir() and
// honour $XDG_CONFIG_HOME on Linux.
func configDir() (string, error) {
	if runtime.GOOS == "windows" {
		// On Windows, os.UserConfigDir returns %APPDATA% which is
		// exactly the spec's "Windows config path" target.
		base, err := os.UserConfigDir()
		if err != nil {
			return base, nil
		}
		return base, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// Load reads and decodes the config from Path(). A missing file
// returns a Default() config and ErrNotFound. A corrupt file
// returns ErrNotFound wrapped with ErrCorrupt so callers can
// distinguish the two cases.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(p)
}

// LoadFrom is the same as Load but reads from an explicit path.
// It exists so tests can use t.TempDir() fixtures.
//
// LoadFrom transparently migrates v1 (or version-less legacy)
// files to v2 in-place and returns the migrated config.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), ErrNotFound
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		// Return Default so the caller can offer to overwrite
		// rather than seeing a nil config.
		return Default(), fmt.Errorf("%w: %v", ErrCorrupt, err)
	}

	if cfg.Version > CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: file is v%d, this build only supports v%d",
			ErrFutureVersion, cfg.Version, CurrentSchemaVersion)
	}

	if cfg.Version <= 1 {
		// v1 (or version-less legacy) file: parse, back up, migrate.
		return migrateV1ToV2(data, path)
	}

	// v2 path: normalise and validate.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// migrateV1ToV2 reads a v1 on-disk config from raw bytes, writes a
// .v1-backup file (preserving any existing backup), and saves the
// migrated v2 config back to disk. The migrated config is returned
// to the caller.
//
// The migration wraps the legacy top-level WoWPath + Addons into a
// single profile named "Default" and marks it active.
func migrateV1ToV2(data []byte, path string) (*Config, error) {
	var v1 v1Config
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("config: parse v1 during migration: %w", err)
	}

	// Write backup before any modification. Skip if a backup
	// already exists — never overwrite the user's safety net.
	backupPath := path + backupSuffix
	if _, err := os.Stat(backupPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(backupPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("config: write v1 backup %s: %w", backupPath, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("config: stat backup %s: %w", backupPath, err)
	}

	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("config: generate profile id: %w", err)
	}

	cfg := &Config{
		Version: CurrentSchemaVersion,
		Profiles: []Profile{
			{
				ID:      id,
				Name:    "Default",
				WoWPath: v1.WoWPath,
				Addons:  v1.Addons,
			},
		},
		ActiveProfileID: id,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validate migrated config: %w", err)
	}

	if err := SaveTo(path, cfg); err != nil {
		return nil, fmt.Errorf("config: save migrated config: %w", err)
	}
	return cfg, nil
}

// Save atomically writes the config to Path(). It creates the
// containing directory if it does not exist. The write is durable:
// it fsyncs the temp file, renames, then fsyncs the parent
// directory.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return SaveTo(p, cfg)
}

// SaveTo is the same as Save but writes to an explicit path.
func SaveTo(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config: cannot save nil config")
	}
	cfg.Version = CurrentSchemaVersion
	// Coerce nil Addons slices on every profile to empty, so the
	// on-disk shape is always `[]` rather than `null`.
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Addons == nil {
			cfg.Profiles[i].Addons = []Addon{}
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("config: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("config: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("config: close temp: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("config: rename %s -> %s: %w", tmpName, path, err)
	}

	// Best-effort fsync of the directory so the rename is durable
	// across a power cut. Ignored on Windows where directory
	// fsync is not supported.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Validate normalises the config in-place (coerce nil Addons, reset
// stale ActiveProfileID) and returns an error if the config is
// structurally invalid (e.g. duplicate profile names).
func (c *Config) Validate() error {
	seen := make(map[string]struct{}, len(c.Profiles))
	for i := range c.Profiles {
		if c.Profiles[i].Addons == nil {
			c.Profiles[i].Addons = []Addon{}
		}
		key := strings.ToLower(c.Profiles[i].Name)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%w: %q", ErrDuplicateProfile, c.Profiles[i].Name)
		}
		seen[key] = struct{}{}
	}

	if c.ActiveProfileID != "" && c.FindProfileByID(c.ActiveProfileID) == nil {
		c.ActiveProfileID = ""
	}
	return nil
}

// =============================================================================
// Profile methods (T2)
// =============================================================================

// AddonByName returns a pointer to the named addon in the profile
// (case-sensitive) or nil if not present. The returned pointer
// aliases the slice element so callers can mutate the entry
// directly before re-saving.
func (p *Profile) AddonByName(name string) *Addon {
	for i := range p.Addons {
		if p.Addons[i].Name == name {
			return &p.Addons[i]
		}
	}
	return nil
}

// AddonIndex returns the index of the named addon in the profile's
// slice, or -1 if not present.
func (p *Profile) AddonIndex(name string) int {
	for i := range p.Addons {
		if p.Addons[i].Name == name {
			return i
		}
	}
	return -1
}

// UpsertAddon inserts or updates the named addon, preserving the
// existing entry's position. It returns the new count of addons.
func (p *Profile) UpsertAddon(a Addon) int {
	for i := range p.Addons {
		if p.Addons[i].Name == a.Name {
			p.Addons[i] = a
			return len(p.Addons)
		}
	}
	p.Addons = append(p.Addons, a)
	return len(p.Addons)
}

// RemoveAddon deletes the named addon from the profile. It returns
// true if an entry was removed.
func (p *Profile) RemoveAddon(name string) bool {
	for i := range p.Addons {
		if p.Addons[i].Name == name {
			p.Addons = append(p.Addons[:i], p.Addons[i+1:]...)
			return true
		}
	}
	return false
}

// =============================================================================
// Config profile lookups + CRUD (T2)
// =============================================================================

// FindProfileByID returns a pointer to the profile with the given
// UUID, or nil if not present.
func (c *Config) FindProfileByID(id string) *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].ID == id {
			return &c.Profiles[i]
		}
	}
	return nil
}

// FindProfileByName returns a pointer to the first profile whose
// name matches the given name case-insensitively, or nil if not
// present.
func (c *Config) FindProfileByName(name string) *Profile {
	target := strings.ToLower(name)
	for i := range c.Profiles {
		if strings.ToLower(c.Profiles[i].Name) == target {
			return &c.Profiles[i]
		}
	}
	return nil
}

// ProfileNames returns the names of all profiles in declaration
// order. The returned slice is a fresh copy; callers may mutate it
// without affecting the underlying Profiles.
func (c *Config) ProfileNames() []string {
	out := make([]string, 0, len(c.Profiles))
	for _, p := range c.Profiles {
		out = append(out, p.Name)
	}
	return out
}

// AddProfile appends a new profile. It enforces the max-profiles
// limit and rejects duplicate names (case-insensitive). On
// success, the new profile is stored in cfg.Profiles.
func (c *Config) AddProfile(p Profile) error {
	if len(c.Profiles) >= MaxProfiles {
		return ErrMaxProfiles
	}
	if c.FindProfileByName(p.Name) != nil {
		return fmt.Errorf("%w: %q", ErrDuplicateProfile, p.Name)
	}
	c.Profiles = append(c.Profiles, p)
	return nil
}

// RemoveProfile deletes the profile with the given id. It refuses
// to remove the active profile — switch first.
func (c *Config) RemoveProfile(id string) error {
	if id == c.ActiveProfileID {
		return fmt.Errorf("%w: %q is the active profile", ErrActiveProfile, id)
	}
	for i := range c.Profiles {
		if c.Profiles[i].ID == id {
			c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: id %q", ErrProfileNotFound, id)
}

// RenameProfile changes the name of the profile with the given id.
// It rejects names that collide with another profile (case-insensitive)
// and treats renaming to the same name (case-insensitive) as a no-op.
func (c *Config) RenameProfile(id, newName string) error {
	target := c.FindProfileByID(id)
	if target == nil {
		return fmt.Errorf("%w: id %q", ErrProfileNotFound, id)
	}
	if strings.EqualFold(target.Name, newName) {
		return nil
	}
	if c.FindProfileByName(newName) != nil {
		return fmt.Errorf("%w: %q", ErrDuplicateProfile, newName)
	}
	target.Name = newName
	return nil
}

// =============================================================================
// Misc
// =============================================================================

// mu guards filesystem mutation in helpers that read-modify-write
// outside the package (e.g. from tests using goroutines). It is not
// strictly required for the single-threaded TUI but the field is
// exported for future plugin hosts that may call Save concurrently.
var mu sync.Mutex

// Lock is exported so callers that race Load + Save can serialise
// them. Most callers can ignore it.
func Lock()   { mu.Lock() }
func Unlock() { mu.Unlock() }

// newUUID returns a freshly generated UUID v4 (36 chars,
// "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" where y is 8/9/a/b).
func newUUID() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	// Set version (4) and variant (10xx) bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// NewUUID is the exported form of newUUID so packages that
// need to mint profile IDs (e.g. the UI when creating a
// profile through the picker) can do so without depending on
// internal helpers.
func NewUUID() (string, error) { return newUUID() }
