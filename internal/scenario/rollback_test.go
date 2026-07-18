package scenario

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pentsec/lazyaddons/internal/backup"
	"github.com/pentsec/lazyaddons/internal/config"
)

// ---- addon-rollback scenarios ----

func TestRollback_BackupIntegrity(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	dir := filepath.Join(addons, "MyAddon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.toc"), []byte("## Title: x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := backup.New(addons)
	if err := mgr.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Backup must contain a .toc file.
	backupTOC := filepath.Join(addons, backup.BackupDirName, "MyAddon", "MyAddon.toc")
	if _, err := os.Stat(backupTOC); err != nil {
		t.Errorf("backup missing .toc: %v", err)
	}
}

func TestRollback_Execution(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	dir := filepath.Join(addons, "MyAddon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.toc"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.lua"), []byte("-- v1"), 0o644); err != nil {
		t.Fatalf("write lua: %v", err)
	}

	mgr := backup.New(addons)
	if err := mgr.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Overwrite live with v2.
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.lua"), []byte("-- v2"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	// Rollback.
	if err := mgr.Restore("MyAddon"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "MyAddon.lua"))
	if string(got) != "-- v1" {
		t.Errorf("lua after rollback = %q, want v1", got)
	}
}

func TestRollback_EmptyBackupError(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	// Manually create an empty backup.
	empty := filepath.Join(addons, backup.BackupDirName, "MyAddon")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(empty, "junk.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := backup.New(addons)
	err := mgr.Restore("MyAddon")
	if err == nil {
		t.Errorf("Restore(empty backup) = nil, want error")
	}
}

func TestRollback_ConfigStateUpdate(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Version: 1, Addons: []config.Addon{
		{Name: "X", URL: "u", TrackMode: "release", TrackTarget: "v1.0.0"},
	}}
	// Simulate update: change track target to v1.1.0.
	entry := cfg.AddonByName("X")
	entry.TrackTarget = "v1.1.0"
	// Simulate rollback: revert to v1.0.0.
	entry.TrackTarget = "v1.0.0"
	if cfg.Addons[0].TrackTarget != "v1.0.0" {
		t.Errorf("TrackTarget = %q, want v1.0.0 (rolled back)", cfg.Addons[0].TrackTarget)
	}
}

func TestRollback_PreservesBackupAfterRestore(t *testing.T) {
	t.Parallel()
	addons := t.TempDir()
	dir := filepath.Join(addons, "MyAddon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyAddon.toc"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mgr := backup.New(addons)
	if err := mgr.Create("MyAddon"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mgr.Restore("MyAddon"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	has, _ := mgr.HasBackup("MyAddon")
	if !has {
		t.Errorf("backup lost after restore, want preserved")
	}
}
