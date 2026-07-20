package ui

import (
	"strings"
	"testing"

	"github.com/pentsec/lazyaddons/internal/app"
)

// TestFooter_ShowsProfileName verifies the Footer line contains
// the active profile name when one is set.
func TestFooter_ShowsProfileName(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.Config = goldenTestProfile()
	m.SetActiveProfile(m.Config.FindProfileByID(m.Config.ActiveProfileID))

	got := stripANSI(m.Footer(80))
	if !strings.Contains(got, "Profile:") {
		t.Errorf("Footer missing 'Profile:' label: %q", got)
	}
	if !strings.Contains(got, "Retail") {
		t.Errorf("Footer missing active profile name 'Retail': %q", got)
	}
	// The version should still appear.
	if !strings.Contains(got, app.Version) && !strings.Contains(got, "lazyaddons v") {
		t.Errorf("Footer missing version: %q", got)
	}
}

// TestFooter_NoProfileShowsDefault verifies the Footer renders
// "Profile: none" when no active profile is set.
func TestFooter_NoProfileShowsDefault(t *testing.T) {
	t.Parallel()
	m := NewModel()
	// No Config, no ActiveProfile.

	got := stripANSI(m.Footer(80))
	if !strings.Contains(got, "Profile: none") {
		t.Errorf("Footer = %q, want it to contain 'Profile: none'", got)
	}
}
