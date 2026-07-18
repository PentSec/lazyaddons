package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// SelfUpdate downloads the latest release binary and replaces
// the current executable. Returns an error if the operation fails.
func SelfUpdate(releaseTag string) error {
	assetName := assetNameForPlatform()
	url := fmt.Sprintf(
		"%s/repos/%s/%s/releases/download/%s/%s",
		"https://github.com", RepoOwner, RepoName, releaseTag, assetName,
	)

	// Download the binary to a temp file.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("selfupdate: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("selfupdate: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("selfupdate: download returned %d", resp.StatusCode)
	}

	// Write to a temp file in the same directory as the executable
	// so the atomic rename works (same filesystem).
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("selfupdate: executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("selfupdate: resolve symlink: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), "lazyaddons-*.tmp")
	if err != nil {
		return fmt.Errorf("selfupdate: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("selfupdate: write binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("selfupdate: close temp: %w", err)
	}

	// Make the temp file executable.
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("selfupdate: chmod: %w", err)
	}

	// Atomic rename. On Linux/macOS this is an atomic filesystem
	// operation if src and dst are on the same filesystem.
	if err := os.Rename(tmpPath, exe); err != nil {
		return fmt.Errorf("selfupdate: replace binary: %w", err)
	}

	return nil
}

// assetNameForPlatform returns the asset filename for the current
// OS and architecture. The naming matches GoReleaser's name_template:
// "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}". The version is in the
// release download URL path, not in the filename.
func assetNameForPlatform() string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("lazyaddons_windows_%s.zip", runtime.GOARCH)
	default:
		return fmt.Sprintf("lazyaddons_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	}
}
