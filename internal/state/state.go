// Package state defines the on-disk state file format and read/write helpers.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Kind enumerates the possible session states.
type Kind string

const (
	KindNone         Kind = "none"          // no instance
	KindIdle         Kind = "idle"          // attached but no activity
	KindWorking      Kind = "working"       // mid-prompt
	KindReady        Kind = "ready"         // green: result ready
	KindWaitingInput Kind = "waiting_input" // red: needs user
)

// State is the JSON shape written by hooks and consumed by the watcher.
type State struct {
	Kind    Kind      `json:"kind"`
	Ts      time.Time `json:"ts"`
	Session string    `json:"session"`
}

// FilePath returns the absolute path to the state file for slug under repoRoot.
func FilePath(repoRoot, slug string) string {
	return filepath.Join(repoRoot, ".muster", "state", slug+".json")
}

// DirPath returns the absolute path to the .muster/state directory under repoRoot.
func DirPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".muster", "state")
}

// Read returns the State for slug under repoRoot. Missing or corrupt files
// return KindNone with a nil error so callers can render gracefully.
func Read(repoRoot, slug string) (State, error) {
	b, err := os.ReadFile(FilePath(repoRoot, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return State{Kind: KindNone}, nil
		}
		return State{Kind: KindNone}, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		// Corrupt file: degrade gracefully.
		return State{Kind: KindNone}, nil
	}
	if s.Kind == "" {
		s.Kind = KindNone
	}
	return s, nil
}

// Write atomically writes s as the state file for slug under repoRoot.
func Write(repoRoot, slug string, s State) error {
	dir := DirPath(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	if s.Ts.IsZero() {
		s.Ts = time.Now().UTC()
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+slug+".*.tmp")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, FilePath(repoRoot, slug)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
