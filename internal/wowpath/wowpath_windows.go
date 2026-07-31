// Package wowpath Windows-specific helpers for enumerating drives
// and checking fixed install locations.
//
// This file uses golang.org/x/sys/windows for clean API access to
// GetLogicalDriveStrings. The Windows branch of DetectCandidates
// lives here; non-Windows targets use the stub in wowpath_other.go.

//go:build windows

package wowpath

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// windowsDrives returns the list of available drive roots (e.g.
// "C:\\", "D:\\") reported by the OS via GetLogicalDriveStrings.
// On error or when no drives are reported, it falls back to a
// conservative hard-coded list so the caller never returns an
// empty slice — the worst case is a few extra os.Stat calls on
// non-existent paths.
func windowsDrives() []string {
	const initialBuf = 256
	buf := make([]uint16, initialBuf)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil {
		return []string{"C:\\", "D:\\"}
	}
	// n counts the total chars (excluding the final terminator) that
	// *would* be written when the buffer is large enough; if it
	// exceeds our buffer we need more room.
	if n == 0 {
		return []string{"C:\\", "D:\\"}
	}
	if n >= uint32(len(buf)) {
		buf = make([]uint16, n+1)
		if _, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0]); err != nil {
			return []string{"C:\\", "D:\\"}
		}
	}

	var drives []string
	for i := 0; i < len(buf); {
		if buf[i] == 0 {
			break
		}
		// Decode one null-terminated UTF-16 string. UTF16ToString
		// returns the Go string and consumes up to (but not
		// including) the terminator, so we advance past the
		// terminator manually.
		s := windows.UTF16ToString(buf[i:])
		if s == "" {
			break
		}
		// Advance i by the number of UTF-16 code units we just
		// consumed (the runes that became s) plus the terminator.
		// UTF16PtrFromString would force another allocation; instead
		// we count code units by re-encoding — but the simpler and
		// equally correct path is to find the next zero.
		j := i
		for j < len(buf) && buf[j] != 0 {
			j++
		}
		i = j + 1

		// Skip drives that are not fixed or removable: network
		// drives and RAM disks are slow to stat and noisy.
		letter := strings.TrimSuffix(s, "\\")
		if len(letter) != 2 || letter[1] != ':' {
			continue
		}
		driveType := windows.GetDriveType(utf16Ptr(letter + "\\"))
		if driveType != windows.DRIVE_FIXED && driveType != windows.DRIVE_REMOVABLE {
			continue
		}
		drives = append(drives, s)
	}
	if len(drives) == 0 {
		return []string{"C:\\", "D:\\"}
	}
	return drives
}

// utf16Ptr builds a UTF-16 NUL-terminated pointer for the given Go
// string. The returned slice stays referenced for the lifetime of
// the call; we never retain it past the syscall.
func utf16Ptr(s string) (p *uint16) {
	wide, _ := windows.UTF16PtrFromString(s)
	return wide
}

// windowsFixedLocations returns the well-known fixed paths where WoW
// is installed under "World of Warcraft"-style folder names on each
// enumerated drive root. Each is stat'd directly (no recursive
// walk), so the list can be larger than the legacy hard-coded set
// without hurting detection latency. Only entries that exist as
// directories are returned.
func windowsFixedLocations(drives []string) []string {
	// Sub-folders of <drive>\ that commonly hold a WoW install.
	// Names are matched as-is; the filesystem lookup is
	// case-insensitive on Windows.
	subPaths := []string{
		"World of Warcraft",
		"World of Warcraft Classic",
		"World of Warcraft-3.3.5a",
		"Program Files\\World of Warcraft",
		"Program Files (x86)\\World of Warcraft",
		"Program Files\\World of Warcraft Classic",
		"Program Files (x86)\\World of Warcraft Classic",
		"Games\\World of Warcraft",
		"Games\\World of Warcraft Classic",
		"Games\\WoW",
		"Games\\WoW 3.3.5a",
	}
	var out []string
	for _, drive := range drives {
		for _, sub := range subPaths {
			cand := filepath.Join(drive, sub)
			if info, err := os.Stat(cand); err == nil && info.IsDir() {
				out = append(out, cand)
			}
		}
	}
	return out
}

// detectWindowsCandidates is the Windows branch of DetectCandidates.
// It prefers cheap os.Stat at known fixed locations across all
// enumerated drives, falling back to a shallow scan of obvious
// game folders only when no fixed location matches. Anything deeper
// is left to the interactive browser.
func detectWindowsCandidates() []string {
	drives := windowsDrives()

	var candidates []string
	for _, root := range windowsFixedLocations(drives) {
		p, err := Resolve(root)
		if err == nil {
			candidates = append(candidates, string(p))
		}
	}

	// Last-resort: a shallow listing of per-drive game folders, in
	// case the user dropped WoW into a non-standard name. We cap
	// depth at 1 and count at 10 per folder to keep the TUI
	// responsive.
	for _, drive := range drives {
		for _, gameDir := range []string{"Games", "Jogos", "Spiele", "Jeux"} {
			root := filepath.Join(drive, gameDir)
			for _, hit := range shallowFindAddOns(root, 1, 10) {
				p, err := Resolve(hit)
				if err == nil {
					candidates = append(candidates, string(p))
				}
			}
		}
	}

	return dedupeCandidates(candidates)
}

// shallowFindAddOns walks dir up to maxDepth levels looking for
// Interface\AddOns subdirectories, capping the result at maxFound.
// It is the depth-bounded counterpart of findAddOnsInDir used by the
// Windows last-resort path so a single slow drive cannot stall
// detection.
func shallowFindAddOns(root string, maxDepth, maxFound int) []string {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	var found []string
	depth := 0
	var walk func(string)
	walk = func(dir string) {
		if depth > maxDepth || len(found) >= maxFound {
			return
		}
		depth++
		defer func() { depth-- }()

		addonsPath := filepath.Join(dir, "Interface", "AddOns")
		if st, err := os.Stat(addonsPath); err == nil && st.IsDir() {
			found = append(found, addonsPath)
			return
		}
		if name := filepath.Base(dir); strings.EqualFold(name, "AddOns") || strings.EqualFold(name, "addons") {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				found = append(found, dir)
				return
			}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			walk(filepath.Join(dir, e.Name()))
		}
	}
	walk(root)
	return found
}

func init() {
	browseStartImpl = func() []string { return windowsDrives() }
	browseStartIsRoot = true
}
