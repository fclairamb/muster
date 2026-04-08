// Package slug derives stable identifiers from absolute filesystem paths.
package slug

import (
	"crypto/sha256"
	"encoding/hex"
)

// Length is the number of hex chars used in a slug.
const Length = 12

// Slug returns a stable 12-char hex identifier for an absolute path.
// It is deterministic and never includes the org abbreviation; it is safe
// to use as part of tmux session names and on-disk file paths.
func Slug(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:])[:Length]
}

// For returns the slug for a (path, instance) pair. instance == ""
// yields the bare directory slug; a non-empty instance yields
// "<dirSlug>-<instance>", which is the canonical identifier for parallel
// claude sessions sharing one directory. The instance string is assumed
// to already be sanitized (validated by the caller via ValidateInstance).
func For(absPath, instance string) string {
	s := Slug(absPath)
	if instance == "" {
		return s
	}
	return s + "-" + instance
}
