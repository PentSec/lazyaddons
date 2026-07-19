package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ghLookPath is exec.LookPath for "gh". Package-level for testability.
var ghLookPath = exec.LookPath

// resolveGitHubToken returns a GitHub token for API auth, trying in order:
// 1. GITHUB_TOKEN env var
// 2. GH_TOKEN env var (gh CLI convention)
// 3. `gh auth token` CLI output (if gh is available)
// Returns empty string if none is available.
func resolveGitHubToken() string {
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("GH_TOKEN")); token != "" {
		return token
	}
	if ghPath, err := ghLookPath("gh"); err == nil {
		var out bytes.Buffer
		cmd := exec.Command(ghPath, "auth", "token")
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			if token := strings.TrimSpace(out.String()); token != "" {
				return token
			}
		}
	}
	return ""
}

// SelfUpdate downloads the latest release binary and replaces
// the current executable. Returns an error if the operation fails.
func SelfUpdate(releaseTag string) error {
	version := strings.TrimPrefix(releaseTag, "v")
	assetName := assetNameForPlatform(version)
	url := fmt.Sprintf(
		"%s/repos/%s/%s/releases/download/%s/%s",
		"https://github.com", RepoOwner, RepoName, releaseTag, assetName,
	)

	// Download the archive to a temp file.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("selfupdate: %w", err)
	}
	req.Header.Set("User-Agent", "lazyaddons-selfupdate")
	if token := resolveGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("selfupdate: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("selfupdate: download returned %d for %s", resp.StatusCode, url)
	}

	// Write archive to a temp file.
	tmpArchive, err := os.CreateTemp("", "lazyaddons-archive-*")
	if err != nil {
		return fmt.Errorf("selfupdate: temp file: %w", err)
	}
	tmpArchivePath := tmpArchive.Name()
	defer os.Remove(tmpArchivePath)

	if _, err := io.Copy(tmpArchive, resp.Body); err != nil {
		tmpArchive.Close()
		return fmt.Errorf("selfupdate: write archive: %w", err)
	}
	tmpArchive.Close()

	// Extract the binary from the archive.
	binPath, err := extractBinary(tmpArchivePath, assetName)
	if err != nil {
		return fmt.Errorf("selfupdate: extract: %w", err)
	}
	defer os.Remove(binPath)

	// Get current executable path.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("selfupdate: executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("selfupdate: resolve symlink: %w", err)
	}

	// Write to a temp file in the same directory as the executable
	// so the atomic rename works (same filesystem).
	tmp, err := os.CreateTemp(filepath.Dir(exe), "lazyaddons-*.tmp")
	if err != nil {
		return fmt.Errorf("selfupdate: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	src, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("selfupdate: open binary: %w", err)
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
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

// extractBinary extracts the lazyaddons binary from an archive
// (tar.gz on Linux/macOS, zip on Windows) and returns the path
// to the extracted binary.
func extractBinary(archivePath, archiveName string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") || strings.Contains(archiveName, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "lazyaddons" || name == "lazyaddons.exe" {
			out, err := os.CreateTemp("", "lazyaddons-bin-*")
			if err != nil {
				return "", err
			}
			rc, err := f.Open()
			if err != nil {
				out.Close()
				return "", err
			}
			if _, err := io.Copy(out, rc); err != nil {
				rc.Close()
				out.Close()
				return "", err
			}
			rc.Close()
			out.Close()
			return out.Name(), nil
		}
	}
	return "", fmt.Errorf("lazyaddons binary not found in zip")
}

func extractFromTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		name := filepath.Base(hdr.Name)
		if name == "lazyaddons" {
			out, err := os.CreateTemp("", "lazyaddons-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return out.Name(), nil
		}
	}
	return "", fmt.Errorf("lazyaddons binary not found in tar.gz")
}

// assetNameForPlatform returns the asset filename for the current
// OS and architecture. The naming matches GoReleaser's name_template:
// "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}".
func assetNameForPlatform(version string) string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("lazyaddons_%s_windows_%s.zip", version, runtime.GOARCH)
	default:
		return fmt.Sprintf("lazyaddons_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	}
}
