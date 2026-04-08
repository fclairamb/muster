// Package gitstats computes a small set of git status counts for a directory.
//
// Used by the TUI directory listing to surface "needs attention" indicators
// (unpushed commits, unstaged tracked changes, untracked files) without
// having to attach to the session.
package gitstats

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// Stats is a per-entry rollup of the bits muster shows in the list.
type Stats struct {
	// Unpushed is the number of local commits ahead of upstream (repo-wide).
	// Zero when there's no upstream or HEAD is detached.
	Unpushed int
	// Behind is the number of upstream commits not yet in HEAD (repo-wide).
	// Only meaningful after a `git fetch` — the value reflects whatever the
	// last fetch left in the local refs.
	Behind int
	// Modified counts tracked files with unstaged changes inside scope.
	Modified int
	// Untracked counts files git doesn't know about inside scope.
	Untracked int
}

// Compute runs git inside dir and returns the stats scoped to that directory.
// dir may be a subdirectory of a repo — `git -C dir status -- .` naturally
// scopes the file counts. Unpushed is repo-wide.
func Compute(dir string) Stats {
	var s Stats
	if dir == "" {
		return s
	}
	// File counts: porcelain v1, scoped to "."
	if out, err := runGit(dir, "status", "-s", "--porcelain", "--", "."); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(out))
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) < 2 {
				continue
			}
			x, y := line[0], line[1]
			if x == '?' && y == '?' {
				s.Untracked++
				continue
			}
			if y != ' ' && y != 0 {
				s.Modified++
			}
		}
	}
	// Ahead / behind upstream in one shot. Errors (no upstream, detached
	// HEAD) leave both at zero.
	if out, err := runGit(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
		fields := strings.Fields(strings.TrimSpace(out))
		if len(fields) == 2 {
			behind, _ := strconv.Atoi(fields[0])
			ahead, _ := strconv.Atoi(fields[1])
			s.Behind = behind
			s.Unpushed = ahead
		}
	}
	return s
}

// Fetch runs `git fetch --quiet` inside dir. Used by the background refresh
// loop so Behind reflects what's actually on the remote.
func Fetch(dir string) error {
	if dir == "" {
		return nil
	}
	cmd := exec.Command("git", "fetch", "--quiet")
	cmd.Dir = dir
	return cmd.Run()
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
