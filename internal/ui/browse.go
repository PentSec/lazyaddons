package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

// browseEntry represents a directory in the file browser.
type browseEntry struct {
	Name string
	Path string
}

// viewWowBrowse renders the directory browser.
func viewWowBrowse(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Browse WoW folder "))
	b.WriteString("\n\n")

	// Drive-root view (Windows): WowBrowsePath is the sentinel set
	// byBrowseStart on Windows. We render the enumerated drives
	// directly and skip the breadcrumb / parent-up affordances, which
	// do not apply at the "My Computer" pseudo-root.
	if wowpath.IsBrowseRoot(m.WowBrowsePath) {
		b.WriteString(dimStyle.Render("Drives"))
		b.WriteString("\n\n")
		roots := driveRoots()
		for i, d := range roots {
			marker := "  "
			if i == m.WowBrowseSel {
				marker = "> "
			}
			entry := marker + d
			if i == m.WowBrowseSel {
				b.WriteString(selectedStyle.Render(entry))
			} else {
				b.WriteString(entry)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓ navigate • enter open drive • esc cancel"))
		b.WriteString("\n")
		return b.String()
	}

	// Breadcrumbs
	parts := strings.Split(filepath.Clean(m.WowBrowsePath), string(filepath.Separator))
	if len(parts) > 4 {
		parts = parts[len(parts)-4:]
	}
	b.WriteString(dimStyle.Render("🏠 /" + strings.Join(parts, " / ")))
	b.WriteString("\n\n")

	dirs, err := listDirs(m.WowBrowsePath)
	if err != nil {
		b.WriteString(errorStyle.Render("Cannot read: " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter open • backspace up • esc cancel"))
		return b.String()
	}

	// Show parent dir as first entry.
	b.WriteString(dimStyle.Render("  ../  (go up)"))
	b.WriteString("\n")

	for i, d := range dirs {
		marker := "  "
		if i == m.WowBrowseSel {
			marker = "> "
		}
		name := d.Name
		hint := ""
		if isWowFolder(name) {
			hint = dimStyle.Render("  ← WoW?")
		}
		entry := fmt.Sprintf("%s %s/%s", marker, name, hint)
		if i == m.WowBrowseSel {
			b.WriteString(selectedStyle.Render(entry))
		} else {
			b.WriteString(entry)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.WowBrowseError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.WowBrowseError))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate • enter open • backspace up • s select this folder • esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// updateWowBrowse handles keyboard events for the directory browser.
func updateWowBrowse(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Drive-root view (Windows): keys are navigate/open/cancel only.
	// backspace-up does nothing — we are already at the pseudo-root.
	if wowpath.IsBrowseRoot(m.WowBrowsePath) {
		switch key.String() {
		case "esc":
			m.Screen = screenWowPath
			return *m, nil
		case "up", "k":
			if m.WowBrowseSel > 0 {
				m.WowBrowseSel--
			}
			return *m, nil
		case "down", "j":
			roots := driveRoots()
			if m.WowBrowseSel < len(roots)-1 {
				m.WowBrowseSel++
			}
			return *m, nil
		case "enter":
			roots := driveRoots()
			if m.WowBrowseSel >= 0 && m.WowBrowseSel < len(roots) {
				m.WowBrowsePath = roots[m.WowBrowseSel]
				m.WowBrowseSel = 0
			}
			return *m, nil
		}
		return *m, nil
	}

	switch key.String() {
	case "esc":
		m.Screen = screenWowPath
		return *m, nil
	case "up", "k":
		if m.WowBrowseSel > 0 {
			m.WowBrowseSel--
		}
		return *m, nil
	case "down", "j":
		dirs, _ := listDirs(m.WowBrowsePath)
		if m.WowBrowseSel < len(dirs)-1 {
			m.WowBrowseSel++
		}
		return *m, nil
	case "enter":
		dirs, _ := listDirs(m.WowBrowsePath)
		if m.WowBrowseSel >= 0 && m.WowBrowseSel < len(dirs) {
			m.WowBrowsePath = dirs[m.WowBrowseSel].Path
			m.WowBrowseSel = 0
		}
		return *m, nil
	case "backspace":
		parent := filepath.Dir(m.WowBrowsePath)
		if parent != m.WowBrowsePath {
			m.WowBrowsePath = parent
			m.WowBrowseSel = 0
		} else {
			// Already at the filesystem root of this drive: bounce
			// up to the drive-list pseudo-root (Windows only). On
			// other platforms there is no higher level, so stay put.
			if runtime.GOOS == "windows" {
				m.WowBrowsePath = wowpath.BrowseStart()
				m.WowBrowseSel = 0
			}
		}
		return *m, nil
	case "s":
		// Resolve the selected folder as the AddOns path.
		p, err := wowpath.Resolve(m.WowBrowsePath)
		if err != nil {
			m.WowBrowseError = err.Error()
			return *m, nil
		}
		m.WowBrowseError = ""
		if m.PendingProfileName != "" {
			m.PendingProfilePath = p.String()
			return acceptPath(m)
		}
		m.WowPath = p
		if m.ActiveProfile != nil {
			m.ActiveProfile.WoWPath = p.String()
		}
		m.Screen = screenList
		return *m, nil
	}
	return *m, nil
}

// listDirs returns directories in the given path, sorted with
// WoW-suggestive folder names surfaced to the top so the user can
// reach their installation in fewer keystrokes. The remaining
// directories retain alphabetical order.
func listDirs(path string) ([]browseEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs []browseEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && e.Name() != ".." {
			continue // skip hidden dirs
		}
		dirs = append(dirs, browseEntry{
			Name: e.Name(),
			Path: filepath.Join(path, e.Name()),
		})
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	// Reorder so WoW-suggestive entries lead the list. We use a
	// stable partition (two passes) so the alphabetical order inside
	// each subgroup is preserved.
	wow, rest := make([]browseEntry, 0, len(dirs)), make([]browseEntry, 0, len(dirs))
	for _, d := range dirs {
		if isWowFolder(d.Name) {
			wow = append(wow, d)
		} else {
			rest = append(rest, d)
		}
	}
	return append(wow, rest...), nil
}

// isWowFolder returns true if the folder name suggests it's WoW-related.
func isWowFolder(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range []string{"wow", "world of warcraft", "interface", "addons", "addon", "warcraft", "blizzard"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// driveRoots returns the top-level browsing roots for the current
// platform. On Windows this is the enumerated drive list (C:\, D:\…);
// elsewhere wowpath.ListBrowseRoots already returns the home dir,
// which the browser reaches through the non-sentinel path on Unix.
// We expose a thin alias here so viewWowBrowse does not need to
// import wowpath for the browse-root case, keeping the imports tidy
// despite the win/non-win split living in the wowpath package.
func driveRoots() []string {
	return wowpath.ListBrowseRoots()
}
