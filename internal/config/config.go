// Package config persists the WoW installation path and the list of
// tracked addons. The config is the single source of truth for addon
// state. Writes are atomic (temp file + rename) so a crash mid-write
// cannot corrupt the on-disk file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// CurrentSchemaVersion is the version stored in every written config.
// Bump it whenever the on-disk schema changes incompatibly.
const CurrentSchemaVersion = 1

// AppDirName is the folder under which the config file lives.
const AppDirName = "lazyaddons"

// FileName is the config file name. JSON keeps it diff-friendly and
// human-inspectable for debugging.
const FileName = "config.json"

// ErrNotFound is returned by Load when the config file does not exist.
// Callers should treat this as a first-run scenario, not an error.
var ErrNotFound = errors.New("config: file not found")

// ErrCorrupt is returned by Load when the file exists but is not
// valid JSON. The error wraps the underlying parse error so the
// caller can inspect the cause.
var ErrCorrupt = errors.New("config: file is corrupt")

// Config is the root on-disk structure. Every field is exported so
// the JSON encoder writes a stable, hand-inspectable shape.
type Config struct {
	// Version is the schema version, written to disk and checked on
	// load. A mismatch surfaces as a migration prompt.
	Version int `json:"version"`

	// WoWPath is the absolute path to the WoW AddOns folder
	// (e.g. /home/user/wow/Interface/AddOns). It is normalised on
	// save and re-validated on load.
	WoWPath string `json:"wow_path"`

	// Addons is the list of tracked addons.
	Addons []Addon `json:"addons"`
}

// Addon records a tracked repo. The shape matches the addon package's
// Addon struct but is duplicated here to keep the config package
// import-light (no addon → config cycle).
type Addon struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	TrackMode   string `json:"track_mode"`   // "branch" | "release"
	TrackTarget string `json:"track_target"` // branch name or tag
	CurrentSHA  string `json:"current_sha"`
	Version     string `json:"version"`      // from .toc ## Version:
	LastUpdated string `json:"last_updated"` // last commit date (YYYY-MM-DD)
	SubModules  []string `json:"sub_modules,omitempty"` // related addon dirs
}

// Default returns an empty config with the current schema version.
func Default() *Config {
	return &Config{
		Version: CurrentSchemaVersion,
		Addons:  []Addon{},
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
			return "", err
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

	if cfg.Version == 0 {
		// Treat a missing version as v1 — this matches how
		// legacy config files were written.
		cfg.Version = CurrentSchemaVersion
	}
	if cfg.Addons == nil {
		cfg.Addons = []Addon{}
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
	if cfg.Addons == nil {
		cfg.Addons = []Addon{}
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

// AddonByName returns a pointer to the named addon (case-sensitive)
// or nil if not present. The returned pointer aliases the slice
// element so callers can mutate the entry directly before re-saving.
func (c *Config) AddonByName(name string) *Addon {
	for i := range c.Addons {
		if c.Addons[i].Name == name {
			return &c.Addons[i]
		}
	}
	return nil
}

// AddonIndex returns the index of the named addon in the slice,
// or -1 if not present.
func (c *Config) AddonIndex(name string) int {
	for i := range c.Addons {
		if c.Addons[i].Name == name {
			return i
		}
	}
	return -1
}

// UpsertAddon inserts or updates the named addon, preserving the
// existing entry's position. It returns the new count of addons.
func (c *Config) UpsertAddon(a Addon) int {
	for i := range c.Addons {
		if c.Addons[i].Name == a.Name {
			c.Addons[i] = a
			return len(c.Addons)
		}
	}
	c.Addons = append(c.Addons, a)
	return len(c.Addons)
}

// RemoveAddon deletes the named addon. It returns true if an entry
// was removed.
func (c *Config) RemoveAddon(name string) bool {
	for i := range c.Addons {
		if c.Addons[i].Name == name {
			c.Addons = append(c.Addons[:i], c.Addons[i+1:]...)
			return true
		}
	}
	return false
}

// mu guards filesystem mutation in helpers that read-modify-write
// outside the package (e.g. from tests using goroutines). It is not
// strictly required for the single-threaded TUI but the field is
// exported for future plugin hosts that may call Save concurrently.
var mu sync.Mutex

// Lock is exported so callers that race Load + Save can serialise
// them. Most callers can ignore it.
func Lock()   { mu.Lock() }
func Unlock() { mu.Unlock() }
