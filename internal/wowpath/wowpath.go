// Package wowpath resolves the World of Warcraft installation directory
// and constructs cross-platform paths to the AddOns folder.
package wowpath

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// autoDetect delegates to DetectCandidates and returns the first
// match, preserving backward compatibility for callers that only
// need a single path.
func autoDetect() (Path, error) {
	candidates := DetectCandidates()
	for _, cand := range candidates {
		p, err := Resolve(cand)
		if err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: no auto-detected WoW install", ErrNoWoWPath)
}

// DetectCandidates searches common locations for WoW Interface/AddOns
// folders. Returns all valid candidates (may be empty).
func DetectCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var candidates []string

	winePrefixes := []string{
		filepath.Join(home, ".wine"),
		filepath.Join(home, ".local", "share", "wineprefixes"),
	}
	for _, prefix := range winePrefixes {
		candidates = append(candidates, findAddOnsInWinePrefix(prefix)...)
	}

	steamRoot := filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata")
	candidates = append(candidates, findAddOnsInSteam(steamRoot)...)

	lutrisRoot := filepath.Join(home, "Games")
	candidates = append(candidates, findAddOnsInDir(lutrisRoot)...)

	bottlesRoot := filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles")
	candidates = append(candidates, findAddOnsInDir(bottlesRoot)...)

	directCandidates := []string{
		filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "World of Warcraft"),
		filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft"),
	}
	for _, root := range directCandidates {
		p, err := Resolve(root)
		if err == nil {
			candidates = append(candidates, string(p))
		}
	}

	if runtime.GOOS == "windows" {
		for _, drive := range []string{"C:", "D:", "E:"} {
			candidates = append(candidates, findAddOnsInDir(filepath.Join(drive+"\\", "Games"))...)
			candidates = append(candidates, findAddOnsInDir(filepath.Join(drive+"\\", "Program Files"))...)
			candidates = append(candidates, findAddOnsInDir(filepath.Join(drive+"\\", "Program Files (x86)"))...)
		}
	}

	return dedupeCandidates(candidates)
}

// findAddOnsInWinePrefix looks for WoW inside a Wine prefix structure.
func findAddOnsInWinePrefix(prefix string) []string {
	return findAddOnsInDir(filepath.Join(prefix, "drive_c"))
}

// findAddOnsInSteam looks for WoW inside Steam Proton compatdata prefixes.
func findAddOnsInSteam(steamCompat string) []string {
	entries, err := os.ReadDir(steamCompat)
	if err != nil {
		return nil
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pfxPath := filepath.Join(steamCompat, e.Name(), "pfx", "drive_c")
		candidates = append(candidates, findAddOnsInDir(pfxPath)...)
	}
	return candidates
}

// findAddOnsInDir walks dir (max 4 levels deep) looking for
// Interface/AddOns subdirectories. Returns absolute paths to found
// AddOns folders.
func findAddOnsInDir(root string) []string {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	var found []string
	depth := 0
	var walk func(string)
	walk = func(dir string) {
		if depth > 4 || len(found) >= 5 {
			return
		}
		depth++
		defer func() { depth-- }()

		addonsPath := filepath.Join(dir, "Interface", "AddOns")
		if st, err := os.Stat(addonsPath); err == nil && st.IsDir() {
			found = append(found, addonsPath)
			return
		}
		if strings.EqualFold(filepath.Base(dir), "AddOns") || strings.EqualFold(filepath.Base(dir), "addons") {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				found = append(found, dir)
				return
			}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			walk(filepath.Join(dir, e.Name()))
		}
	}
	walk(root)
	return found
}

func dedupeCandidates(c []string) []string {
	seen := make(map[string]bool)
	out := c[:0]
	for _, s := range c {
		cleaned := filepath.Clean(s)
		if !seen[cleaned] {
			seen[cleaned] = true
			out = append(out, cleaned)
		}
	}
	return out
}

// cleanSegment rejects path-traversal attempts and null bytes from a
// user-supplied segment. Whitespace is allowed because WoW addons
// sometimes have spaces; Unicode is preserved.
func cleanSegment(s string) (string, error) {
	return safepath.Validate(s)
}

// IsWritable returns true if the process can create files in dir.
func IsWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".lazyaddons-writetest-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// RelaunchAsAdmin re-executes the current binary with elevated
// privileges. On Windows it uses runas; on Linux/macOS it tries
// pkexec then sudo. Returns an error if no escalation method is
// available or the user cancels.
func RelaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("wowpath: cannot find executable: %w", err)
	}
	switch runtime.GOOS {
	case "windows":
		// runas triggers the UAC prompt.
		return exec.Command("runas", "/user:Administrator", exe).Start()
	default:
		// Linux/macOS: try pkexec first (Polkit, more GUI-friendly),
		// then sudo as fallback.
		if _, err := exec.LookPath("pkexec"); err == nil {
			return exec.Command("pkexec", exe).Start()
		}
		if _, err := exec.LookPath("sudo"); err == nil {
			return exec.Command("sudo", exe).Start()
		}
		return errors.New("wowpath: no admin escalation tool found (pkexec or sudo)")
	}
}
