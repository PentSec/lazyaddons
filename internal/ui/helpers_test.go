package ui

import (
	"os"
	"path/filepath"
	"regexp"
)

// readTestData reads a file relative to the package's testdata
// directory. The `name` is either bare (e.g. "list_view.golden")
// or already prefixed with "testdata/".
func readTestData(name string) ([]byte, error) {
	if !filepath.HasPrefix(name, "testdata/") {
		name = filepath.Join("testdata", name)
	}
	return os.ReadFile(name)
}

// stripANSI removes ANSI escape sequences from a string. The
// golden file is checked against a stripped version because the
// terminal may or may not include color codes depending on the
// environment.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
