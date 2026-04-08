package tui

import (
	"sort"
	"time"

	"github.com/fclairamb/muster/internal/state"
)

// Entry is a single row in the TUI list. It can be a registered repo, a
// subdir of a repo, or a worktree under a repo. Indent controls vertical
// nesting in the rendered list.
type Entry struct {
	Display string
	Indent  int
	Kind    state.Kind
	// Path is the *registered* path. May be a subdir of a repo. Used as
	// the tmux session cwd, file-manager target, and registry key.
	Path string
	// RepoRoot is the git repo root that contains Path (or Path itself
	// for non-git dirs). State files live under RepoRoot/.ssf/state and
	// hooks are installed at RepoRoot/.claude/settings.json.
	RepoRoot   string
	Slug       string
	LastOpen   time.Time
	IsWorktree bool // worktrees get the git-worktree-remove flow; registered repos get unregistered
}

// kindRank maps a state.Kind to its sort priority (lower = higher up).
func kindRank(k state.Kind) int {
	switch k {
	case state.KindWaitingInput:
		return 0
	case state.KindReady:
		return 1
	case state.KindWorking:
		return 2
	case state.KindIdle:
		return 3
	default:
		return 4
	}
}

// SortEntries returns entries ordered by status, then last-opened desc.
// Indented children stay attached to their parent.
func SortEntries(entries []Entry) []Entry {
	// Group: gather indent==0 entries with the children that follow them.
	type group struct {
		root     Entry
		children []Entry
	}
	var groups []group
	for _, e := range entries {
		if e.Indent == 0 {
			groups = append(groups, group{root: e})
			continue
		}
		if len(groups) == 0 {
			// Orphan child: treat as root for stability.
			groups = append(groups, group{root: e})
			continue
		}
		groups[len(groups)-1].children = append(groups[len(groups)-1].children, e)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		ri, rj := kindRank(groups[i].root.Kind), kindRank(groups[j].root.Kind)
		if ri != rj {
			return ri < rj
		}
		return groups[i].root.LastOpen.After(groups[j].root.LastOpen)
	})
	out := make([]Entry, 0, len(entries))
	for _, g := range groups {
		out = append(out, g.root)
		out = append(out, g.children...)
	}
	return out
}

// FilterEntries returns the subset whose Display contains needle (case-insensitive).
// Children whose parent matches are included even if they don't match individually.
func FilterEntries(entries []Entry, needle string) []Entry {
	if needle == "" {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	parentMatch := false
	for _, e := range entries {
		match := containsFold(e.Display, needle)
		if e.Indent == 0 {
			parentMatch = match
			if match {
				out = append(out, e)
			}
			continue
		}
		if parentMatch || match {
			out = append(out, e)
		}
	}
	return out
}

func containsFold(s, sub string) bool {
	return indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	if len(sub) > len(s) {
		return -1
	}
	ls, lsub := toLowerASCII(s), toLowerASCII(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
