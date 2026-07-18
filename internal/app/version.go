// Package app holds application-level constants and helpers that
// are consumed by main and the UI layer.
package app

import (
	"runtime/debug"
	"strings"
)

// Version is resolved by main from the build-time ldflags variable.
// It defaults to "dev" and is set to the resolved semver before the
// TUI starts.
var Version = "dev"

// buildInfoReader is swappable for testing.
var buildInfoReader = debug.ReadBuildInfo

// ResolveVersion determines the application version from ldflags
// with a fallback to debug.BuildInfo. Priority:
//
//  1. ldflags override (GoReleaser builds) — returns the tag.
//  2. debug.BuildInfo.Main.Version (go install @tag builds).
//  3. "dev" — local development build; self-update is skipped.
func ResolveVersion(ldflagsVersion string) string {
	if ldflagsVersion != "dev" {
		return ldflagsVersion
	}
	info, ok := buildInfoReader()
	if !ok {
		return "dev"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(v, "v")
}
