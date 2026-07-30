package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto"
	_ "crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/minio/selfupdate"
)

// ghLookPath is exec.LookPath for "gh". Package-level for testability.
var ghLookPath = exec.LookPath

// releaseDownloadBase is the base URL for GitHub release asset
// downloads. It is a var so tests can point it at a fake server.
var releaseDownloadBase = "https://github.com"

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

// verifyFileSHA256 hashes the file at path and compares its hex digest
// against expectedHex. Returns nil on match, an error otherwise.
func verifyFileSHA256(path, expectedHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := crypto.SHA256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expectedHex) {
		return fmt.Errorf("expected %s, got %s", expectedHex, got)
	}
	return nil
}

// fetchAssetChecksum downloads checksums.txt from the given release
// and returns the hex SHA256 for the named asset.
//
// Return contract (deliberate, see point 1 of the review):
//   - ("", nil) when checksums.txt is not published for this release
//     (HTTP 404). This is the only legitimate "skip verification" case
//     — older releases predate the checksum file. The caller logs the
//     skip so it is never invisible.
//   - ("", nil) when the file is published but the asset is not listed
//     in it. Treated the same as 404 for resilience.
//   - ("", err) for any other HTTP failure (500, 429 rate limit, 401
//     bad token, timeout, …). The caller hard-fails the update rather
//     than silently disabling integrity verification — a network
//     blip must not turn into an unverified swap.
//   - (hash, nil) when the entry is found.
//
// The format produced by GoReleaser is one "<hex-sha256>  <filename>"
// per line. strings.Fields already trims surrounding spaces and CR
// (verified), so \r\n endings common on Windows-generated files do not
// corrupt the parsed hash or name.
func fetchAssetChecksum(releaseTag, assetName string) (string, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/releases/download/%s/checksums.txt",
		releaseDownloadBase, RepoOwner, RepoName, releaseTag,
	)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("checksum request: %w", err)
	}
	req.Header.Set("User-Agent", "lazyaddons-selfupdate")
	if token := resolveGitHubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("checksum download: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// proceed below
	case http.StatusNotFound:
		// Release predates checksums.txt — skip verification,
		// but log so the skip is never invisible in production.
		log.Printf("selfupdate: checksums.txt not published for %s; skipping integrity check", releaseTag)
		return "", nil
	default:
		// 500, 429, 401, … — hard-fail instead of silently
		// disabling verification (see review point 1).
		return "", fmt.Errorf("checksum download returned %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("checksum read: %w", err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash, name := fields[0], fields[1]
		if name == assetName {
			return hash, nil
		}
	}
	// File published but our asset missing — unusual but treat as
	// skip-with-log so an in-progress release upload doesn't block.
	log.Printf("selfupdate: asset %q not listed in checksums.txt for %s; skipping integrity check", assetName, releaseTag)
	return "", nil
}

// SelfUpdate downloads the latest release binary and replaces
// the current executable. Returns an error if the operation fails.
func SelfUpdate(releaseTag string) error {
	version := strings.TrimPrefix(releaseTag, "v")
	assetName := assetNameForPlatform(version)
	url := fmt.Sprintf(
		"%s/%s/%s/releases/download/%s/%s",
		releaseDownloadBase, RepoOwner, RepoName, releaseTag, assetName,
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

	// Verify the archive against the checksum published in checksums.txt
	// before extracting or replacing anything. This detects corrupted
	// downloads and tampered release assets. The hash published by
	// GoReleaser covers the whole archive (tar.gz/zip), not the inner
	// binary, so we hash the archive file on disk.
	expectedHex, err := fetchAssetChecksum(releaseTag, assetName)
	if err != nil {
		return fmt.Errorf("selfupdate: checksum: %w", err)
	}
	if expectedHex != "" {
		if err := verifyFileSHA256(tmpArchivePath, expectedHex); err != nil {
			return fmt.Errorf("selfupdate: checksum mismatch: %w", err)
		}
	}

	// Extract the binary from the archive.
	binPath, err := extractBinary(tmpArchivePath, assetName)
	if err != nil {
		return fmt.Errorf("selfupdate: extract: %w", err)
	}
	defer os.Remove(binPath)

	// Resolve the current executable path so the old binary can be
	// preserved next to it for rollback.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("selfupdate: executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("selfupdate: resolve symlink: %w", err)
	}

	// Stream the extracted binary into minio/selfupdate, which writes
	// it to a temporary .new file and validates it without touching
	// the running executable. On Windows the running .exe is locked,
	// so a plain os.Rename over it fails with ERROR_ACCESS_DENIED —
	// minio/selfupdate handles this by renaming the current binary
	// to .old (permitted even while locked) and moving .new into place,
	// then hiding .old on Windows so it can be removed on next start.
	bin, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("selfupdate: open extracted binary: %w", err)
	}
	defer bin.Close()

	opts := selfupdate.Options{
		TargetPath:  exe,
		TargetMode:  0o755,
		OldSavePath: exe + ".old",
	}

	if err := selfupdate.PrepareAndCheckBinary(bin, opts); err != nil {
		return fmt.Errorf("selfupdate: prepare: %w", err)
	}

	if err := selfupdate.CommitBinary(opts); err != nil {
		// TODO(rollback-test): the CommitBinary-fails + RollbackError
		// branch is the most fragile path of this whole feature and is
		// NOT exercised by any automated test nor by manual validation
		// yet. minio/selfupdate restores the original binary from
		// OldSavePath on commit failure; if that also fails it returns
		// a RollbackError wrapping both. Before shipping a release that
		// relies on this, add a test that simulates a Commit failure
		// (e.g. corrupt the .new file after Prepare, or make the target
		// dir read-only) and asserts the .old binary is restored and
		// RollbackError is surfaced. See review point 2.
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("selfupdate: commit failed and rollback also failed: %w (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("selfupdate: commit: %w", err)
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
