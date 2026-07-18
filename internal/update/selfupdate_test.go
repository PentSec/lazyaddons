package update

import (
	"runtime"
	"strings"
	"testing"
)

func TestAssetNameForPlatform_returnsNonEmpty(t *testing.T) {
	t.Parallel()
	name := assetNameForPlatform()
	if name == "" {
		t.Error("assetNameForPlatform() returned empty string")
	}
}

func TestAssetNameForPlatform_lowercase(t *testing.T) {
	t.Parallel()
	name := assetNameForPlatform()
	if strings.ToLower(name) != name {
		t.Errorf("assetNameForPlatform() = %q, expected lowercase", name)
	}
}

func TestAssetNameForPlatform_containsOSArch(t *testing.T) {
	t.Parallel()
	name := assetNameForPlatform()
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("assetNameForPlatform() = %q, missing GOOS %q", name, runtime.GOOS)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("assetNameForPlatform() = %q, missing GOARCH %q", name, runtime.GOARCH)
	}
}

func TestAssetNameForPlatform_platformExtension(t *testing.T) {
	t.Parallel()
	name := assetNameForPlatform()
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(name, ".zip") {
			t.Errorf("assetNameForPlatform() = %q, expected .zip on windows", name)
		}
	} else {
		if !strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("assetNameForPlatform() = %q, expected .tar.gz", name)
		}
	}
}
