// Package backup provides single-level backup and restore of an
// addon folder. It is intentionally simple: exactly one backup per
// addon, stored under `<AddOns>/.backup/<addon>/`.
//
// The design assumes the AddOns folder is the working set and the
// backup folder is a sibling that we never interact with outside
// this package.
package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pentsec/lazyaddons/internal/safepath"
)

// BackupDirName is the name of the sibling folder under AddOns that
// stores backups. Leading dot keeps it hidden on Linux/macOS and
// out of the way on Windows Explorer.
const BackupDirName = ".backup"

// ErrEmptyBackup is returned by Restore when the backup directory
// is empty or missing required files.
var ErrEmptyBackup = errors.New("backup: backup directory is empty or missing .toc")

// Manager coordinates create / restore operations. It is a thin
// wrapper around a filesystem location so it can be unit-tested
// with t.TempDir() and configured at runtime.
type Manager struct {
	addonsPath string
}

// New returns a Manager that operates under the given AddOns root.
func New(addonsPath string) *Manager {
	return &Manager{addonsPath: addonsPath}
}

// BackupDir returns the absolute path to the backup directory for
// a named addon.
func (m *Manager) BackupDir(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	return filepath.Join(m.addonsPath, BackupDirName, name), nil
}

// AddonDir returns the absolute path to a live addon directory.
func (m *Manager) AddonDir(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	return filepath.Join(m.addonsPath, name), nil
}

// Create copies the current addon folder into the backup location.
// The destination is wiped first so we maintain exactly one backup.
// If the source folder does not exist, Create is a no-op and
// returns nil — there is nothing to back up.
//
// Any I/O failure mid-copy leaves the backup directory in an
// indeterminate state; the caller should treat the addon as not
// backed up in that case.
func (m *Manager) Create(name string) error {
	src, err := m.AddonDir(name)
	if err != nil {
		return err
	}
	dst, err := m.BackupDir(name)
	if err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("backup: stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("backup: %s is not a directory", src)
	}

	// Wipe the existing backup, if any.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("backup: wipe %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("backup: mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := copyTree(src, dst); err != nil {
		return fmt.Errorf("backup: copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

// Restore replaces the live addon folder with the contents of the
// backup. The backup itself is preserved. It returns ErrEmptyBackup
// if the backup is missing the .toc file required for an integrity
// check.
func (m *Manager) Restore(name string) error {
	src, err := m.BackupDir(name)
	if err != nil {
		return err
	}
	dst, err := m.AddonDir(name)
	if err != nil {
		return err
	}

	// Integrity check: backup must exist and contain at least
	// one .toc file.
	hasTOC, err := containsTOC(src)
	if err != nil {
		return fmt.Errorf("backup: integrity %s: %w", src, err)
	}
	if !hasTOC {
		return fmt.Errorf("%w: %s", ErrEmptyBackup, src)
	}

	// Wipe the live folder and re-create it from the backup.
	if err := os.RemoveAll(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup: wipe live %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("backup: mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := copyTree(src, dst); err != nil {
		return fmt.Errorf("backup: restore %s -> %s: %w", src, dst, err)
	}
	return nil
}

// HasBackup reports whether a backup exists for the named addon.
func (m *Manager) HasBackup(name string) (bool, error) {
	dir, err := m.BackupDir(name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// copyTree recursively copies src into dst. dst must exist.
// Symlinks are resolved to their targets (we do not preserve them)
// because WoW addons do not use them and following them avoids
// escaping the addon folder.
func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())

		// Reject symlinks to avoid path traversal.
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to copy symlink %s", s)
		}

		if e.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d, e.Type()); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a single file, preserving its mode.
func copyFile(src, dst string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// containsTOC walks dir (non-recursively) and reports whether it
// contains at least one *.toc file. We don't recurse because an
// addon root can contain nested folder trees but only top-level
// .toc files are valid manifests.
func containsTOC(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".toc") {
			return true, nil
		}
	}
	return false, nil
}

// validateName rejects path-traversal attempts in addon names.
// The check is shared by AddonDir and BackupDir so both share the
// same trust boundary.
func validateName(name string) error {
	_, err := safepath.Validate(name)
	return err
}
