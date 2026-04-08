// Package gitignore manages the per-clone exclude file (.git/info/exclude)
// for repositories muster touches.
//
// We use .git/info/exclude rather than the tracked .gitignore so muster
// never produces an unwanted diff in the user's working tree. The effect is
// the same: git treats listed paths as ignored.
package gitignore

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// EnsureMusterIgnored makes sure ".muster/" is listed in
// repoRoot/.git/info/exclude. It is a no-op when the entry already exists,
// when repoRoot is not a git repository (no .git directory), or when the
// exclude file can't be written. Errors are returned for the caller to log
// but never fatal — failing to ignore is annoying, not catastrophic.
func EnsureMusterIgnored(repoRoot string) error {
	if repoRoot == "" {
		return errors.New("empty repo root")
	}
	gitDir := filepath.Join(repoRoot, ".git")
	fi, err := os.Stat(gitDir)
	if err != nil || !fi.IsDir() {
		// Not a standard git checkout (worktree, bare, or non-repo) — skip.
		return nil
	}
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	const entry = ".muster/"
	existing, _ := os.ReadFile(excludePath)
	if hasEntry(existing, entry) {
		return nil
	}
	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("# added by muster\n")
	buf.WriteString(entry)
	buf.WriteByte('\n')
	return os.WriteFile(excludePath, buf.Bytes(), 0o644)
}

func hasEntry(content []byte, entry string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == entry || line == strings.TrimSuffix(entry, "/") {
			return true
		}
	}
	return false
}
