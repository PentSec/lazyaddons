package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/backup"
	"github.com/pentsec/lazyaddons/internal/config"
	gh "github.com/pentsec/lazyaddons/internal/github"
	"github.com/pentsec/lazyaddons/internal/gitops"
)

// cloneDoneMsg is sent when a background git clone finishes.
type cloneDoneMsg struct {
	Name          string
	URL           string
	Mode          string
	Target        string
	DefaultBranch string // detected default branch (e.g. "main", "master")
	Err           error
}

// releaseFetchedMsg is sent when the GitHub API returns the
// latest release tag. The handler starts the clone with the real
// tag name instead of the placeholder "latest".
type releaseFetchedMsg struct {
	Name    string
	URL     string
	TagName string
	Err     error
}

// cloneCmd runs gitops.Clone in a goroutine. After a successful
// clone it detects the default branch so the caller can store the
// real branch name (not a hardcoded "main") in the config.
func cloneCmd(url, destDir, branch string, a cloneDoneMsg) tea.Cmd {
	return func() tea.Msg {
		a.Err = gitops.Clone(context.Background(), url, destDir, branch)
		if a.Err == nil {
			a.DefaultBranch = gitops.DefaultBranch(destDir)
		}
		return a
	}
}

// fetchLatestReleaseCmd calls the GitHub API for the latest
// release tag.
func fetchLatestReleaseCmd(client *gh.Client, owner, repo, name, url string) tea.Cmd {
	return func() tea.Msg {
		rel, err := client.LatestRelease(owner, repo)
		if err != nil {
			return releaseFetchedMsg{Name: name, URL: url, Err: err}
		}
		if rel == nil {
			return releaseFetchedMsg{
				Name: name, URL: url,
				Err: fmt.Errorf("no releases found for %s/%s", owner, repo),
			}
		}
		return releaseFetchedMsg{Name: name, URL: url, TagName: rel.TagName}
	}
}

// viewAddForm renders the URL input screen.
func viewAddForm(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Add addon "))
	b.WriteString("\n\n")
	b.WriteString("Enter git URL (HTTPS or SSH):\n\n")
	b.WriteString("> " + m.AddInput)
	if m.AddInput == "" {
		b.WriteString(dimStyle.Render("https://github.com/owner/Addon"))
	}
	b.WriteString("\n\n")
	if m.AddError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.AddError))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Render("enter submit • esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// updateAddForm handles key events on the URL input screen.
func updateAddForm(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.Screen = screenList
		return *m, nil
	case "backspace":
		if len(m.AddInput) > 0 {
			m.AddInput = m.AddInput[:len(m.AddInput)-1]
		}
		return *m, nil
	case "enter":
		return submitAddForm(m)
	}

	for _, r := range key.Runes {
		if r >= 32 && r < 127 {
			m.AddInput += string(r)
		}
	}
	return *m, nil
}

// submitAddForm validates the URL and either shows the GitHub
// release picker or starts cloning directly for non-GitHub repos.
func submitAddForm(m *Model) (tea.Model, tea.Cmd) {
	url := strings.TrimSpace(m.AddInput)
	if err := addon.ValidateURL(url); err != nil {
		m.AddError = err.Error()
		return *m, nil
	}
	name, err := addon.DeriveName(url)
	if err != nil {
		m.AddError = err.Error()
		return *m, nil
	}
	if m.Config.AddonByName(name) != nil {
		m.AddError = fmt.Sprintf("addon %q is already tracked", name)
		return *m, nil
	}

	if !isGitHubURL(url) {
		// Non-GitHub: clone on whatever the remote's default branch is.
		cmd := m.startClone(name, url, addon.TrackModeBranch, "")
		return *m, cmd
	}

	// GitHub: show release picker so the user can choose.
	if _, _, err := gh.ParseOwnerRepo(url); err != nil {
		m.AddError = err.Error()
		return *m, nil
	}
	m.AddInput = url
	m.AddReleases = nil
	m.AddPickedRelease = ""
	m.Screen = screenReleasePicker
	return *m, nil
}

// startClone populates PendingAddon, switches to the progress
// screen, and returns the async clone command. If the destination
// directory already exists and is a git repo, it skips the clone
// and adds the addon to tracking immediately.
func (m *Model) startClone(name, url, mode, target string) tea.Cmd {
	m.PendingAddon.Name = name
	m.PendingAddon.URL = url
	m.PendingAddon.Mode = mode
	m.PendingAddon.Target = target

	destDir, _ := m.WowPath.AddonPath(name)
	addonsRoot := string(m.WowPath)

	// Already cloned (new style — .lazyaddons/<name>/.git)?
	newRepoDir := filepath.Join(addonsRoot, ".lazyaddons", name)
	if st, err := os.Stat(newRepoDir); err == nil && st.IsDir() {
		if gi, err := os.Stat(filepath.Join(newRepoDir, ".git")); err == nil && gi.IsDir() {
			m.Config.UpsertAddon(config.Addon{
				Name: name, URL: url,
				TrackMode: mode, TrackTarget: target,
			})
			m.Statuses[name] = StatusOK
			m.Screen = screenList
			return nil
		}
	}
	// Already cloned (legacy .repo style)?
	oldRepoDir := filepath.Join(addonsRoot, name+".repo")
	if st, err := os.Stat(oldRepoDir); err == nil && st.IsDir() {
		if gi, err := os.Stat(filepath.Join(oldRepoDir, ".git")); err == nil && gi.IsDir() {
			m.Config.UpsertAddon(config.Addon{
				Name: name, URL: url,
				TrackMode: mode, TrackTarget: target,
			})
			m.Statuses[name] = StatusOK
			m.Screen = screenList
			return nil
		}
	}
	// Folder exists but is NOT a git repo — offer to back up and replace.
	if st, err := os.Stat(destDir); err == nil && st.IsDir() {
		m.ReplaceFolder = destDir
		m.ReplaceName = name
		m.ReplaceURL = url
		m.ReplaceMode = mode
		m.ReplaceTarget = target
		m.Screen = screenConfirmReplace
		return nil
	}

	m.startProgress(fmt.Sprintf("Cloning %s...", name), 1, 1)

	return tea.Batch(
		spinnerCmd(),
		cloneCmd(url, destDir, target, cloneDoneMsg{
		Name:   name,
		URL:    url,
		Mode:   mode,
		Target: target,
	}),
)
}

// handleCloneDone processes a finished clone operation.
func (m *Model) handleCloneDone(msg cloneDoneMsg) {
	m.ProgressLabel = ""
	if msg.Err != nil {
		m.ErrMessage = fmt.Sprintf("Clone failed for %s: %v", msg.Name, msg.Err)
		m.Screen = screenError
		return
	}

	destDir, _ := m.WowPath.AddonPath(msg.Name)
	addonsRoot := string(m.WowPath)

	promoted, err := addon.PromoteAddonDirs(addonsRoot, destDir)
	if err != nil {
		m.ErrMessage = fmt.Sprintf("Failed to restructure %s: %v", msg.Name, err)
		m.Screen = screenError
		return
	}

	if len(promoted) == 0 {
		m.ErrMessage = fmt.Sprintf("No valid .toc found in %s.", msg.Name)
		m.Screen = screenError
		return
	}

	// The addon name is derived from the .toc-bearing folder inside
	// the repo, not from the URL. This handles repos like
	// "CleanerChat-WotLK" whose actual addon folder is "CleanerChat".
	actualName := msg.Name
	if len(promoted) > 0 {
		actualName = promoted[0]
	}

	// If the actual name differs from the clone name, rename the
	// directories to match.
	if !strings.EqualFold(actualName, msg.Name) {
		oldAddon := filepath.Join(addonsRoot, msg.Name)
		newAddon := filepath.Join(addonsRoot, actualName)
		oldRepo := filepath.Join(addonsRoot, ".lazyaddons", msg.Name)
		newRepo := filepath.Join(addonsRoot, ".lazyaddons", actualName)
		_ = os.Rename(oldAddon, newAddon)
		_ = os.Rename(oldRepo, newRepo)
	}

	m.Config.UpsertAddon(config.Addon{
		Name:        actualName,
		URL:         msg.URL,
		TrackMode:   msg.Mode,
		TrackTarget: defaultTrackTarget(msg.Target, msg.DefaultBranch),
		SubModules:  subModules(promoted, actualName),
	})
	m.Statuses[actualName] = StatusOK
	m.refreshAddonMeta(actualName)
	m.Screen = screenList
}

func isGitHubURL(u string) bool {
	return gh.IsGitHubHost(u)
}

func viewReleasePicker(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Choose track mode "))
	b.WriteString("\n\n")
	b.WriteString(promptStyle.Render(m.AddInput))
	b.WriteString("\n\n")
	b.WriteString("1. Track main branch\n")
	b.WriteString("2. Track latest release\n")
	if m.AddError != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("Error: " + m.AddError))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("1 branch • 2 release • esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func updateReleasePicker(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.Screen = screenList
		return *m, nil
	case "1":
		name, _ := addon.DeriveName(m.AddInput)
		return *m, m.startClone(name, m.AddInput, addon.TrackModeBranch, "")
	case "2":
		name, _ := addon.DeriveName(m.AddInput)
		owner, repo, err := gh.ParseOwnerRepo(m.AddInput)
		if err != nil {
			m.ErrMessage = err.Error()
			m.Screen = screenError
			return *m, nil
		}
		m.startProgress("Fetching latest release...", 1, 1)
		return *m, tea.Batch(
			spinnerCmd(),
			fetchLatestReleaseCmd(m.GitHub, owner, repo, name, m.AddInput),
		)
	}
	return *m, nil
}

// subModules returns all promoted names except the main one.
func subModules(promoted []string, main string) []string {
	var out []string
	for _, name := range promoted {
		if !strings.EqualFold(name, main) {
			out = append(out, name)
		}
	}
	return out
}

// defaultTrackTarget returns target if non-empty, otherwise falls
// back to the detected default branch. If neither is available it
// returns "main" as a last resort.
func defaultTrackTarget(target, detected string) string {
	if target != "" {
		return target
	}
	if detected != "" {
		return detected
	}
	return "main"
}

// viewConfirmReplace renders the "folder exists — back up and replace?" prompt.
func viewConfirmReplace(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Folder already exists "))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf(
		"%s already exists in AddOns\nbut is not tracked by lazyaddons.",
		m.ReplaceName,
	))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("> Replace"))
	b.WriteString(" — back up existing folder to .backup/, then clone fresh\n")
	b.WriteString("  ")
	b.WriteString(dimStyle.Render("Cancel"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter replace • esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// updateConfirmReplace handles key presses on the replace-confirmation screen.
func updateConfirmReplace(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		return doReplace(m)
	case "esc":
		m.ReplaceFolder = ""
		m.Screen = screenList
		return *m, nil
	}
	return *m, nil
}

// doReplace backs up the existing addon folder, then proceeds with clone.
func doReplace(m *Model) (tea.Model, tea.Cmd) {
	destDir := m.ReplaceFolder
	name := m.ReplaceName
	url := m.ReplaceURL
	mode := m.ReplaceMode
	target := m.ReplaceTarget
	m.ReplaceFolder = ""

	// Back up the existing folder before cloning.
	bk := backup.New(string(m.WowPath))
	if err := bk.Create(name); err != nil {
		m.ErrMessage = fmt.Sprintf("Failed to back up %s: %v", name, err)
		m.Screen = screenError
		return *m, nil
	}

	// Remove the old folder so clone can create it fresh.
	if err := os.RemoveAll(destDir); err != nil {
		m.ErrMessage = fmt.Sprintf("Failed to remove old %s: %v", name, err)
		m.Screen = screenError
		return *m, nil
	}

	// Proceed with clone using the normal flow.
	m.startProgress(fmt.Sprintf("Cloning %s...", name), 1, 1)
	return *m, tea.Batch(
		spinnerCmd(),
		cloneCmd(url, destDir, target, cloneDoneMsg{
			Name:   name,
			URL:    url,
			Mode:   mode,
			Target: target,
		}),
	)
}
