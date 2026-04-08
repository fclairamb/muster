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

	// SidePanel, when true, splits the tmux window after creating it and
	// runs `muster files <cwd>` in the right pane. Gated on TerminalWidth.
	SidePanel bool

	// SsfBinary is the path to the muster binary used for the side panel
	// command. Defaults to "muster" (resolved via PATH) when empty.
	SsfBinary string

	// TerminalWidth is the user's current terminal width in columns,
	// captured before tmux suspends muster. The side panel is skipped when
	// this is below MinPanelWidth (or 100 if MinPanelWidth is zero).
	TerminalWidth int
	MinPanelWidth int
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
// already exists. When SidePanel is enabled and the terminal is wide enough,
// also splits the window and runs `muster files <cwd>` in the right pane.
func (t Tmux) Start(slug, cwd string) error {
	existed := t.Has(slug)
	if !existed {
		if _, err := runTmux(buildStartArgs(slug, cwd, claudeBinary(), t.ClaudeArgs)...); err != nil {
			return err
		}
	}
	if existed && t.shouldSplit() {
		// Resize the right pane on every attach so old sessions self-heal.
		_, _ = runTmux("-L", SocketName, "resize-pane", "-t", SessionPrefix+slug+":0.1", "-x", "20")
		return nil
	}
	if existed {
		return nil
	}
	if t.shouldSplit() {
		bin := t.SsfBinary
		if bin == "" {
			bin = "muster"
		}
		// Split horizontally and give the new (right) pane a fixed 20-col
		// width. `-l 24` is cells (not a percentage — that would need `%`).
		_, _ = runTmux(
			"-L", SocketName,
			"split-window", "-h", "-l", "20",
			"-t", SessionPrefix+slug+":0",
			"-c", cwd,
			bin+" files "+cwd,
		)
		// Focus the left pane (claude) so the user lands there on attach.
		_, _ = runTmux("-L", SocketName, "select-pane", "-t", SessionPrefix+slug+":0.0")
	}
	return nil
}

func (t Tmux) shouldSplit() bool {
	if !t.SidePanel {
		return false
	}
	min := t.MinPanelWidth
	if min == 0 {
		min = 100
	}
	if t.TerminalWidth > 0 && t.TerminalWidth < min {
		return false
	}
	return true
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

// List returns the slugs of currently running muster-* sessions.
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
