// Package session manages tmux sessions running claude instances.
package session

import "os"

// Manager abstracts tmux operations so the rest of the code can be unit-tested.
type Manager interface {
	Start(slug, cwd string) error
	Has(slug string) bool
	Attach(slug string) error
	Kill(slug string) error
	List() ([]string, error)
}

// SocketName is the dedicated tmux socket name ssf uses to avoid polluting
// the user's default tmux server.
const SocketName = "ssf"

// SessionPrefix is prepended to every slug to form the tmux session name.
const SessionPrefix = "ssf-"

// claudeBinary returns the path/name of the claude binary, honoring
// $SSF_CLAUDE_BINARY for tests.
func claudeBinary() string {
	if v := os.Getenv("SSF_CLAUDE_BINARY"); v != "" {
		return v
	}
	return "claude"
}

// buildStartArgs returns the argv for spawning a detached tmux session that
// runs the given claude binary in cwd.
func buildStartArgs(slug, cwd, binary string) []string {
	return []string{
		"-L", SocketName,
		"new-session",
		"-d",
		"-s", SessionPrefix + slug,
		"-c", cwd,
		binary,
	}
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
