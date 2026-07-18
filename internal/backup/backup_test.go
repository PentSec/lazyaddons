package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makeAddonDir creates a fake addon folder inside root with the
// given name and a minimal .toc + .lua so validation passes.
func makeAddonDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".toc"), []byte("## Title: x\n"), 0o644); err != nil {
		t.Fatalf("toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".lua"), []byte("-- body\n"), 0o644); err != nil {
		t.Fatalf("lua: %v", err)
	}
	return dir
}

func TestManager_CreateAndRestore(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)
	makeAddonDir(t, addons, "MyAddon")

	// Modify the live folder so we can detect that restore works.
	live := filepath.Join(addons, "MyAddon", "MyAddon.lua")
	if err := os.WriteFile(live, []byte("-- v2\n"), 0o644); err != nil {
		t.Fatalf("write live: %v", err)
	}

	// Create a backup — should snapshot the current (v2) state.
	if err := m.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mutate the live folder to v3 so the backup differs.
	if err := os.WriteFile(live, []byte("-- v3\n"), 0o644); err != nil {
		t.Fatalf("write v3: %v", err)
	}

	// Restore should bring back v2.
	if err := m.Restore("MyAddon"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(live)
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	if string(got) != "-- v2\n" {
		t.Errorf("live after restore = %q, want %q", got, "-- v2\n")
	}
}

func TestCreate_NoExistingFolderIsNoop(t *testing.T) {
	t.Parallel()
	m := New(t.TempDir())
	if err := m.Create("Ghost"); err != nil {
		t.Errorf("Create(ghost) = %v, want nil", err)
	}
	has, err := m.HasBackup("Ghost")
	if err != nil {
		t.Fatalf("HasBackup: %v", err)
	}
	if has {
		t.Errorf("HasBackup(ghost) = true, want false")
	}
}

func TestCreate_OverwritesPriorBackup(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)
	makeAddonDir(t, addons, "MyAddon")

	// First backup: live = v1.
	if err := m.Create("MyAddon"); err != nil {
		t.Fatalf("Create#1: %v", err)
	}

	// Mutate live to v2, then re-backup.
	live := filepath.Join(addons, "MyAddon", "MyAddon.lua")
	if err := os.WriteFile(live, []byte("-- v2\n"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	if err := m.Create("MyAddon"); err != nil {
		t.Fatalf("Create#2: %v", err)
	}

	// The backup should now hold v2.
	backupLua := filepath.Join(addons, BackupDirName, "MyAddon", "MyAddon.lua")
	got, err := os.ReadFile(backupLua)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "-- v2\n" {
		t.Errorf("backup after overwrite = %q, want %q", got, "-- v2\n")
	}
}

func TestRestore_RejectsEmptyBackup(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)

	// Manually create a backup folder with no .toc inside.
	empty := filepath.Join(addons, BackupDirName, "MyAddon")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(empty, "junk.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	err := m.Restore("MyAddon")
	if !errors.Is(err, ErrEmptyBackup) {
		t.Errorf("Restore(empty) error = %v, want ErrEmptyBackup", err)
	}
}

func TestRestore_RejectsMissingBackup(t *testing.T) {
	t.Parallel()
	m := New(t.TempDir())
	err := m.Restore("Ghost")
	if !errors.Is(err, ErrEmptyBackup) {
		t.Errorf("Restore(missing) error = %v, want ErrEmptyBackup", err)
	}
}

func TestHasBackup(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)

	if has, err := m.HasBackup("X"); err != nil || has {
		t.Errorf("HasBackup(none) = (%v, %v), want (false, nil)", has, err)
	}

	makeAddonDir(t, addons, "X")
	if err := m.Create("X"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if has, err := m.HasBackup("X"); err != nil || !has {
		t.Errorf("HasBackup(after create) = (%v, %v), want (true, nil)", has, err)
	}
}

func TestCreate_CopiesNestedFolders(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)
	dir := makeAddonDir(t, addons, "Atlas")
	nested := filepath.Join(dir, "Modules", "Core")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "core.lua"), []byte("-- core"), 0o644); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	if err := m.Create("Atlas"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	backupNested := filepath.Join(addons, BackupDirName, "Atlas", "Modules", "Core", "core.lua")
	if _, err := os.Stat(backupNested); err != nil {
		t.Errorf("nested file missing in backup: %v", err)
	}
}

func TestCreate_PreservesBackupAfterRestore(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	m := New(addons)
	makeAddonDir(t, addons, "MyAddon")

	if err := m.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Restore("MyAddon"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	has, err := m.HasBackup("MyAddon")
	if err != nil {
		t.Fatalf("HasBackup: %v", err)
	}
	if !has {
		t.Errorf("backup missing after restore, want it preserved")
	}
}

func TestCreate_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	m := New(t.TempDir())
	_, err := m.BackupDir("../escape")
	if err == nil {
		t.Errorf("BackupDir(traversal) = nil, want error")
	}
}

func TestCreate_RejectsNullByte(t *testing.T) {
	t.Parallel()
	m := New(t.TempDir())
	_, err := m.BackupDir("bad\x00name")
	if err == nil {
		t.Errorf("BackupDir(null) = nil, want error")
	}
}
