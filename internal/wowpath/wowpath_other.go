// Package wowpath non-Windows stub. detectWindowsCandidates is the
// Windows branch of DetectCandidates and is implemented in
// wowpath_windows.go. This stub allows DetectCandidates to call it
// unconditionally while letting the linker drop the body on
// non-Windows targets; the call site gates it behind a runtime.GOOS
// check so this stub is unreachable in practice.

//go:build !windows

package wowpath

// detectWindowsCandidates is a no-op on non-Windows targets.
// The real implementation lives in wowpath_windows.go and is only
// reachable when runtime.GOOS == "windows".
func detectWindowsCandidates() []string { return nil }
