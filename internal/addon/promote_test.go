package addon

import (
	"os"
	"path/filepath"
	"testing"
)

// writeNestedAddon creates <root>/<repo>/<name>/<name>.toc plus a
// marker file, so a test can tell the repo copy from a stale copy
// that was already installed.
func writeNestedAddon(t *testing.T, root, repo, name, marker string) {
	t.Helper()
	dir := filepath.Join(root, repo, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	toc := "## Title: " + name + "\n## Version: 2.0\n"
	if err := os.WriteFile(filepath.Join(dir, name+".toc"), []byte(toc), 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".lua"), []byte("-- "+marker+"\n"), 0o644); err != nil {
		t.Fatalf("write lua: %v", err)
	}
}

func TestPromote_ReplacesExistingAddonDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeNestedAddon(t, root, "survivor-ui", "SurvivorUI", "fresh")

	// A manual install is already sitting in the AddOns root.
	dst := filepath.Join(root, "SurvivorUI")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "SurvivorUI.toc"), []byte("## Title: old\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "SurvivorUI.lua"), []byte("-- stale\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	promoted, err := PromoteAddonDirs(root, filepath.Join(root, "survivor-ui"))
	if err != nil {
		t.Fatalf("PromoteAddonDirs: %v", err)
	}
	if len(promoted) != 1 || promoted[0] != "SurvivorUI" {
		t.Fatalf("promoted = %v, want [SurvivorUI]", promoted)
	}

	got, err := os.ReadFile(filepath.Join(dst, "SurvivorUI.lua"))
	if err != nil {
		t.Fatalf("read promoted file: %v", err)
	}
	if string(got) != "-- fresh\n" {
		t.Errorf("installed file = %q, want the repo copy", got)
	}

	if _, err := os.Stat(filepath.Join(root, ".backup", "SurvivorUI", "SurvivorUI.lua")); err != nil {
		t.Errorf("replaced folder was not backed up: %v", err)
	}
}

// A repo left parked in .lazyaddons/ by an earlier failed add must
// still promote: the caller passes the parked path as cloneDir, so
// there is no rename to do and only the unpack matters.
func TestPromote_ParkedRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	parked := filepath.Join(root, ".lazyaddons", "survivor-ui")
	writeNestedAddon(t, root, filepath.Join(".lazyaddons", "survivor-ui"), "SurvivorUI", "fresh")

	promoted, err := PromoteAddonDirs(root, parked)
	if err != nil {
		t.Fatalf("PromoteAddonDirs(parked): %v", err)
	}
	if len(promoted) != 1 || promoted[0] != "SurvivorUI" {
		t.Fatalf("promoted = %v, want [SurvivorUI]", promoted)
	}
	if _, err := os.Stat(filepath.Join(root, "SurvivorUI", "SurvivorUI.toc")); err != nil {
		t.Errorf("addon not unpacked to the AddOns root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parked, "SurvivorUI")); err == nil {
		t.Errorf("repo copy still inside .lazyaddons, want it moved out")
	}
}

// A repository that contains no addon folder is not an error here:
// the caller decides whether an empty result is acceptable.
func TestPromote_RepoWithoutTOCReturnsNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	clone := filepath.Join(root, "not-an-addon")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	promoted, err := PromoteAddonDirs(root, clone)
	if err != nil {
		t.Fatalf("PromoteAddonDirs: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("promoted = %v, want empty", promoted)
	}
}

func TestExistingTargets_ListsOnlyInstalledFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, ".lazyaddons", "survivor-ui")
	writeNestedAddon(t, root, filepath.Join(".lazyaddons", "survivor-ui"), "SurvivorUI", "fresh")
	writeNestedAddon(t, root, filepath.Join(".lazyaddons", "survivor-ui"), "SurvivorPlates", "fresh")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ExistingTargets(root, repo); len(got) != 0 {
		t.Errorf("with nothing installed: %v, want empty", got)
	}

	if err := os.MkdirAll(filepath.Join(root, "SurvivorPlates"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := ExistingTargets(root, repo)
	if len(got) != 1 || got[0] != "SurvivorPlates" {
		t.Errorf("ExistingTargets = %v, want [SurvivorPlates]", got)
	}
}

// A flat addon whose .toc name matches the repository unpacks onto
// the clone itself, which is not a conflict with an existing install.
func TestExistingTargets_IgnoresTheCloneItself(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	clone := filepath.Join(root, "CleanerChat")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clone, "CleanerChat.toc"), []byte("## Title: CleanerChat\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ExistingTargets(root, clone); len(got) != 0 {
		t.Errorf("ExistingTargets = %v, want empty (the clone is not an existing install)", got)
	}

	// A flat addon whose .toc name differs from the repository does
	// conflict with a folder installed under that .toc name.
	if err := os.MkdirAll(filepath.Join(root, "CleanerChat2"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	other := filepath.Join(root, "OtherRepo")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(other, "CleanerChat2.toc"), []byte("## Title: CleanerChat2\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := ExistingTargets(root, other)
	if len(got) != 1 || got[0] != "CleanerChat2" {
		t.Errorf("ExistingTargets = %v, want [CleanerChat2]", got)
	}
}
