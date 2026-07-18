package wowpath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolve_WithValidWoWRoot(t *testing.T) {
	t.Parallel()
	// Use platform-specific separator for fixture creation
	sep := string(filepath.Separator)
	_ = sep

	root := t.TempDir()
	addons := filepath.Join(root, "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := Resolve(root)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	want := filepath.Clean(addons)
	if got.String() != want {
		t.Errorf("Resolve(%q) = %q, want %q", root, got, want)
	}
}

func TestResolve_AcceptsAddOnsDirectly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	addons := filepath.Join(root, "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Caller can pass the AddOns folder directly — Resolve should
	// not append another "Interface/AddOns" to the path.
	got, err := Resolve(addons)
	if err != nil {
		t.Fatalf("Resolve(addons) returned error: %v", err)
	}

	if got.String() != filepath.Clean(addons) {
		t.Errorf("Resolve(addons) = %q, want %q", got, addons)
	}
}

func TestResolve_RejectsNonExistentPath(t *testing.T) {
	t.Parallel()
	bogus := filepath.Join(t.TempDir(), "does", "not", "exist")
	_, err := Resolve(bogus)
	if err == nil {
		t.Fatalf("Resolve(bogus) = nil error, want ErrNoAddOnsFolder")
	}
	if !errors.Is(err, ErrNoAddOnsFolder) {
		t.Errorf("Resolve(bogus) error = %v, want ErrNoAddOnsFolder", err)
	}
}

func TestResolve_RejectsPathWithoutInterfaceAddOns(t *testing.T) {
	t.Parallel()
	// Directory exists but is not a WoW install — no AddOns subfolder.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "stuff"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := Resolve(root)
	if err == nil {
		t.Fatalf("Resolve(non-wow-dir) = nil error, want ErrNoAddOnsFolder")
	}
}

func TestResolve_NormalisesToAbsolute(t *testing.T) {
	// Uses t.Chdir — cannot be parallel.
	root := t.TempDir()
	addons := filepath.Join(root, "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Pass a relative path that resolves into the temp dir.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := Resolve(".")
	if err != nil {
		t.Fatalf("Resolve(relative) error: %v", err)
	}
	if !filepath.IsAbs(got.String()) {
		t.Errorf("Resolve(relative) = %q, want absolute path", got)
	}
}

func TestResolve_AutoDetectWithNoWoWReturnsError(t *testing.T) {
	// Set HOME to a directory that contains no wine/proton prefixes.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", empty)
	}

	_, err := Resolve("")
	if err == nil {
		t.Fatalf("Resolve(\"\") with empty HOME = nil error, want ErrNoWoWPath")
	}
	if !errors.Is(err, ErrNoWoWPath) {
		t.Errorf("Resolve(\"\") error = %v, want ErrNoWoWPath", err)
	}
}

func TestResolve_AutoDetectsWinePrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("wine detection is Linux/macOS only")
	}

	home := t.TempDir()
	wowRoot := filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "World of Warcraft")
	addons := filepath.Join(wowRoot, "Interface", "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Setenv("HOME", home)

	got, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\") with wine prefix = error: %v", err)
	}

	want := filepath.Clean(addons)
	if got.String() != want {
		t.Errorf("Resolve(\"\") = %q, want %q", got, want)
	}
}

func TestPath_AddonPath_ValidName(t *testing.T) {
	t.Parallel()
	p := Path("/tmp/wow/Interface/AddOns")
	got, err := p.AddonPath("MyAddon")
	if err != nil {
		t.Fatalf("AddonPath error: %v", err)
	}
	want := filepath.Join("/tmp/wow/Interface/AddOns", "MyAddon")
	if got != want {
		t.Errorf("AddonPath = %q, want %q", got, want)
	}
}

func TestPath_AddonPath_RejectsTraversal(t *testing.T) {
	t.Parallel()
	p := Path("/tmp/wow/Interface/AddOns")
	_, err := p.AddonPath("../etc/passwd")
	if err == nil {
		t.Fatalf("AddonPath(traversal) = nil error, want error")
	}
}

func TestPath_AddonPath_RejectsNullByte(t *testing.T) {
	t.Parallel()
	p := Path("/tmp/wow/Interface/AddOns")
	_, err := p.AddonPath("bad\x00name")
	if err == nil {
		t.Fatalf("AddonPath(null) = nil error, want error")
	}
}

func TestPath_BackupPath_ValidName(t *testing.T) {
	t.Parallel()
	p := Path("/tmp/wow/Interface/AddOns")
	got, err := p.BackupPath("MyAddon")
	if err != nil {
		t.Fatalf("BackupPath error: %v", err)
	}
	want := filepath.Join("/tmp/wow/Interface/AddOns", ".backup", "MyAddon")
	if got != want {
		t.Errorf("BackupPath = %q, want %q", got, want)
	}
}

func TestPath_Validate_OnExistingDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := Path(dir)
	if err := p.Validate(); err != nil {
		t.Errorf("Validate(existing) = %v, want nil", err)
	}
}

func TestPath_Validate_OnMissingDir(t *testing.T) {
	t.Parallel()
	p := Path(filepath.Join(t.TempDir(), "missing"))
	err := p.Validate()
	if err == nil {
		t.Fatalf("Validate(missing) = nil, want error")
	}
	if !errors.Is(err, ErrNoAddOnsFolder) {
		t.Errorf("Validate(missing) error = %v, want ErrNoAddOnsFolder", err)
	}
}

func TestCleanSegment_AllowsSpaces(t *testing.T) {
	t.Parallel()
	got, err := cleanSegment("My Add On")
	if err != nil {
		t.Fatalf("cleanSegment(spaces) error: %v", err)
	}
	if got != "My Add On" {
		t.Errorf("cleanSegment(spaces) = %q, want %q", got, "My Add On")
	}
}

func TestCleanSegment_AllowsUnicode(t *testing.T) {
	t.Parallel()
	got, err := cleanSegment("名称-Addon")
	if err != nil {
		t.Fatalf("cleanSegment(unicode) error: %v", err)
	}
	if got != "名称-Addon" {
		t.Errorf("cleanSegment(unicode) = %q, want %q", got, "名称-Addon")
	}
}
