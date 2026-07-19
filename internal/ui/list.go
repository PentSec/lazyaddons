package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pentsec/lazyaddons/internal/config"
)

const (
	listOverhead      = 13 // border(2) + header(6) + col-header(1) + help(1) + footer(1) + search(2)
	listOverheadNoSer = 11 // same minus search lines
	listMinRows       = 5
)

// Color tokens for the status badges. They are top-level so the
// golden-file tests can refer to them by name in the future.
var (
	colorOK      = lipgloss.Color("42")  // green
	colorUpdate  = lipgloss.Color("220") // yellow
	colorError   = lipgloss.Color("196") // red
	colorInstall = lipgloss.Color("39")  // blue
)

// renderBadge returns a styled status character for the given
// status. The styling matches the spec: ✓ green, ↑ yellow,
// ✗ red, ⟳ blue.
func renderBadge(s AddonStatus) string {
	switch s {
	case StatusOK:
		return lipgloss.NewStyle().Foreground(colorOK).Render("✓")
	case StatusUpdate:
		return lipgloss.NewStyle().Foreground(colorUpdate).Render("↑")
	case StatusError:
		return lipgloss.NewStyle().Foreground(colorError).Render("✗")
	case StatusInstalling:
		return lipgloss.NewStyle().Foreground(colorInstall).Render("⟳")
	}
	return " "
}

// colWidths computes proportional column widths for the addon list
// given the inner content width (border interior).
type colWidths struct {
	Name, Ver, Track, Updated int
}

func computeCols(inner int) colWidths {
	// Overhead per row: marker(2) + 5 spaces + badge(1) + double-space(2)
	// = 2+1+1+1+1+1+1+2 = 10. Plus the name %-*s uses cols.Name+1.
	// Leave 15 chars for the status label on the right.
	fixed := 10 + 1 + 15 // overhead + name-pad + label
	avail := inner - fixed
	if avail < 55 {
		avail = 55
	}
	scale := float64(avail) / float64(55)
	return colWidths{
		Name:    int(22 * scale),
		Ver:     int(8 * scale),
		Track:   int(15 * scale),
		Updated: int(10 * scale),
	}
}

func viewList(m *Model) string {
	if m.Width == 0 {
		m.Width = 80
	}
	if m.Height == 0 {
		m.Height = 24
	}
	inner := m.Width - 2 // border characters
	if inner < minInner {
		inner = minInner
	}
	cols := computeCols(inner)

	var b strings.Builder

	// Self-update banner.
	if m.UpdateBanner != nil && m.UpdateBanner.UpdateAvailable {
		b.WriteString(updateBannerStyle.Render(
			fmt.Sprintf("⚡ lazyaddons %s available → %s  (press U to install)",
				m.UpdateBanner.LatestVersion, m.UpdateBanner.LatestURL),
		))
		b.WriteString("\n\n")
	}

	// Search bar.
	if m.SearchActive {
		b.WriteString(promptStyle.Render("  / ") + m.SearchQuery + dimStyle.Render("_"))
		b.WriteString("\n\n")
	}

	// Filter addons by search query.
	filtered := filterAddons(m.Config.Addons, m.SearchQuery)
	total := len(m.Config.Addons)
	shown := len(filtered)

	if shown == 0 {
		if total == 0 {
			b.WriteString(dimStyle.Render("No addons tracked yet. Press a to add one."))
		} else {
			b.WriteString(dimStyle.Render("No addons match your search."))
		}
		b.WriteString("\n")
		return b.String()
	}

	// Visible rows based on terminal height.
	overhead := listOverhead
	if !m.SearchActive {
		overhead = listOverheadNoSer
	}
	visible := m.Height - overhead
	if visible < listMinRows {
		visible = listMinRows
	}
	if visible > shown {
		visible = shown
	}

	// Clamp scroll offset.
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
	if m.ScrollOffset > shown-visible {
		m.ScrollOffset = shown - visible
	}
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}

	// Column headers.
	b.WriteString(headerStyle.Render(
		fmt.Sprintf("%-*s %-*s %-*s %-*s  %-*s  %s",
			cols.Name+2, "NAME",
			cols.Ver, "VER",
			cols.Track, "TRACK",
			1, "S",
			cols.Updated, "UPDATED",
			""),
	))
	b.WriteString("\n")

	// Render visible rows.
	viewStart := m.ScrollOffset
	viewEnd := viewStart + visible
	for i := viewStart; i < viewEnd; i++ {
		a := filtered[i]
		// Find real index for selection highlighting.
		realIdx := m.Config.AddonIndex(a.Name)
		marker := "  "
		if realIdx == m.Selection {
			marker = "> "
		}
		name := a.Name
		if len(name) > cols.Name {
			name = name[:cols.Name-3] + "..."
		}
		ver := a.Version
		if len(ver) > cols.Ver {
			ver = ver[:cols.Ver-3] + "..."
		}
		track := a.TrackMode + ":" + a.TrackTarget
		if len(track) > cols.Track {
			track = track[:cols.Track-3] + "..."
		}
		status := renderBadge(m.Statuses[a.Name])
		label := m.Statuses[a.Name].Label()
		updated := a.LastUpdated
		if len(updated) > cols.Updated {
			updated = updated[:cols.Updated]
		}
		row := fmt.Sprintf("%s %-*s %-*s %-*s %s %-*s  %s",
			marker,
			cols.Name+1, name,
			cols.Ver, ver,
			cols.Track, track,
			status,
			cols.Updated, updated,
			label)
		if realIdx == m.Selection {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}

	// Help bar with filter info.
	b.WriteString("\n")
	if m.SearchQuery != "" {
		help := fmt.Sprintf("%d/%d addons • esc clear", shown, total)
		b.WriteString(helpStyle.Render(help))
	} else {
		help := fmt.Sprintf("%d addons • / search • a add • d rm • u update • q quit", total)
		b.WriteString(helpStyle.Render(help))
	}
	return b.String()
}

// filterAddons returns addons whose names contain query (case-insensitive).
func filterAddons(addons []config.Addon, query string) []config.Addon {
	if query == "" {
		return addons
	}
	q := strings.ToLower(query)
	var out []config.Addon
	for _, a := range addons {
		if strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	return out
}

func updateList(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Search mode: intercept keys for the search input.
	if m.SearchActive {
		switch key.String() {
		case "esc":
			m.SearchActive = false
			m.SearchQuery = ""
			m.ScrollOffset = 0
			return *m, nil
		case "enter":
			m.SearchActive = false
			return *m, nil
		case "backspace":
			if len(m.SearchQuery) > 0 {
				m.SearchQuery = m.SearchQuery[:len(m.SearchQuery)-1]
			} else {
				m.SearchActive = false
			}
			m.ScrollOffset = 0
			return *m, nil
		case "up", "down", "left", "right":
			// Let navigation keys fall through to normal handling.
		default:
			if len(key.Runes) > 0 {
				m.SearchQuery += string(key.Runes)
				m.ScrollOffset = 0
			}
			return *m, nil
		}
	}

	switch key.String() {
	case "q", "esc":
		return *m, tea.Quit
	case "/":
		m.SearchActive = true
		m.SearchQuery = ""
		m.ScrollOffset = 0
		return *m, nil
	case "up", "k":
		if m.Selection > 0 {
			m.Selection--
			m.ensureVisible(m)
		}
		return *m, nil
	case "down", "j":
		if m.Selection < len(m.Config.Addons)-1 {
			m.Selection++
			m.ensureVisible(m)
		}
		return *m, nil
	case "a":
		m.Screen = screenAddForm
		m.AddInput = ""
		m.AddError = ""
		return *m, nil
	case "d":
			a := m.selectedAddon()
			if a == nil {
				return *m, nil
			}
			m.PendingRemove = a.Name
			m.Screen = screenConfirmRemove
			return *m, nil
	case "u":
		if len(m.Config.Addons) == 0 {
			return *m, nil
		}
		m.Screen = screenProgress
		m.ProgressLabel = "Checking for updates..."
		return *m, checkAllUpdatesCmd(string(m.WowPath), m.Config.Addons)
	case "enter":
		a := m.selectedAddon()
		if a == nil || m.Statuses[a.Name] != StatusUpdate {
			return *m, nil
		}
		m.Screen = screenProgress
		m.ProgressLabel = fmt.Sprintf("Updating %s...", a.Name)
		return *m, applyAddonCmd(string(m.WowPath), *a)
	case "U":
		if m.UpdateBanner == nil || !m.UpdateBanner.UpdateAvailable {
			return *m, nil
		}
		m.Screen = screenProgress
		m.ProgressLabel = "Downloading lazyaddons update..."
		return *m, selfUpdateCmd(m.UpdateBanner.LatestVersion)
	}
	return *m, nil
}

// ensureVisible adjusts ScrollOffset so the current Selection is
// within the visible window. Must be called after Selection changes.
func (m *Model) ensureVisible(_ *Model) {
	filtered := filterAddons(m.Config.Addons, m.SearchQuery)
	shown := len(filtered)
	if shown == 0 {
		return
	}
	overhead := listOverhead
	if !m.SearchActive {
		overhead = listOverheadNoSer
	}
	visible := m.Height - overhead
	if visible < listMinRows {
		visible = listMinRows
	}
	if visible > shown {
		visible = shown
	}

	// Find the position of the selected addon in the filtered list.
	selIdx := -1
	for i, a := range filtered {
		if a.Name == m.Config.Addons[m.Selection].Name {
			selIdx = i
			break
		}
	}
	if selIdx < 0 {
		// Selection not in filtered list — snap to nearest.
		if m.Selection < len(filtered) {
			selIdx = m.Selection
		} else {
			selIdx = shown - 1
		}
		if selIdx < 0 {
			selIdx = 0
		}
		m.Selection = m.Config.AddonIndex(filtered[selIdx].Name)
	}

	// Scroll to keep selection in view.
	if selIdx < m.ScrollOffset {
		m.ScrollOffset = selIdx
	}
	if selIdx >= m.ScrollOffset+visible {
		m.ScrollOffset = selIdx - visible + 1
	}
}

// doRemove performs the actual filesystem and config cleanup for a
// tracked addon. It is called from the confirmation screen handler.
func doRemove(m *Model) {
	addonsRoot := string(m.WowPath)
	a := m.Config.AddonByName(m.PendingRemove)
	if a == nil {
		return
	}

	// Remove sub-module folders tracked in config.
	for _, sub := range a.SubModules {
		_ = os.RemoveAll(filepath.Join(addonsRoot, sub))
	}

	// Remove main addon folder and repo directory (both styles).
	_ = os.RemoveAll(filepath.Join(addonsRoot, a.Name))
	_ = os.RemoveAll(filepath.Join(addonsRoot, ".lazyaddons", a.Name))
	_ = os.RemoveAll(filepath.Join(addonsRoot, a.Name+".repo"))

	m.Config.RemoveAddon(a.Name)
	delete(m.Statuses, a.Name)
	if m.Selection >= len(m.Config.Addons) {
		m.Selection = len(m.Config.Addons) - 1
	}
}

// updateConfirmRemove handles key presses on the remove-confirmation
// screen. y/enter confirms, n/esc cancels.
func updateConfirmRemove(m *Model, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "Y", "enter":
		doRemove(m)
		m.PendingRemove = ""
		m.Screen = screenList
		return *m, nil
	case "n", "N", "esc":
		m.PendingRemove = ""
		m.Screen = screenList
		return *m, nil
	}
	return *m, nil
}

// viewConfirmRemove renders the confirmation prompt.
func viewConfirmRemove(m *Model) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Remove Addon "))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf(
		"Are you sure you want to remove %q?\nThis will delete the addon folder and its repository.",
		m.PendingRemove,
	))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("y/enter confirm • n/esc cancel"))
	b.WriteString("\n")
	return b.String()
}

// Style cache for the list screen.
var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	headerStyle       = lipgloss.NewStyle().Bold(true).Underline(true)
	selectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dimStyle          = lipgloss.NewStyle().Faint(true)
	helpStyle         = lipgloss.NewStyle().Faint(true)
	errorStyle        = lipgloss.NewStyle().Foreground(colorError)
	promptStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	progressStyle     = lipgloss.NewStyle().Foreground(colorInstall)
	releaseSelStyle   = lipgloss.NewStyle().Bold(true)
	updateBannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Background(lipgloss.Color("58"))
)
