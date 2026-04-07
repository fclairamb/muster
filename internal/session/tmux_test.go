//go:build tmux

package session

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func randSlug(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "test" + hex.EncodeToString(b[:])
}

func writeFakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTmuxLifecycle(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux missing")
	}
	t.Setenv("SSF_CLAUDE_BINARY", writeFakeClaude(t))

	m := NewTmux()
	slug := randSlug(t)

	t.Cleanup(func() {
		_ = m.Kill(slug)
	})

	if err := m.Start(slug, t.TempDir()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Give tmux a moment to register the session.
	time.Sleep(100 * time.Millisecond)
	if !m.Has(slug) {
		t.Fatal("Has should be true after Start")
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range list {
		if s == slug {
			found = true
		}
	}
	if !found {
		t.Fatalf("slug %q not in list %v", slug, list)
	}
	if err := m.Kill(slug); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if m.Has(slug) {
		t.Fatal("Has should be false after Kill")
	}
}
