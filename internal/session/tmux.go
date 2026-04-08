package session

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// Tmux is the real Manager implementation backed by tmux on a dedicated socket.
type Tmux struct {
	// ClaudeArgs are appended to the claude binary when starting a new
	// session. nil means "no extra args".
	ClaudeArgs []string
}

// NewTmux returns a new tmux Manager with no extra claude args.
func NewTmux() *Tmux { return &Tmux{} }

func runTmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}

// Start spawns a detached tmux session running claude in cwd. No-op if it
// already exists.
func (t Tmux) Start(slug, cwd string) error {
	if t.Has(slug) {
		return nil
	}
	_, err := runTmux(buildStartArgs(slug, cwd, claudeBinary(), t.ClaudeArgs)...)
	return err
}

// Has reports whether a session for slug exists.
func (Tmux) Has(slug string) bool {
	cmd := exec.Command("tmux", buildHasArgs(slug)...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// Attach replaces the current process with `tmux attach`. The TUI must
// suspend itself before calling this; on detach the parent shell resumes.
func (Tmux) Attach(slug string) error {
	args := append([]string{"tmux"}, buildAttachArgs(slug)...)
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(bin, args, nil)
}

// Kill removes a session.
func (Tmux) Kill(slug string) error {
	_, err := runTmux(buildKillArgs(slug)...)
	return err
}

// List returns the slugs of currently running ssf-* sessions.
func (Tmux) List() ([]string, error) {
	out, err := runTmux(buildListArgs()...)
	if err != nil {
		// "no server running" is not an error: empty list.
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "error connecting") {
			return nil, nil
		}
		return nil, err
	}
	var slugs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, SessionPrefix) {
			slugs = append(slugs, strings.TrimPrefix(line, SessionPrefix))
		}
	}
	return slugs, nil
}
