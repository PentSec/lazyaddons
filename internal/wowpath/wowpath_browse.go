// Package wowpath browser-entry helpers shared between platforms.
// BrowseStart returns the initial directory (or pseudo-root) that the
// interactive path browser should open on. On Windows this is the
// empty string sentinel meaning "show drives"; on Unix-like systems
// it is the user's home directory.
package wowpath

import "os"

// The empty string is used by the UI to mean "list drive roots" on
// Windows. The browser, on seeing WowBrowsePath equal to "" while
// browseStartIsRoot is set, calls ListBrowseRoots to populate its
// entries instead of os.ReadDir. The sentinel is only honoured when
// the platform has set browseStartIsRoot (see wowpath_windows.go);
// on other platforms "" is never a legitimate browse path, so
// IsBrowseRoot reports false for it there.

// IsBrowseRoot reports whether the given browser path is the special
// "show drive roots" sentinel. Callers that distinguish home (Unix)
// from drive-list (Windows) should check this before treating the
// path as a regular directory.
func IsBrowseRoot(path string) bool {
	return browseStartIsRoot && path == ""
}

// ListBrowseRoots returns the top-level directories the interactive
// browser should display when at its root view. On Windows it is the
// fixed and removable drives reported by GetLogicalDriveStrings. On
// Unix-like systems it is a single entry pointing at the user's home
// directory, which is the conventional browsing root there.
func ListBrowseRoots() []string {
	if browseStartImpl == nil {
		home, _ := os.UserHomeDir()
		return []string{home}
	}
	return browseStartImpl()
}

// BrowseStart returns the path the browser should initialise its
// cursor at. On non-Windows targets this is the user's home directory;
// on Windows it returns the empty sentinel so the browser renders the
// drive list instead of diving into C:\Users\… directly.
func BrowseStart() string {
	if browseStartIsRoot {
		return ""
	}
	home, _ := os.UserHomeDir()
	return home
}

// browseStartImpl and browseStartIsRoot are set by platform-specific
// files (wowpath_windows.go / wowpath_other.go) to override the
// default Unix behaviour. They remain nil/false on platforms without
// an override.
var (
	browseStartImpl   func() []string
	browseStartIsRoot bool
)
