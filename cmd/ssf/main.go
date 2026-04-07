// Command ssf orchestrates a set of Claude Code instances across worktrees.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/hooks"
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

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ssf:", err)
		os.Exit(1)
	}
}

const usage = `ssf — superset, fixed: orchestrate Claude Code instances across worktrees.

Usage:
  ssf [<dir>]              Register <dir> (or cwd) and open the TUI.
  ssf hook write <slug> <state>
                           Internal: invoked by Claude Code hooks.
  ssf -h | --help          Show this message.
`

func run(args []string, stdout, stderr *os.File) error {
	if len(args) >= 1 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Fprint(stdout, usage)
			return nil
		case "hook":
			return runHook(args[1:], stderr)
		}
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	reg, err := registry.New(cfgPath)
	if err != nil {
		return err
	}

	var target string
	if len(args) >= 1 {
		target = args[0]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target = cwd
	}
	// Validate the argument is an existing directory before touching the
	// registry. Otherwise typos and stray flags get registered as bogus
	// entries (e.g. "--help") that the user then has to clean up.
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", target, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("%q: %w", target, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q is not a directory", target)
	}
	target = abs
	if err := reg.Add(target); err != nil {
		return fmt.Errorf("register dir: %w", err)
	}
	if info, err := repoinfo.Inspect(target); err == nil {
		if err := hooks.Install(info.RepoRoot, slug.Slug(info.RepoRoot)); err != nil {
			slog.Warn("install hooks", "err", err)
		}
	}

	entries, err := buildEntries(reg)
	if err != nil {
		return err
	}

	if isTerminal(stdout) {
		return launchTUI(reg, entries)
	}
	// Non-TTY: print the rendered list.
	for _, e := range entries {
		fmt.Fprintln(stdout, e.Display)
	}
	return nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// buildEntries reads the registry, runs repoinfo on each dir, derives org
// prefixes, looks up the on-disk state, and returns sorted TUI entries.
func buildEntries(reg *registry.Registry) ([]tui.Entry, error) {
	dirs, err := reg.List()
	if err != nil {
		return nil, err
	}
	infos := make([]repoinfo.Info, len(dirs))
	orgs := make([]string, 0, len(dirs))
	for i, d := range dirs {
		info, _ := repoinfo.Inspect(d.Path)
		infos[i] = info
		if info.IsGitHub {
			orgs = append(orgs, info.Org)
		}
	}
	prefixes := orgprefix.Derive(orgs, nil)

	entries := make([]tui.Entry, 0, len(dirs))
	for i, d := range dirs {
		s := slug.Slug(infos[i].RepoRoot)
		st, _ := state.Read(infos[i].RepoRoot, s)
		entries = append(entries, tui.Entry{
			Display:  render.Line(d, infos[i], prefixes[infos[i].Org]),
			Path:     infos[i].RepoRoot,
			Slug:     s,
			Kind:     st.Kind,
			LastOpen: d.LastOpened,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastOpen.After(entries[j].LastOpen)
	})
	return entries, nil
}

func launchTUI(reg *registry.Registry, entries []tui.Entry) error {
	tmux := session.NewTmux()
	deps := tui.Deps{
		Session: tmux,
		Git:     realGit{},
		Opener:  realOpener{},
		AttachCmdFunc: func(s string) *exec.Cmd {
			return exec.Command("tmux", "-L", session.SocketName, "attach", "-t", session.SessionPrefix+s)
		},
		Unregister: func(path string) error {
			if err := reg.Remove(path); err != nil {
				return err
			}
			// Best-effort hook cleanup; failures are non-fatal.
			if info, err := repoinfo.Inspect(path); err == nil {
				_ = hooks.Uninstall(info.RepoRoot, slug.Slug(info.RepoRoot))
			}
			return nil
		},
	}
	cfg, _ := reg.Load()
	deps.FileManager = cfg.Settings.ResolveFileManager()
	deps.Editor = cfg.Settings.ResolveEditor()

	model := tui.NewModel(entries).WithDeps(deps)
	program := tea.NewProgram(model, tea.WithAltScreen())

	// Start a watcher pump goroutine that translates state events into
	// program.Send calls so the model re-renders live.
	repoRoots := make([]string, 0, len(entries))
	for _, e := range entries {
		repoRoots = append(repoRoots, e.Path)
		_ = os.MkdirAll(state.DirPath(e.Path), 0o755)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if ch, err := watcher.Watch(ctx, repoRoots); err == nil {
		notifier := notify.OsascriptNotifier{}
		dispatcher := notify.NewDispatcher(notifier, displayNameLookup(entries))
		go func() {
			for ev := range ch {
				dispatcher.Handle(ev)
				program.Send(tui.StateMsg{Slug: ev.Slug, Kind: ev.State.Kind})
			}
		}()
	}

	_, err := program.Run()
	return err
}

func displayNameLookup(entries []tui.Entry) func(string) string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Slug] = e.Display
	}
	return func(s string) string {
		if v, ok := m[s]; ok {
			return v
		}
		return s
	}
}

// realGit is a thin shell-out wrapper implementing tui.GitRunner.
type realGit struct{}

func (realGit) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (realGit) IsDirty(dir string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// realOpener spawns external GUI tools (file manager, editor) and returns
// without blocking.
type realOpener struct{}

func (realOpener) Open(binary, path string) error {
	cmd := exec.Command(binary, path)
	return cmd.Start()
}

func runHook(args []string, stderr *os.File) error {
	if len(args) < 3 || args[0] != "write" {
		return fmt.Errorf("usage: ssf hook write <slug> <state>")
	}
	hookSlug := args[1]
	kind := state.Kind(args[2])

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	info, err := repoinfo.Inspect(cwd)
	if err != nil {
		return err
	}
	st := state.State{
		Kind:    kind,
		Ts:      time.Now().UTC(),
		Session: "ssf-" + hookSlug,
	}
	return state.Write(info.RepoRoot, hookSlug, st)
}
