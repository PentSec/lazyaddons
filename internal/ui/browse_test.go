package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListDirs_WowFoldersSurfaceFirst verifies that listDirs brings
// WoW-suggestive folder names to the top of the returned slice while
// keeping the alphabetical order within each subgroup stable.
func TestListDirs_WowFoldersSurfaceFirst(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Build a mixed set: some WoW-suggestive names, some unrelated.
	names := []string{
		"Zoom",
		"World of Warcraft",
		"addons",
		"System32",
		"Blizzard Art",
		"Downloads",
	}
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}

	got, err := listDirs(root)
	if err != nil {
		t.Fatalf("listDirs: %v", err)
	}
	if len(got) != len(names) {
		t.Fatalf("got %d dirs, want %d", len(got), len(names))
	}

	// The leading block must be the WoW-suggestive entries in
	// case-insensitive alphabetical order.
	wantLead := []string{"addons", "Blizzard Art", "World of Warcraft"}
	for i, want := range wantLead {
		if got[i].Name != want {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, want)
		}
	}

	// The rest must follow in case-insensitive alphabetical order.
	wantRest := []string{"Downloads", "System32", "Zoom"}
	for i, want := range wantRest {
		if got[len(wantLead)+i].Name != want {
			t.Errorf("got[%d].Name = %q, want %q", len(wantLead)+i, got[len(wantLead)+i].Name, want)
		}
	}
}

// TestListDirs_OrderIsCaseInsensitiveAlphabeticalWithinGroups
// guards against regressions that would re-introduce byte-order
// sorting (uppercase first) inside each partition.
func TestListDirs_OrderIsCaseInsensitiveAlphabeticalWithinGroups(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Two WoW-suggestive names that differ only in case prefix.
	for _, n := range []string{"World of Warcraft", "world of warcraft dlc"} {
		if err := os.Mkdir(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}

	got, err := listDirs(root)
	if err != nil {
		t.Fatalf("listDirs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d dirs, want 2", len(got))
	}
	// Case-insensitive sort puts the shorter (no "dlc" suffix) first.
	if got[0].Name != "World of Warcraft" {
		t.Errorf("got[0] = %q, want %q", got[0].Name, "World of Warcraft")
	}
}

// TestListDirs_OnlyDirectoriesReturned ensures listDirs does not
// surface files even when they live next to the directories.
func TestListDirs_OnlyDirectoriesReturned(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "wow-folder"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	got, err := listDirs(root)
	if err != nil {
		t.Fatalf("listDirs: %v", err)
	}
	if len(got) != 1 || got[0].Name != "wow-folder" {
		t.Fatalf("got = %v, want [wow-folder]", got)
	}
}

// TestListDirs_HiddenDirectoriesSkippedExceptParent ensures the
// existing behaviour of skipping dot-prefixed directories is intact
// after the reordering change.
func TestListDirs_HiddenDirectoriesSkippedExceptParent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, n := range []string{".hidden", "visible"} {
		if err := os.Mkdir(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}

	got, err := listDirs(root)
	if err != nil {
		t.Fatalf("listDirs: %v", err)
	}
	if len(got) != 1 || got[0].Name != "visible" {
		t.Fatalf("got = %v, want [visible]", got)
	}
	for _, e := range got {
		if strings.HasPrefix(e.Name, ".") {
			t.Errorf("hidden dir surfaced: %q", e.Name)
		}
	}
}
