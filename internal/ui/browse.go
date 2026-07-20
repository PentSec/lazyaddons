package ui

import (
	"fmt"
	"os"
	"path/filepath"
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

// listDirs returns directories in the given path, sorted by name.
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
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	return dirs, nil
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
