package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/config"
)

// runCloneMsg executes a command — possibly a tea.Batch — and
// returns the clonePostDoneMsg it produces.
func runCloneMsg(t *testing.T, cmd tea.Cmd) clonePostDoneMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("command is nil")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if done, ok := c().(clonePostDoneMsg); ok {
				return done
			}
		}
		t.Fatal("batch produced no clonePostDoneMsg")
	}
	done, ok := msg.(clonePostDoneMsg)
	if !ok {
		t.Fatalf("message = %T, want clonePostDoneMsg", msg)
	}
	return done
}

// writeAddonTree lays out <dir>/<name>/{<name>.toc,<name>.lua}
// with a marker file so tests can tell the repo copy from a stale
// one.
func writeAddonTree(t *testing.T, dir, name string) {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, name+".toc"), []byte("## Title: "+name+"\n## Version: 2.0\n"), 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, name+".lua"), []byte("-- fresh\n"), 0o644); err != nil {
		t.Fatalf("write lua: %v", err)
	}
}

// parkRepo lays out <root>/.lazyaddons/<repo> as a parked clone:
// a .git directory plus the addon tree.
func parkRepo(t *testing.T, root, repo, name string) {
	t.Helper()
	base := filepath.Join(root, ".lazyaddons", repo)
	writeAddonTree(t, base, name)
	if err := os.MkdirAll(filepath.Join(base, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
}

// installStale lays out a manual install of <name> in the AddOns
// root and returns its folder.
func installStale(t *testing.T, root, name string) string {
	t.Helper()
	dst := filepath.Join(root, name)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, name+".toc"), []byte("## Title: old\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, name+".lua"), []byte("-- stale\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return dst
}

// newAddTestModel wires a Model to a single profile rooted at root.
func newAddTestModel(root string) *Model {
	m := NewModel()
	m.Config = &config.Config{
		Version: config.CurrentSchemaVersion,
		Profiles: []config.Profile{{
			ID: "p1", Name: "benilla", WoWPath: root,
		}},
		ActiveProfileID: "p1",
	}
	m.SetActiveProfile(m.Config.FindProfileByID("p1"))
	m.Screen = screenList
	return m
}

// A previous add cloned the repo and then failed before unpacking.
// Re-adding must unpack the addon folders and track the addon
// instead of silently tracking a repo with no folder in AddOns.
func TestAdd_ResumeExistingRepoUnpacksAndTracks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	parkRepo(t, root, "survivor-ui", "SurvivorUI")

	m := newAddTestModel(root)
	installStale(t, root, "SurvivorUI")

	repoDir := filepath.Join(root, ".lazyaddons", "survivor-ui")
	done := runCloneMsg(t, m.resumeExistingRepo(
		"survivor-ui", "https://github.com/morderous/survivor-ui",
		"branch", "", repoDir,
	))
	if done.Err != nil {
		t.Fatalf("resume returned error: %v", done.Err)
	}

	updated, _ := m.Update(done)
	mm := updated.(Model)

	if len(done.Promoted) != 1 || done.Promoted[0] != "SurvivorUI" {
		t.Errorf("Promoted = %v, want [SurvivorUI]", done.Promoted)
	}
	if a := mm.ActiveProfile.AddonByName("SurvivorUI"); a == nil {
		t.Fatalf("SurvivorUI not tracked: %+v", mm.ActiveProfile.Addons)
	} else if a.URL != "https://github.com/morderous/survivor-ui" {
		t.Errorf("tracked URL = %q, want the repo URL", a.URL)
	}
	got, err := os.ReadFile(filepath.Join(root, "SurvivorUI", "SurvivorUI.lua"))
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if string(got) != "-- fresh\n" {
		t.Errorf("installed file = %q, want the repo copy", got)
	}
	if mm.Screen != screenList {
		t.Errorf("Screen = %v, want screenList", mm.Screen)
	}
}

// A repo with no addon folder at all is still rejected on the
// fresh-clone path: nothing was installed, so the add must fail
// rather than track an empty repository.
func TestPromoteCmd_RejectsRepoWithoutTOC(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	clone := filepath.Join(root, "not-an-addon")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	msg := promoteCmd(root, clone, clonePostDoneMsg{Name: "not-an-addon"}, false)()
	done, ok := msg.(clonePostDoneMsg)
	if !ok {
		t.Fatalf("message = %T, want clonePostDoneMsg", msg)
	}
	if done.Err == nil {
		t.Fatalf("promoteCmd on an empty repo = nil error, want a .toc error")
	}
}

// Nested repositories install folders named after their .toc files,
// not after the repository, so the keep/replace prompt has to list
// those folders: the repository folder itself usually does not
// exist and the old name-only check would never fire.
func TestAdd_ReplacePromptListsTOCFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	parkRepo(t, root, "survivor-ui", "SurvivorUI")
	parkRepo(t, root, "survivor-ui", "SurvivorPlates")
	installStale(t, root, "SurvivorUI")
	installStale(t, root, "SurvivorPlates")

	m := newAddTestModel(root)
	if cmd := m.startClone("survivor-ui", "https://github.com/morderous/survivor-ui", "branch", ""); cmd != nil {
		t.Fatalf("startClone = %v, want the confirm prompt with no command", cmd)
	}
	if m.Screen != screenConfirmReplace {
		t.Fatalf("Screen = %v, want screenConfirmReplace", m.Screen)
	}
	if len(m.ReplaceTargets) != 2 || m.ReplaceTargets[0] != "SurvivorPlates" || m.ReplaceTargets[1] != "SurvivorUI" {
		t.Fatalf("ReplaceTargets = %v, want both TOC folders", m.ReplaceTargets)
	}

	// Keep: nothing is installed, nothing is tracked.
	updated, cmd := updateConfirmReplace(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("keep returned a command, want none")
	}
	mm := updated.(Model)
	if mm.Screen != screenList {
		t.Fatalf("Screen = %v, want screenList", mm.Screen)
	}
	if len(mm.ActiveProfile.Addons) != 0 {
		t.Fatalf("keep tracked addons: %+v", mm.ActiveProfile.Addons)
	}
	got, err := os.ReadFile(filepath.Join(root, "SurvivorPlates", "SurvivorPlates.lua"))
	if err != nil {
		t.Fatalf("read existing folder: %v", err)
	}
	if string(got) != "-- stale\n" {
		t.Errorf("keep overwrote the existing folder: %q", got)
	}
}

// Choosing Replace on the same prompt unpacks the parked repo over
// the manual installs, backing them up first.
func TestAdd_ReplacePromptInstallsRepoCopy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	parkRepo(t, root, "survivor-ui", "SurvivorUI")
	parkRepo(t, root, "survivor-ui", "SurvivorPlates")
	installStale(t, root, "SurvivorUI")
	installStale(t, root, "SurvivorPlates")

	m := newAddTestModel(root)
	m.startClone("survivor-ui", "https://github.com/morderous/survivor-ui", "branch", "")
	m.ReplaceSel = 1
	updated, cmd := updateConfirmReplace(m, tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)

	done := runCloneMsg(t, cmd)
	if done.Err != nil {
		t.Fatalf("replace returned error: %v", done.Err)
	}
	finalModel, _ := mm.Update(done)
	final := finalModel.(Model)

	if a := final.ActiveProfile.AddonByName("SurvivorPlates"); a == nil {
		t.Fatalf("addon not tracked: %+v", final.ActiveProfile.Addons)
	}
	for _, name := range []string{"SurvivorUI", "SurvivorPlates"} {
		got, err := os.ReadFile(filepath.Join(root, name, name+".lua"))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != "-- fresh\n" {
			t.Errorf("%s = %q, want the repo copy", name, got)
		}
		if _, err := os.Stat(filepath.Join(root, ".backup", name, name+".lua")); err != nil {
			t.Errorf("%s was replaced without a backup: %v", name, err)
		}
	}
	if final.Screen != screenList {
		t.Errorf("Screen = %v, want screenList", final.Screen)
	}
}

// A clone that lands next to addon folders it would overwrite must
// stop at the prompt too — nothing is unpacked until the user says
// so.
func TestAdd_HandleCloneDonePromptsWhenTargetsExist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	clone := filepath.Join(root, "survivor-ui")
	writeAddonTree(t, clone, "SurvivorUI")
	installStale(t, root, "SurvivorUI")

	m := newAddTestModel(root)
	cmd := m.handleCloneDone(cloneDoneMsg{
		Name: "survivor-ui",
		URL:  "https://github.com/morderous/survivor-ui",
		Mode: "branch",
	})
	if cmd != nil {
		t.Fatalf("handleCloneDone = %v, want the confirm prompt with no command", cmd)
	}
	if m.Screen != screenConfirmReplace {
		t.Fatalf("Screen = %v, want screenConfirmReplace", m.Screen)
	}
	if len(m.ReplaceTargets) != 1 || m.ReplaceTargets[0] != "SurvivorUI" {
		t.Fatalf("ReplaceTargets = %v, want [SurvivorUI]", m.ReplaceTargets)
	}
	if m.ReplaceRepoDir != clone {
		t.Fatalf("ReplaceRepoDir = %q, want %q", m.ReplaceRepoDir, clone)
	}
}
