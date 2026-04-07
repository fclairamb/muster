// Package render formats registered directories for display.
package render

import (
	"path/filepath"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/repoinfo"
)

// Line returns the human-readable label for a single registered directory.
//
// For GitHub-backed repos: "<prefix>/<repo> [<branch>]".
// For local non-GitHub dirs: the basename, with no branch.
//
// prefix is the abbreviation derived by orgprefix.Derive for info.Org. It
// may be empty for non-GitHub entries.
func Line(d config.Dir, info repoinfo.Info, prefix string) string {
	if info.IsGitHub {
		return prefix + "/" + info.Repo + " [" + info.Branch + "]"
	}
	return filepath.Base(d.Path)
}
