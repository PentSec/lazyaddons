package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
	"github.com/pentsec/lazyaddons/internal/app"
)

var (
	colorText = lipgloss.Color("51")
)

// minInner is the smallest border interior we allow, so the logo
// still fits even on very narrow terminals.
const minInner = 56

// Header returns the lazyaddons ASCII-art logo with ANSI colors,
// padded to the given content width (inner width of the border).
func Header(width int) string {
	width = max(width, minInner)
	text := lipgloss.NewStyle().Foreground(colorText).Bold(true)

	r1 := pad(" "+
		text.Render("_")+
		"                         "+
		text.Render("_")+"       "+
		text.Render("_")+"     "+
		text.Render("_")+"                 ", width)

	r2 := pad(text.Render("| |    __ _ _____   _     / \\   __| | __| | ___  _ __  ___ "), width)

	r3 := pad(text.Render("| |   / _` |_  / | | |   / _ \\ / _` |/ _` |/ _ \\| '_ \\/ __|"), width)

	r4 := pad(text.Render("| |__| (_| |/ /| |_| |  / ___ \\ (_| | (_| | (_) | | | \\__ \\"), width)

	r5 := pad(text.Render("|_____\\__,_/___|\\__, | /_/   \\_\\__,_|\\__,_|\\___/|_| |_|___/"), width)

	r6 := pad("                "+
		text.Render("|___/")+
		"                                      ", width)

	return lipgloss.JoinVertical(lipgloss.Left,
		r1, r2, r3, r4, r5, r6,
	)
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func visibleLen(s string) int {
	count := 0
	inEscape := false
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			inEscape = true
			i++
			continue
		}
		if inEscape {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEscape = false
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		count++
	}
	return count
}

func pad(s string, width int) string {
	v := visibleLen(s)
	if v > width {
		// Truncate to fit, preserving ANSI codes.
		return truncateVisible(s, width)
	}
	if v < width {
		return s + repeat(" ", width-v)
	}
	return s
}

// truncateVisible truncates s so its visible length is at most n,
// preserving any trailing ANSI reset sequences.
func truncateVisible(s string, n int) string {
	count := 0
	inEscape := false
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			inEscape = true
			i++
			continue
		}
		if inEscape {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEscape = false
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		count++
		if count == n {
			return s[:i+size]
		}
		i += size
	}
	return s
}

// Footer returns the active-profile indicator on the left edge and
// the version on the right edge of the given content width. When no
// profile is active the indicator shows "Profile: none" so the user
// always knows whether they're operating on a real profile or the
// pre-profile-addition empty state.
func (m *Model) Footer(width int) string {
	faint := lipgloss.NewStyle().Faint(true)
	version := faint.Render("lazyaddons v" + app.Version)
	profileName := "none"
	if m != nil && m.ActiveProfile != nil && m.ActiveProfile.Name != "" {
		profileName = m.ActiveProfile.Name
	}
	profile := faint.Render("Profile: " + profileName)

	gap := width - visibleLen(profile) - visibleLen(version)
	if gap < 1 {
		gap = 1
	}
	return profile + repeat(" ", gap) + version
}

// WrapFrame wraps arbitrary content in a rounded border with a
// multi-color gradient that blends across the frame perimeter. The
// given width is the border interior, so the border is drawn around
// a content area of exactly width columns.
func WrapFrame(content string, width int) string {
	width = max(width, minInner)
	var padded []string
	for _, line := range strings.Split(content, "\n") {
		padded = append(padded, pad(line, width))
	}
	content = strings.Join(padded, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForegroundBlend(
			charmtone.Cherry,
			charmtone.Charple,
			charmtone.Guac,
			charmtone.Charple,
			charmtone.Sriracha,
		).
		Width(width + 2).
		Render(content)
}
