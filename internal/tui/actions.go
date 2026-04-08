package tui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fclairamb/ssf/internal/session"
	"github.com/fclairamb/ssf/internal/state"
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
}

// BuildWorktreeAddArgs returns the git argv for adding a new worktree.
//
// The new worktree lives at <repo>/.ssf/worktrees/<repo>-<branch-slug> and
// branches off <branch> from HEAD.
func BuildWorktreeAddArgs(repo, branch string) []string {
	slug := slugifyBranch(branch)
	base := filepath.Base(repo)
	target := filepath.Join(repo, ".ssf", "worktrees", base+"-"+slug)
	return []string{"-C", repo, "worktree", "add", target, "-b", branch}
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
// It is intentionally stricter than git itself; ssf is the gatekeeper for the
// .ssf/worktrees directory layout.
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
