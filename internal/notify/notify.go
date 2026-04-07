// Package notify dispatches macOS notifications on session state transitions.
package notify

import (
	"os/exec"
	"strings"
	"sync"
)

// Notifier delivers a single notification to the user.
type Notifier interface {
	Notify(title, body string) error
}

// OsascriptNotifier shells out to /usr/bin/osascript to display a native
// macOS notification. Title and body are escaped against AppleScript string
// injection.
type OsascriptNotifier struct{}

// Notify implements Notifier.
func (OsascriptNotifier) Notify(title, body string) error {
	script := `display notification "` + escape(body) + `" with title "` + escape(title) + `"`
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
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
	Calls []Recording
}

// Recording is a single observed Notify call.
type Recording struct{ Title, Body string }

// Notify records the call and returns nil.
func (r *RecordingNotifier) Notify(title, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = append(r.Calls, Recording{Title: title, Body: body})
	return nil
}

// Snapshot returns a copy of the recorded calls.
func (r *RecordingNotifier) Snapshot() []Recording {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Recording, len(r.Calls))
	copy(out, r.Calls)
	return out
}
