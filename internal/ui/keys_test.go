package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// charKey returns a KeyMsg for a single ASCII character.
func charKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// downKey returns a KeyMsg for the down arrow.
func downKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyDown} }

// upKey returns a KeyMsg for the up arrow.
func upKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyUp} }

// readFile returns the contents of a file in the package's
// testdata directory.
func readFile(t testing.TB, name string) string {
	t.Helper()
	data, err := readTestData(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// readArgs returns the args passed to the test binary.
func readArgs() []string { return os.Args[1:] }

// writeTestData writes content to a testdata file.
func writeTestData(name string, content []byte) error {
	return os.WriteFile(name, content, 0o644)
}

// trimTrailingSpaces removes trailing whitespace from each line
// of the input. Used to make golden file comparisons
// deterministic across lipgloss format changes.
func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// normaliseWhitespace collapses runs of spaces and trims each
// line. This is the looser comparison used for golden files:
// it ignores lipgloss padding tweaks while still verifying the
// structural content (addon names, status badges, headers).
func normaliseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, " \t")
		// Collapse 2+ spaces into 1.
		for strings.Contains(l, "  ") {
			l = strings.ReplaceAll(l, "  ", " ")
		}
		l = strings.TrimLeft(l, " ")
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}
