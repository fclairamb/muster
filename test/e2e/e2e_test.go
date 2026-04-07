//go:build e2e && tmux

// Package e2e holds end-to-end integration tests for ssf.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/ssf/internal/notify"
	"github.com/fclairamb/ssf/internal/orgprefix"
	"github.com/fclairamb/ssf/internal/registry"
	"github.com/fclairamb/ssf/internal/render"
	"github.com/fclairamb/ssf/internal/repoinfo"
	"github.com/fclairamb/ssf/internal/session"
	"github.com/fclairamb/ssf/internal/slug"
	"github.com/fclairamb/ssf/internal/state"
	"github.com/fclairamb/ssf/internal/state/watcher"
	"github.com/fclairamb/ssf/internal/tui"
)

// repoRoot returns the absolute path to the project root by walking up from
// this test file until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ssf")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ssf")
	cmd.Dir = repoRoot(t)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
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

func gitInit(t *testing.T, dir string) {
	t.Helper()
	steps := [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t.test", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init", "-q"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func TestFullPipeline(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux missing")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing")
	}

	// Shorten watcher timing for the test.
	old1, old2 := watcher.DebounceWindow, watcher.GreenConfirm
	watcher.DebounceWindow = 30 * time.Millisecond
	watcher.GreenConfirm = 50 * time.Millisecond
	t.Cleanup(func() { watcher.DebounceWindow, watcher.GreenConfirm = old1, old2 })

	bin := buildBinary(t)
	xdg := t.TempDir()
	t.Setenv("SSF_CLAUDE_BINARY", writeFakeClaude(t))

	// Build a fake git repo with an upstream remote.
	repo := t.TempDir()
	gitInit(t, repo)
	cmd := exec.Command("git", "remote", "add", "upstream", "git@github.com:stonal-tech/datalake.git")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v %s", err, out)
	}

	// Step 1: register the repo via the binary.
	cmd = exec.Command(bin, repo)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ssf register: %v %s", err, out)
	}
	if !strings.Contains(string(out), "s/datalake [") {
		t.Fatalf("expected abbreviated display, got %q", out)
	}

	// Step 2: load registry → entries.
	reg, err := registry.New(filepath.Join(xdg, "ssf", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	dirs, _ := reg.List()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	info, _ := repoinfo.Inspect(dirs[0].Path)
	prefixes := orgprefix.Derive([]string{info.Org}, nil)
	display := render.Line(dirs[0], info, prefixes[info.Org])

	entries := []tui.Entry{{
		Display:  display,
		Path:     info.RepoRoot,
		Slug:     slug.Slug(info.RepoRoot),
		Kind:     state.KindNone,
		LastOpen: dirs[0].LastOpened,
	}}

	// Step 3: spin up the watcher and dispatcher. Ensure .ssf/state exists
	// before watching so fsnotify can attach immediately.
	if err := os.MkdirAll(state.DirPath(info.RepoRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := watcher.Watch(ctx, []string{info.RepoRoot})
	if err != nil {
		t.Fatal(err)
	}
	rec := &notify.RecordingNotifier{}
	disp := notify.NewDispatcher(rec, func(s string) string { return display })

	// Prime the dispatcher with the current state, then write a Ready
	// state file via the binary's hook subcommand.
	disp.Handle(watcher.Event{Slug: entries[0].Slug, State: state.State{Kind: state.KindWorking}})

	cmd = exec.Command(bin, "hook", "write", entries[0].Slug, "ready")
	cmd.Dir = info.RepoRoot
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook write: %v %s", err, out)
	}

	// Wait for the watcher → dispatcher path.
	select {
	case ev := <-ch:
		if ev.State.Kind != state.KindReady {
			t.Fatalf("expected ready, got %q", ev.State.Kind)
		}
		disp.Handle(ev)
	case <-time.After(2 * time.Second):
		t.Fatal("watcher event timeout")
	}

	calls := rec.Snapshot()
	if len(calls) != 1 || calls[0].Body != "Ready" {
		t.Fatalf("expected one Ready notification, got %v", calls)
	}

	// Step 4: drive Enter via the Model against a real Tmux manager.
	sm := session.NewTmux()
	t.Cleanup(func() {
		_ = sm.Kill(entries[0].Slug)
		_ = exec.Command("tmux", "-L", session.SocketName, "kill-server").Run()
	})

	// We use FakeOpener to avoid spawning real GUI tools.
	deps := tui.Deps{
		Session: stubSessionAttach{Tmux: sm},
		Git:     &tui.FakeGit{},
		Opener:  &tui.FakeOpener{},
	}
	m := tui.NewModel(entries).WithDeps(deps)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tui.Model)
	_ = m
	time.Sleep(150 * time.Millisecond)
	if !sm.Has(entries[0].Slug) {
		t.Fatal("tmux session not started")
	}
}

// stubSessionAttach wraps session.Tmux but stubs Attach so the e2e test
// doesn't actually exec into tmux.
type stubSessionAttach struct {
	*session.Tmux
}

func (s stubSessionAttach) Attach(slug string) error { return nil }
