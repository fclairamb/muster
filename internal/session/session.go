// Package session manages tmux sessions running claude instances.
package session

import "os"

// Manager abstracts tmux operations so the rest of the code can be unit-tested.
type Manager interface {
	Start(slug, cwd string) error
	StartShell(slug, cwd string) error
	Has(slug string) bool
	Attach(slug string) error
	Kill(slug string) error
	List() ([]string, error)
}

// ShellSlugSuffix is appended to an entry slug to form the tmux session slug
// for the entry's shell session, so it never collides with the claude session.
const ShellSlugSuffix = "-sh"

// shellBinary returns the user's shell, honoring $SHELL with a bash fallback.
func shellBinary() string {
	if v := os.Getenv("SHELL"); v != "" {
		return v
	}
	return "bash"
}

// SocketName is the dedicated tmux socket name muster uses to avoid polluting
// the user's default tmux server.
const SocketName = "muster"

// SessionPrefix is prepended to every slug to form the tmux session name.
const SessionPrefix = "muster-"

// claudeBinary returns the path/name of the claude binary, honoring
// $MUSTER_CLAUDE_BINARY for tests.
func claudeBinary() string {
	if v := os.Getenv("MUSTER_CLAUDE_BINARY"); v != "" {
		return v
	}
	return "claude"
}

// buildStartArgs returns the argv for spawning a detached tmux session that
// runs the given claude binary in cwd, optionally followed by extra args
// (e.g. --dangerously-skip-permissions) passed straight through to claude.
//
// Each session is started with MUSTER_SLUG=<slug> in its environment via
// `new-session -e`. The hook subcommand muster installs into
// .claude/settings.local.json reads this env var to know which on-disk
// state file to write — that's how multiple parallel claude instances in
// the same repo route their state to distinct files while sharing one
// settings.local.json.
func buildStartArgs(slug, cwd, binary string, claudeArgs []string) []string {
	args := []string{
		"-L", SocketName,
		"new-session",
		"-d",
		"-s", SessionPrefix + slug,
		"-e", "MUSTER_SLUG=" + slug,
		"-c", cwd,
		binary,
	}
	args = append(args, claudeArgs...)
	return args
}

// buildAttachArgs returns the argv to attach to an existing session.
func buildAttachArgs(slug string) []string {
	return []string{"-L", SocketName, "attach", "-t", SessionPrefix + slug}
}

// buildHasArgs returns the argv to query whether a session exists.
func buildHasArgs(slug string) []string {
	return []string{"-L", SocketName, "has-session", "-t", SessionPrefix + slug}
}

// buildKillArgs returns the argv to kill a session.
func buildKillArgs(slug string) []string {
	return []string{"-L", SocketName, "kill-session", "-t", SessionPrefix + slug}
}

// buildListArgs returns the argv to list session names on our socket.
func buildListArgs() []string {
	return []string{"-L", SocketName, "list-sessions", "-F", "#{session_name}"}
}
