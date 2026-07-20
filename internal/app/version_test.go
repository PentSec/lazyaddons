package app

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion_ldflagsOverride(t *testing.T) {
	t.Parallel()
	got := ResolveVersion("2.1.0")
	if got != "2.1.0" {
		t.Errorf("ResolveVersion(2.1.0) = %q, want 2.1.0", got)
	}
}

// The following tests mutate the package-level buildInfoReader
// to inject fake debug.BuildInfo values. They must NOT run in
// parallel: t.Parallel + unsynchronised global mutation trips the
// race detector. They are also very fast, so serial execution
// has no measurable cost.

func TestResolveVersion_ldflagsDevNoBuildInfo(t *testing.T) {
	defer func(orig func() (*debug.BuildInfo, bool)) {
		buildInfoReader = orig
	}(buildInfoReader)

	buildInfoReader = func() (*debug.BuildInfo, bool) { return nil, false }
	got := ResolveVersion("dev")
	if got != "dev" {
		t.Errorf("ResolveVersion(dev) with no BuildInfo = %q, want dev", got)
	}
}

func TestResolveVersion_ldflagsDevBuildInfoDevel(t *testing.T) {
	defer func(orig func() (*debug.BuildInfo, bool)) {
		buildInfoReader = orig
	}(buildInfoReader)

	buildInfoReader = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
	}
	got := ResolveVersion("dev")
	if got != "dev" {
		t.Errorf("ResolveVersion(dev) with (devel) BuildInfo = %q, want dev", got)
	}
}

func TestResolveVersion_ldflagsDevBuildInfoEmpty(t *testing.T) {
	defer func(orig func() (*debug.BuildInfo, bool)) {
		buildInfoReader = orig
	}(buildInfoReader)

	buildInfoReader = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: ""}}, true
	}
	got := ResolveVersion("dev")
	if got != "dev" {
		t.Errorf("ResolveVersion(dev) with empty BuildInfo = %q, want dev", got)
	}
}

func TestResolveVersion_ldflagsDevBuildInfoTagged(t *testing.T) {
	defer func(orig func() (*debug.BuildInfo, bool)) {
		buildInfoReader = orig
	}(buildInfoReader)

	buildInfoReader = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.5.0"}}, true
	}
	got := ResolveVersion("dev")
	if got != "1.5.0" {
		t.Errorf("ResolveVersion(dev) with v1.5.0 BuildInfo = %q, want 1.5.0", got)
	}
}
