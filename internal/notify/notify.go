// Package notify dispatches macOS notifications on session state transitions.
package notify

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Notification is the rich payload sent to the user.
type Notification struct {
	Title    string // bold first line
	Subtitle string // second line below title (e.g. the repo display name)
	Body     string // main message
	Sound    string // macOS sound name (Glass, Funk, Tink, ...) — empty = silent
	Group    string // dedupe key: a new notif with the same group replaces the previous one (terminal-notifier only)
	Activate string // bundle id of the app to focus when the user clicks the notification (terminal-notifier only)
}

// Notifier delivers a single notification to the user.
type Notifier interface {
	Notify(Notification) error
}

// NewBest returns the best available notifier. If terminal-notifier is on
// PATH it's used (better grouping, click-to-focus); otherwise falls back to
// the always-available osascript path.
func NewBest() Notifier {
	if _, err := exec.LookPath("terminal-notifier"); err == nil {
		return TerminalNotifier{}
	}
	return OsascriptNotifier{}
}

// OsascriptNotifier shells out to /usr/bin/osascript. Supports title,
// subtitle, body, and sound — but not grouping or click activation.
type OsascriptNotifier struct{}

// Notify implements Notifier.
func (OsascriptNotifier) Notify(n Notification) error {
	var b strings.Builder
	b.WriteString(`display notification "`)
	b.WriteString(escape(n.Body))
	b.WriteString(`" with title "`)
	b.WriteString(escape(n.Title))
	b.WriteString(`"`)
	if n.Subtitle != "" {
		b.WriteString(` subtitle "`)
		b.WriteString(escape(n.Subtitle))
		b.WriteString(`"`)
	}
	if n.Sound != "" {
		b.WriteString(` sound name "`)
		b.WriteString(escape(n.Sound))
		b.WriteString(`"`)
	}
	cmd := exec.Command("osascript", "-e", b.String())
	return cmd.Run()
}

// TerminalNotifier shells out to terminal-notifier. Supports everything
// OsascriptNotifier does, plus group dedupe and click-to-activate.
//
// Install: `brew install terminal-notifier`.
type TerminalNotifier struct{}

// Notify implements Notifier.
func (TerminalNotifier) Notify(n Notification) error {
	args := []string{
		"-title", n.Title,
		"-message", n.Body,
	}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", n.Subtitle)
	}
	if n.Sound != "" {
		args = append(args, "-sound", n.Sound)
	}
	if n.Group != "" {
		args = append(args, "-group", n.Group)
	}
	activate := n.Activate
	if activate == "" {
		activate = detectTerminalBundleID()
	}
	if activate != "" {
		args = append(args, "-activate", activate)
	}
	cmd := exec.Command("terminal-notifier", args...)
	return cmd.Run()
}

// detectTerminalBundleID maps the user's $TERM_PROGRAM env var to the macOS
// bundle id of their terminal so click-to-activate brings the right window
// to the front. Returns "" if the terminal is unknown.
func detectTerminalBundleID() string {
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		return "com.googlecode.iterm2"
	case "Apple_Terminal":
		return "com.apple.Terminal"
	case "ghostty":
		return "com.mitchellh.ghostty"
	case "WezTerm":
		return "com.github.wez.wezterm"
	case "vscode":
		return "com.microsoft.VSCode"
	case "tmux":
		return ""
	}
	return ""
}

// escape escapes characters that would close or break an AppleScript string literal.
func escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
	)
	return r.Replace(s)
}

// RecordingNotifier captures Notify calls for tests.
type RecordingNotifier struct {
	mu    sync.Mutex
	Calls []Notification
}

// Notify records the call and returns nil.
func (r *RecordingNotifier) Notify(n Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = append(r.Calls, n)
	return nil
}

// Snapshot returns a copy of the recorded notifications.
func (r *RecordingNotifier) Snapshot() []Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Notification, len(r.Calls))
	copy(out, r.Calls)
	return out
}
