// Package wowpath resolves the World of Warcraft installation directory
// and constructs cross-platform paths to the AddOns folder.
package wowpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pentsec/lazyaddons/internal/safepath"
)

// ErrNoWoWPath is returned when no WoW installation can be discovered or
// provided by the caller. The error wraps a human-readable message so the
// UI can surface it without further formatting.
var ErrNoWoWPath = errors.New("wowpath: WoW installation path is not set")

// ErrNoAddOnsFolder is returned when the resolved path does not contain
// the conventional Interface/AddOns subdirectory, signalling either a
// malformed user-provided path or a corrupt install.
var ErrNoAddOnsFolder = errors.New("wowpath: Interface/AddOns folder not found")

// Path is the resolved absolute path to a WoW AddOns folder. It is
// always cleaned and uses the platform-specific separator.
type Path string

// String returns the string form of the AddOns path.
func (p Path) String() string { return string(p) }

// Dir returns the parent (WoW root) directory.
func (p Path) Dir() string { return filepath.Dir(string(p)) }

// AddonPath returns the absolute path to a specific addon folder
// inside the AddOns directory.
func (p Path) AddonPath(name string) (string, error) {
	clean, err := cleanSegment(name)
	if err != nil {
		return "", fmt.Errorf("wowpath: invalid addon name %q: %w", name, err)
	}
	return filepath.Join(string(p), clean), nil
}

// RepoDirName is the subdirectory inside AddOns where lazyaddons
// stores bare git repos. The repos live at <AddOns>/.lazyaddons/<name>.
const RepoDirName = ".lazyaddons"

// RepoPath returns the path to the git repo directory for a named
// addon inside the .lazyaddons/ subfolder of AddOns.
func (p Path) RepoPath(name string) (string, error) {
	clean, err := cleanSegment(name)
	if err != nil {
		return "", fmt.Errorf("wowpath: invalid addon name %q: %w", name, err)
	}
	return filepath.Join(string(p), RepoDirName, clean), nil
}

// OldRepoPath returns the legacy repo path (<name>.repo at AddOns
// root) for backward-compatible discovery.
func (p Path) OldRepoPath(name string) string {
	return filepath.Join(string(p), name+".repo")
}

// BackupPath returns the path to the .backup sibling directory.
func (p Path) BackupPath(name string) (string, error) {
	clean, err := cleanSegment(name)
	if err != nil {
		return "", fmt.Errorf("wowpath: invalid addon name %q: %w", name, err)
	}
	return filepath.Join(string(p), ".backup", clean), nil
}

// Validate ensures the AddOns folder actually exists. It returns
// ErrNoAddOnsFolder when the path is well-formed but the directory
// is missing.
func (p Path) Validate() error {
	info, err := os.Stat(string(p))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNoAddOnsFolder, string(p))
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("wowpath: %s is not a directory", string(p))
	}
	return nil
}

// Resolve turns a user-supplied WoW root into an AddOns Path. The
// supplied root may be the WoW directory itself or the AddOns
// directory directly; both are normalised to the AddOns path. When
// `wowRoot` is empty, Resolve auto-detects a Wine prefix under
// the user's HOME.
func Resolve(wowRoot string) (Path, error) {
	if wowRoot == "" {
		return autoDetect()
	}

	// Allow callers to pass the AddOns directory directly.
	cleaned := filepath.Clean(wowRoot)
	normalized := strings.ToLower(filepath.ToSlash(cleaned))
	candidate := cleaned

	switch {
	case strings.HasSuffix(normalized, "interface/addons"):
		// Already the AddOns path — use as-is.
	case strings.HasSuffix(normalized, "/addons"):
		// Just "AddOns" — use as-is.
	case strings.HasSuffix(normalized, "addons"):
		// "AddOns" without trailing slash — use as-is.
	case strings.HasSuffix(normalized, "/interface"):
		// User provided the Interface folder.
		candidate = filepath.Join(cleaned, "AddOns")
	case strings.HasSuffix(normalized, "interface"):
		// Interface without trailing slash.
		candidate = filepath.Join(cleaned, "AddOns")
	default:
		// Assume it's the WoW root directory.
		candidate = filepath.Join(cleaned, "Interface", "AddOns")
	}

	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("wowpath: cannot absolutise %q: %w", candidate, err)
	}

	p := Path(abs)
	if err := p.Validate(); err != nil {
		return "", err
	}
	return p, nil
}

// autoDetect scans the user's home directory for a Wine/Proton
// WoW prefix. The probe is deliberately conservative — it only
// looks for an Interface/AddOns folder under known prefixes.
func autoDetect() (Path, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("wowpath: cannot resolve home dir: %w", err)
	}

	candidates := []string{
		filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "World of Warcraft"),
		filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft"),
		filepath.Join(home, ".steam", "steam", "steamapps", "common", "World of Warcraft"),
	}

	for _, root := range candidates {
		p, err := Resolve(root)
		if err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%w: no auto-detected WoW install", ErrNoWoWPath)
}

// cleanSegment rejects path-traversal attempts and null bytes from a
// user-supplied segment. Whitespace is allowed because WoW addons
// sometimes have spaces; Unicode is preserved.
func cleanSegment(s string) (string, error) {
	return safepath.Validate(s)
}
