package update

import (
	"crypto"
	_ "crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// saveRestoreGlobals snapshots the package-level vars that fetchAssetChecksum
// depends on and restores them after the test, so concurrent tests don't
// bleed state. Tests touching these vars MUST NOT run in parallel.
func saveRestoreGlobals(t *testing.T) {
	t.Helper()
	oldBase := releaseDownloadBase
	oldClient := httpClient
	t.Cleanup(func() {
		releaseDownloadBase = oldBase
		httpClient = oldClient
	})
}

// newFakeServer returns a httptest.Server whose handler is built by muxFactory.
// muxFactory receives the mux so individual tests can register routes.
func newFakeServer(t *testing.T, muxFactory func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	muxFactory(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchAssetChecksum_found(t *testing.T) {
	saveRestoreGlobals(t)
	body := "a3b0068b9eebd6bcfc88fabed22297d8ce5649efda035a8f660c284667d07a0d  lazyaddons_0.1.0_windows_amd64.zip\n" +
		"c655584521c1cfe44fcbadd1528dd2a84da7ae5cebbfb8a440eaf082c3090d28  lazyaddons_0.1.0_linux_amd64.tar.gz\n"
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.1.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.1.0", "lazyaddons_0.1.0_windows_amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a3b0068b9eebd6bcfc88fabed22297d8ce5649efda035a8f660c284667d07a0d"; got != want {
		t.Errorf("hash = %q, want %q", got, want)
	}
}

func TestFetchAssetChecksum_404_returnsEmptyNoError(t *testing.T) {
	saveRestoreGlobals(t)
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.0.1/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.0.1", "any-asset.zip")
	if err != nil {
		t.Fatalf("404 should return (\"\", nil), got err: %v", err)
	}
	if got != "" {
		t.Errorf("hash = %q, want empty (skip verification)", got)
	}
}

// TestFetchAssetChecksum_non404Error_returnsError is the core of review
// point 1: a server error (500, 429, 401, …) must NOT silently skip
// verification. If this test ever regresses to ("", nil), an outage of
// github.com would turn into silent unverified binary swaps in production.
func TestFetchAssetChecksum_non404Error_returnsError(t *testing.T) {
	saveRestoreGlobals(t)
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.1.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.1.0", "any-asset.zip")
	if err == nil {
		t.Fatalf("non-404 error must be returned as error, got (%q, nil)", got)
	}
	if got != "" {
		t.Errorf("on error, hash must be empty, got %q", got)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

func TestFetchAssetChecksum_rateLimited_returnsError(t *testing.T) {
	saveRestoreGlobals(t)
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.1.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.1.0", "any-asset.zip")
	if err == nil {
		t.Fatalf("429 rate limit must NOT silently skip verification, got (%q, nil)", got)
	}
	if got != "" {
		t.Errorf("on error, hash must be empty, got %q", got)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status 429, got: %v", err)
	}
}

func TestFetchAssetChecksum_assetNotListed_returnsEmptyNoError(t *testing.T) {
	saveRestoreGlobals(t)
	body := "c655584521c1cfe44fcbadd1528dd2a84da7ae5cebbfb8a440eaf082c3090d28  lazyaddons_0.1.0_linux_amd64.tar.gz\n"
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.1.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.1.0", "lazyaddons_0.1.0_windows_amd64.zip")
	if err != nil {
		t.Fatalf("missing asset line should skip-with-log, got err: %v", err)
	}
	if got != "" {
		t.Errorf("hash = %q, want empty (asset not listed)", got)
	}
}

// TestFetchAssetChecksum_crlfEndings verifies review point 4: a checksums.txt
// generated with \r\n line endings (e.g. edited on Windows or built on a
// Windows runner) must not leak CR into the parsed hash or filename, which
// would silently break the comparison.
func TestFetchAssetChecksum_crlfEndings(t *testing.T) {
	saveRestoreGlobals(t)
	body := "a3b0068b9eebd6bcfc88fabed22297d8ce5649efda035a8f660c284667d07a0d  lazyaddons_0.1.0_windows_amd64.zip\r\n" +
		"c655584521c1cfe44fcbadd1528dd2a84da7ae5cebbfb8a440eaf082c3090d28  lazyaddons_0.1.0_linux_amd64.tar.gz\r\n"
	srv := newFakeServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/pentsec/lazyaddons/releases/download/v0.1.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	})
	releaseDownloadBase = srv.URL
	httpClient = srv.Client()

	got, err := fetchAssetChecksum("v0.1.0", "lazyaddons_0.1.0_windows_amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a3b0068b9eebd6bcfc88fabed22297d8ce5649efda035a8f660c284667d07a0d"; got != want {
		t.Errorf("hash = %q, want %q (CR likely leaked into parsed value)", got, want)
	}
}

func TestVerifyFileSHA256_match(t *testing.T) {
	content := []byte("hello world")
	dir := t.TempDir()
	p := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	h := crypto.SHA256.New()
	h.Write(content)
	want := hex.EncodeToString(h.Sum(nil))
	if err := verifyFileSHA256(p, want); err != nil {
		t.Errorf("expected nil error on matching checksum, got: %v", err)
	}
}

func TestVerifyFileSHA256_mismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	err := verifyFileSHA256(p, wrong)
	if err == nil {
		t.Fatal("expected error on checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") && !strings.Contains(err.Error(), "expected") {
		t.Errorf("error should describe mismatch, got: %v", err)
	}
}

func TestVerifyFileSHA256_caseInsensitive(t *testing.T) {
	content := []byte("hello world")
	dir := t.TempDir()
	p := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	h := crypto.SHA256.New()
	h.Write(content)
	uppercase := strings.ToUpper(hex.EncodeToString(h.Sum(nil))) // hex of same hash, uppercase
	if err := verifyFileSHA256(p, uppercase); err != nil {
		t.Errorf("EqualFold comparison should accept uppercase hex, got: %v", err)
	}
}


