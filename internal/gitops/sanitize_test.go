package gitops

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGitSan(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not in PATH: %v", err)
	}
}

func TestSanitizeSegment_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"parent traversal", "../etc/passwd", true},
		{"nested traversal", "a/../../b", true},
		{"empty", "", true},
		{"null byte", "bad\x00name", true},
		{"plain name", "MyAddon", false},
		{"spaces allowed", "My Add On", false},
		{"unicode allowed", "名称-Addon", false},
		{"dots in name", "My.Addon.v2", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := SanitizeSegment(tc.in)
			if tc.want && err == nil {
				t.Errorf("SanitizeSegment(%q) = nil, want error", tc.in)
			}
			if !tc.want && err != nil {
				t.Errorf("SanitizeSegment(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

func TestSanitizeSegment_RoundTrip(t *testing.T) {
	t.Parallel()
	got, err := SanitizeSegment("MyAddon")
	if err != nil {
		t.Fatalf("SanitizeSegment(MyAddon) error: %v", err)
	}
	if got != "MyAddon" {
		t.Errorf("SanitizeSegment = %q, want MyAddon", got)
	}
}

func TestSanitizeSegment_AllowsRelativeSafePath(t *testing.T) {
	t.Parallel()
	got, err := SanitizeSegment("subdir/MyAddon")
	if err != nil {
		t.Fatalf("SanitizeSegment(subdir/MyAddon) error: %v", err)
	}
	if !strings.HasSuffix(got, "subdir"+string(filepath.Separator)+"MyAddon") {
		t.Errorf("SanitizeSegment = %q, want subdir/MyAddon", got)
	}
}
