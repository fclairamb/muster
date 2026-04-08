// Package render formats registered directories for display.
package render

import (
	"path/filepath"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/repoinfo"
)

// Line returns the human-readable label for a single registered directory.
//
// For GitHub-backed repos:
//   - "<prefix>/<repo> [<branch>]" when the registered path is the repo root
//   - "<prefix>/<repo> <subpath> [<branch>]" when it's a subdir of the repo
//
// For local non-GitHub dirs: the basename, with no branch.
//
// prefix is the abbreviation derived by orgprefix.Derive for info.Org. It
// may be empty for non-GitHub entries.
func Line(d config.Dir, info repoinfo.Info, prefix string) string {
	if info.IsGitHub {
		base := prefix + "/" + info.Repo
		if sub := subPath(d.Path, info.RepoRoot); sub != "" {
			base += " " + sub
		}
		return base + " [" + info.Branch + "]"
	}
	return filepath.Base(d.Path)
}

// subPath returns the path of dir relative to repoRoot, or "" if dir IS the
// repo root or the relative path can't be computed.
func subPath(dir, repoRoot string) string {
	if dir == "" || repoRoot == "" || dir == repoRoot {
		return ""
	}
	rel, err := filepath.Rel(repoRoot, dir)
	if err != nil || rel == "." || rel == "" {
		return ""
	}
	return rel
}
