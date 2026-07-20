// Package safepath provides a single function for validating
// user-supplied path segments against path traversal and null
// byte injection attacks. It is the canonical home for this
// check; callers in other packages delegate to it.
package safepath

import (
	"errors"
	"path/filepath"
	"strings"
)

// Validate cleans a user-supplied path segment and checks for
// path traversal and null bytes. Returns the cleaned segment
// or an error.
func Validate(segment string) (string, error) {
	if segment == "" {
		return "", errors.New("safepath: empty segment")
	}
	if strings.ContainsRune(segment, 0) {
		return "", errors.New("safepath: null byte in segment")
	}
	if !filepath.IsLocal(segment) {
		return "", errors.New("safepath: path traversal detected")
	}
	cleaned := filepath.Clean(segment)
	return cleaned, nil
}
