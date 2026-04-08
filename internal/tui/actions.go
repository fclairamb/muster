package tui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fclairamb/muster/internal/gitstats"
	"github.com/fclairamb/muster/internal/session"
	"github.com/fclairamb/muster/internal/state"
)

// GitRunner abstracts git command execution so tests can record calls.
type GitRunner interface {
	Run(dir string, args ...string) (string, error)
	IsDirty(dir string) (bool, error)
}

// Opener abstracts spawning external GUI tools (file manager, editor).
type Opener interface {
	Open(binary, path string) error
}

// Deps bundles the side-effecting collaborators a Model needs to act.
type Deps struct {
	Session     session.Manager
	Git         GitRunner
	Opener      Opener
	FileManager string // resolved file manager binary
	Editor      string // resolved editor binary

	// AttachCmdFunc, when non-nil, is preferred over Session.Attach for the
	// Enter action. It returns the *exec.Cmd that the model wraps in
	// tea.ExecProcess so the TUI suspends, runs tmux attach in the
	// foreground, and resumes on detach.
	AttachCmdFunc func(slug string) *exec.Cmd

	// Unregister persists removal of a registered (non-worktree) entry —
	// drops it from the registry and uninstalls its Claude Code hooks.
	// Without this, removed entries reappear on the next launch.
	Unregister func(path string) error

	// ReadState reads the current state for one entry from disk. Used by
	// the periodic refresh to keep the rendered status in sync even when
	// the watcher's events are missed (e.g. during a tea.ExecProcess
	// suspension while the user is attached to a tmux session). The
	// returned State.Ts is used for staleness decay.
	ReadState func(repoRoot, slug string) state.State

	// ClearState writes a fresh KindIdle state for the given slug. Used
	// after the user detaches from a session that was sitting on
	// KindReady — the green dot should clear to white once viewed.
	ClearState func(repoRoot, slug string) error

	// GitStats, when non-nil, is called during refresh to recompute the
	// per-entry git counts (unpushed / modified / untracked) shown in the
	// list. Tests typically leave it nil.
	GitStats func(path string) gitstats.Stats
}

// WorktreePathFor returns the on-disk path muster will create for a new
// worktree of repo on branch. Used by both the git arg builder and the
// post-create entry insertion so they agree on where the worktree lives.
func WorktreePathFor(repo, branch string) string {
	base := filepath.Base(repo)
	return filepath.Join(repo, ".muster", "worktrees", base+"-"+slugifyBranch(branch))
}

// BuildWorktreeAddArgs returns the git argv for adding a new worktree.
//
// The new worktree lives at <repo>/.muster/worktrees/<repo>-<branch-slug> and
// branches off <branch> from HEAD.
func BuildWorktreeAddArgs(repo, branch string) []string {
	target := WorktreePathFor(repo, branch)
	return []string{"-C", repo, "worktree", "add", target, "-b", branch}
}

// BuildBranchListArgs returns the git argv for listing local branch names,
// one per line, no decoration.
func BuildBranchListArgs(repo string) []string {
	return []string{"-C", repo, "branch", "--format=%(refname:short)"}
}

// BuildCheckoutArgs returns the git argv for checking out a branch in repo.
// When create is true, the branch is created from HEAD.
func BuildCheckoutArgs(repo, branch string, create bool) []string {
	args := []string{"-C", repo, "checkout"}
	if create {
		args = append(args, "-b")
	}
	return append(args, branch)
}

// ParseBranchList splits the output of `git branch --format=%(refname:short)`
// into a slice of branch names. Empty lines are skipped.
func ParseBranchList(out string) []string {
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// BuildWorktreeRemoveArgs returns the git argv for removing a worktree.
func BuildWorktreeRemoveArgs(worktreePath string, force bool) []string {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
	return args
}

// ErrInvalidBranchName is returned when a user-supplied branch name fails validation.
var ErrInvalidBranchName = errors.New("invalid branch name")

// ValidateBranchName rejects empty input, whitespace, and path-escape sequences.
// It is intentionally stricter than git itself; muster is the gatekeeper for the
// .muster/worktrees directory layout.
func ValidateBranchName(name string) error {
	if name == "" {
		return ErrInvalidBranchName
	}
	if strings.ContainsAny(name, " \t\n") {
		return ErrInvalidBranchName
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "-") {
		return ErrInvalidBranchName
	}
	if strings.Contains(name, "..") {
		return ErrInvalidBranchName
	}
	return nil
}

// slugifyBranch converts a branch name to a filesystem-safe slug for the
// worktree directory name.
func slugifyBranch(b string) string {
	r := strings.NewReplacer("/", "-", " ", "-")
	return r.Replace(b)
}
