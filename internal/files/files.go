// Package files renders a live "what changed" view for a single directory.
//
// Used by the `muster files <dir>` hidden subcommand to populate the right tmux
// pane next to a claude session. Output is plain text with ANSI color codes;
// no boxes, borders, or animations.
package files

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
	reset  = "\x1b[0m"
)

// Status is the parsed git state for one directory.
type Status struct {
	Header    string // "apps/api [main ↑2 ↓0]"
	Modified  []FileLine
	Staged    []FileLine
	Untracked []FileLine
}

// FileLine is one row in the rendered list.
type FileLine struct {
	Status string // M, A, D, R, ?
	Path   string
	Add    int // diff --numstat
	Del    int
}

// Compute returns the current Status of dir, scoped via `git -C dir`.
// Returns a header-only Status if dir is not a git repo.
func Compute(dir string) Status {
	s := Status{Header: header(dir)}

	// Modified (unstaged) — `git diff --numstat -- .` for line counts +
	// `git status -s --porcelain -- .` for the M/?/A markers.
	porcelain, err := runGit(dir, "status", "-s", "--porcelain", "--", ".")
	if err != nil {
		return s
	}
	unstagedNumstat := numstat(dir, "diff", "--numstat", "--", ".")
	stagedNumstat := numstat(dir, "diff", "--cached", "--numstat", "--", ".")

	scanner := bufio.NewScanner(strings.NewReader(porcelain))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}
		stagedCh := line[0]
		unstagedCh := line[1]
		path := strings.TrimSpace(line[3:])

		if unstagedCh != ' ' && unstagedCh != 0 {
			if unstagedCh == '?' {
				s.Untracked = append(s.Untracked, FileLine{Status: "?", Path: path})
			} else {
				add, del := unstagedNumstat[path][0], unstagedNumstat[path][1]
				s.Modified = append(s.Modified, FileLine{
					Status: string(unstagedCh),
					Path:   path,
					Add:    add,
					Del:    del,
				})
			}
		}
		if stagedCh != ' ' && stagedCh != 0 && stagedCh != '?' {
			add, del := stagedNumstat[path][0], stagedNumstat[path][1]
			s.Staged = append(s.Staged, FileLine{
				Status: string(stagedCh),
				Path:   path,
				Add:    add,
				Del:    del,
			})
		}
	}
	return s
}

// Render writes a colorized view of dir's git state to w.
func Render(w io.Writer, dir string, width int) {
	s := Compute(dir)
	fmt.Fprintln(w, bold+s.Header+reset)
	fmt.Fprintln(w)
	if len(s.Modified) > 0 {
		fmt.Fprintln(w, dim+"modified"+reset)
		for _, f := range s.Modified {
			fmt.Fprintln(w, "  "+colorize(red, f, width))
		}
	}
	if len(s.Staged) > 0 {
		fmt.Fprintln(w, dim+"staged"+reset)
		for _, f := range s.Staged {
			fmt.Fprintln(w, "  "+colorize(green, f, width))
		}
	}
	if len(s.Untracked) > 0 {
		fmt.Fprintln(w, dim+"untracked"+reset)
		for _, f := range s.Untracked {
			fmt.Fprintln(w, "  "+colorize(yellow, f, width))
		}
	}
	if len(s.Modified)+len(s.Staged)+len(s.Untracked) == 0 {
		fmt.Fprintln(w, dim+"clean"+reset)
	}
}

func colorize(color string, f FileLine, width int) string {
	prefix := color + f.Status + " " + reset
	path := truncatePath(f.Path, width-6)
	if f.Add == 0 && f.Del == 0 {
		return prefix + path
	}
	return prefix + path + " " + dim + fmt.Sprintf("+%d -%d", f.Add, f.Del) + reset
}

// truncatePath shortens a path from the LEFT when it exceeds maxLen,
// preferring to cut at a path-separator boundary so the result reads as
// "…/<latest-segments>".
func truncatePath(p string, maxLen int) string {
	if maxLen <= 0 || len(p) <= maxLen {
		return p
	}
	// Walk separators from the left and pick the latest one whose tail
	// (including the leading "…") still fits in maxLen.
	for i := 0; i < len(p); i++ {
		if p[i] != '/' {
			continue
		}
		tail := p[i:] // includes the leading slash
		if 1+len(tail) <= maxLen {
			return "…" + tail
		}
	}
	// No separator boundary fits — fall back to a hard cut.
	if maxLen <= 1 {
		return "…"
	}
	return "…" + p[len(p)-(maxLen-1):]
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

// header returns "<basename> [<branch> ↑N ↓M]" or just "<basename>" for non-git.
func header(dir string) string {
	base := filepath.Base(dir)
	out, err := runGit(dir, "status", "-b", "--porcelain=v1")
	if err != nil {
		return base
	}
	scanner := bufio.NewScanner(strings.NewReader(out))
	if !scanner.Scan() {
		return base
	}
	first := scanner.Text()
	// Format: "## main...origin/main [ahead 2, behind 1]" or "## main"
	if !strings.HasPrefix(first, "## ") {
		return base
	}
	rest := strings.TrimPrefix(first, "## ")
	branch := rest
	ahead, behind := 0, 0
	if i := strings.Index(rest, "..."); i >= 0 {
		branch = rest[:i]
		tail := rest[i:]
		if j := strings.Index(tail, "[ahead "); j >= 0 {
			fmt.Sscanf(tail[j:], "[ahead %d", &ahead)
		}
		if j := strings.Index(tail, "behind "); j >= 0 {
			fmt.Sscanf(tail[j:], "behind %d", &behind)
		}
	}
	switch {
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("%s [%s ↑%d ↓%d]", base, branch, ahead, behind)
	case ahead > 0:
		return fmt.Sprintf("%s [%s ↑%d]", base, branch, ahead)
	case behind > 0:
		return fmt.Sprintf("%s [%s ↓%d]", base, branch, behind)
	default:
		return fmt.Sprintf("%s [%s]", base, branch)
	}
}

// numstat parses `git diff [--cached] --numstat` into a map[path][2]int{add, del}.
func numstat(dir string, args ...string) map[string][2]int {
	out, err := runGit(dir, args...)
	if err != nil {
		return nil
	}
	m := make(map[string][2]int)
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		var add, del int
		fmt.Sscanf(fields[0], "%d", &add)
		fmt.Sscanf(fields[1], "%d", &del)
		m[fields[2]] = [2]int{add, del}
	}
	return m
}
