// Command muster orchestrates a set of Claude Code instances across worktrees.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v3"
	xterm "golang.org/x/term"

	"github.com/fclairamb/muster/internal/config"
	"github.com/fclairamb/muster/internal/gitignore"
	"github.com/fclairamb/muster/internal/gitstats"
	"github.com/fclairamb/muster/internal/hooks"
	"github.com/fclairamb/muster/internal/notify"
	"github.com/fclairamb/muster/internal/orgprefix"
	"github.com/fclairamb/muster/internal/registry"
	"github.com/fclairamb/muster/internal/render"
	"github.com/fclairamb/muster/internal/repoinfo"
	"github.com/fclairamb/muster/internal/session"
	"github.com/fclairamb/muster/internal/slug"
	"github.com/fclairamb/muster/internal/state"
	"github.com/fclairamb/muster/internal/state/watcher"
	"github.com/fclairamb/muster/internal/tui"
)

// version is overridden at build time via -ldflags="-X main.version=...".
// When ldflags are not set (e.g. `go build`, `go install`), init() falls back
// to runtime/debug.ReadBuildInfo() which embeds VCS revision + dirty flag
// automatically for any go-build from a git checkout.
var version = "dev"

func init() {
	if version != "dev" {
		return // ldflags wins
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	version = rev + dirty
}

func main() {
	autoMigrateIfNeeded()
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "muster:", err)
		os.Exit(1)
	}
}

func newApp() *cli.Command {
	return &cli.Command{
		Name:                  "muster",
		Usage:                 "orchestrate Claude Code instances across worktrees",
		ArgsUsage:             "[dir]",
		Version:               version,
		Action:                rootAction,
		EnableShellCompletion: true,
		ConfigureShellCompletionCommand: func(c *cli.Command) {
			// Un-hide the auto-built completion command so users discover it.
			c.Hidden = false
		},
		Commands: []*cli.Command{
			listCommand(),
			rmCommand(),
			versionCommand(),
			migrateCommand(),
			hookCommand(),
			filesCommand(),
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print the version",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Fprintln(os.Stdout, "muster version", version)
			return nil
		},
	}
}

// rootAction is the default Action.
//
//   - `muster`            → open the TUI, do not touch the registry.
//   - `muster <dir>`      → validate, register/touch <dir>, then open the TUI.
//
// Bare `muster` deliberately does NOT register cwd — otherwise every launch
// from a working directory re-adds it, and entries the user removed via
// `r` reappear on the next run.
func rootAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() > 1 {
		return fmt.Errorf("expected at most one directory argument")
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	reg, err := registry.New(cfgPath)
	if err != nil {
		return err
	}

	var autoAttachSlug string
	if target := cmd.Args().First(); target != "" {
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
		if err := reg.Add(abs); err != nil {
			return fmt.Errorf("register dir: %w", err)
		}
		// Install hooks at the repo root (where claude reads
		// .claude/settings.json) but key them by the registered path's
		// slug. This makes a registered subdir a distinct entry from its
		// parent repo.
		if info, err := repoinfo.Inspect(abs); err == nil {
			repoRoot := info.RepoRoot
			if repoRoot == "" {
				repoRoot = abs
			}
			if err := hooks.Install(repoRoot, slug.Slug(abs)); err != nil {
				slog.Warn("install hooks", "err", err)
			}
			if err := gitignore.EnsureMusterIgnored(repoRoot); err != nil {
				slog.Warn("update gitignore", "err", err)
			}
		}
		autoAttachSlug = slug.Slug(abs)
	}

	entries, err := buildEntries(reg)
	if err != nil {
		return err
	}

	if isTerminal(os.Stdout) {
		return launchTUI(ctx, reg, entries, autoAttachSlug)
	}
	for _, e := range entries {
		fmt.Fprintln(os.Stdout, e.Display)
	}
	return nil
}

// hookCommand returns the hidden hook subcommand tree.
//
// IMPORTANT: the literal command string "muster hook write <slug> <kind>" is
// hard-coded into every .claude/settings.json file muster installs (see
// internal/hooks/hooks.go). Renaming this subcommand or its arguments will
// break every existing installation. Lock the name.
//
// HISTORY: this was previously "ssf hook write …". Slice 15 renamed the
// project from ssf to muster; the rename is the one and only exception.
// `muster migrate` rewrites legacy entries via UninstallLegacy + Install.
func hookCommand() *cli.Command {
	return &cli.Command{
		Name:   "hook",
		Hidden: true,
		Usage:  "internal: invoked by Claude Code hooks",
		Commands: []*cli.Command{
			{
				Name:      "write",
				ArgsUsage: "<slug> <state>",
				Action:    runHookWrite,
			},
		},
	}
}

func runHookWrite(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 2 {
		return fmt.Errorf("usage: muster hook write <slug> <state>")
	}
	hookSlug := args.Get(0)
	kind := state.Kind(args.Get(1))

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
		Session: session.SessionPrefix + hookSlug,
	}
	return state.Write(info.RepoRoot, hookSlug, st)
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
		// Slug is derived from the *registered* path, not the repo root.
		// This makes subdirs distinct entries from their parent repo.
		s := slug.Slug(d.Path)
		repoRoot := infos[i].RepoRoot
		if repoRoot == "" {
			repoRoot = d.Path
		}
		st, _ := state.Read(repoRoot, s)
		entries = append(entries, tui.Entry{
			Display:  render.Line(d, infos[i], prefixes[infos[i].Org]),
			Path:     d.Path,
			RepoRoot: repoRoot,
			Slug:     s,
			Kind:     st.Kind,
			LastOpen: d.LastOpened,
			Stats:    gitstats.Compute(d.Path),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastOpen.After(entries[j].LastOpen)
	})
	return entries, nil
}

func launchTUI(parent context.Context, reg *registry.Registry, entries []tui.Entry, autoAttachSlug string) error {
	cfg, _ := reg.Load()

	tmux := session.NewTmux()
	tmux.ClaudeArgs = cfg.Settings.ResolveClaudeArgs()
	tmux.SidePanel = cfg.Settings.ResolveSidePanel()
	if w, _, err := termGetSize(); err == nil {
		tmux.TerminalWidth = w
	}
	if exe, err := os.Executable(); err == nil {
		tmux.SsfBinary = exe
	}

	deps := tui.Deps{
		Session: tmux,
		Git:     realGit{},
		Opener:  realOpener{},
		AttachCmdFunc: func(s string) *exec.Cmd {
			return exec.Command("tmux", "-L", session.SocketName, "attach", "-t", session.SessionPrefix+s)
		},
		Unregister: func(path string) error {
			return unregister(reg, path)
		},
		ReadState: func(repoRoot, slug string) state.State {
			st, _ := state.Read(repoRoot, slug)
			return st
		},
		GitStats: func(path string) gitstats.Stats {
			return gitstats.Compute(path)
		},
		ClearState: func(repoRoot, slug string) error {
			return state.Write(repoRoot, slug, state.State{
				Kind:    state.KindIdle,
				Ts:      time.Now().UTC(),
				Session: session.SessionPrefix + slug,
			})
		},
	}
	deps.FileManager = cfg.Settings.ResolveFileManager()
	deps.Editor = cfg.Settings.ResolveEditor()

	// Refresh once before launching so the initial frame reflects the live
	// tmux session set instead of any stale state file from a previous run.
	model := tui.NewModel(entries).WithDeps(deps).Refresh()
	if autoAttachSlug != "" {
		model = model.WithAutoAttach(autoAttachSlug)
	}
	program := tea.NewProgram(model, tea.WithAltScreen())

	rootSet := make(map[string]bool)
	for _, e := range entries {
		root := e.RepoRoot
		if root == "" {
			root = e.Path
		}
		rootSet[root] = true
		_ = os.MkdirAll(state.DirPath(root), 0o755)
	}
	repoRoots := make([]string, 0, len(rootSet))
	for r := range rootSet {
		repoRoots = append(repoRoots, r)
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if ch, err := watcher.Watch(ctx, repoRoots); err == nil {
		notifier := notify.NewBest()
		dispatcher := notify.NewDispatcher(notifier, displayNameLookup(entries))
		go func() {
			for ev := range ch {
				dispatcher.Handle(ev)
				program.Send(tui.StateMsg{Slug: ev.Slug, Kind: ev.State.Kind})
			}
		}()
	}

	// Background fetch loop: every 5 minutes (and immediately at startup),
	// run `git fetch` for each entry's path so the Behind count reflects the
	// real remote. After each fetch wave, push a RefreshMsg so the list
	// recomputes its stats.
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	// One-shot ignore-file pass for every touched repo root.
	for r := range rootSet {
		_ = gitignore.EnsureMusterIgnored(r)
	}
	go func() {
		fetchAll := func() {
			for _, p := range paths {
				_ = gitstats.Fetch(p)
			}
			program.Send(tui.RefreshMsg{})
		}
		fetchAll()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fetchAll()
			}
		}
	}()

	// Background refresh: every second, push a RefreshMsg into the program.
	// tea.Tick can get lost across tea.ExecProcess suspensions; program.Send
	// always reaches the model on resume.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				program.Send(tui.RefreshMsg{})
			}
		}
	}()

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

// termGetSize returns the controlling terminal width/height, falling back
// to stdin if stdout isn't a tty (e.g. in tests).
func termGetSize() (int, int, error) {
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stdin.Fd())} {
		if w, h, err := xterm.GetSize(fd); err == nil {
			return w, h, nil
		}
	}
	return 0, 0, fmt.Errorf("not a terminal")
}

// realOpener spawns external GUI tools (file manager, editor) and returns
// without blocking.
type realOpener struct{}

func (realOpener) Open(binary, path string) error {
	cmd := exec.Command(binary, path)
	return cmd.Start()
}
