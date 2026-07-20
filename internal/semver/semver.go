package semver

import (
	"strconv"
	"strings"
)

// Parse extracts major.minor.patch from a version tag string.
// Leading "v"/"V" is stripped. Non-numeric segments are treated as 0.
// Returns a [3]int and true if the tag produced at least 1 segment.
func Parse(tag string) ([3]int, bool) {
	s := strings.TrimLeft(tag, "vV")
	parts := strings.SplitN(s, ".", 4)
	var seg [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		seg[i], _ = strconv.Atoi(parts[i])
	}
	return seg, len(parts) > 0
}

// Version holds parsed major.minor.patch version numbers.
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseStrict extracts exactly 3 numeric segments. Returns ok=false
// if the tag does not match the expected shape.
func ParseStrict(tag string) (Version, bool) {
	t := strings.TrimLeft(tag, "vV")
	parts := strings.SplitN(t, ".", 3)
	if len(parts) != 3 {
		return Version{}, false
	}
	major, ok1 := atoi(parts[0])
	minor, ok2 := atoi(parts[1])
	patch, ok3 := atoi(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return Version{}, false
	}
	return Version{major, minor, patch}, true
}

func atoi(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// Compare returns >0 if a is newer, <0 if b is newer, 0 if equal.
func Compare(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}
