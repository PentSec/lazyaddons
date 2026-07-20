package ui

import (
	"fmt"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/pentsec/lazyaddons/internal/app"
)

var (
	colorPurple = lipgloss.Color("99")
	colorText   = lipgloss.Color("51")
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

// Footer returns the version string padded to the given content width.
func Footer(width int) string {
	style := lipgloss.NewStyle().Faint(true)
	version := app.Version
	return pad(style.Render(fmt.Sprintf("lazyaddons v%s", version)), width)
}

// WrapFrame wraps arbitrary content in the purple box-drawing border
// used by the header. Every line is padded to width and flanked by
// ║ on both sides, with a top ╔═══╗ and bottom ╚═══╝.
func WrapFrame(content string, width int) string {
	width = max(width, minInner)
	border := lipgloss.NewStyle().Foreground(colorPurple).Bold(true)
	sep := border.Render("║")
	top := border.Render("╔" + repeat("═", width) + "╗")
	bot := border.Render("╚" + repeat("═", width) + "╝")

	var lines []string
	lines = append(lines, top)
	for _, line := range splitLines(content) {
		lines = append(lines, sep+pad(line, width)+sep)
	}
	lines = append(lines, bot)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// splitLines breaks a string into lines, preserving empty lines.
func splitLines(s string) []string {
	return splitByNewline(s)
}

func splitByNewline(s string) []string {
	if s == "" {
		return []string{""}
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
