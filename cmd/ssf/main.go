// Command ssf orchestrates a set of Claude Code instances across worktrees.
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
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "ssf:", err)
		os.Exit(1)
	}
}

func newApp() *cli.Command {
	return &cli.Command{
		Name:                  "ssf",
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
			hookCommand(),
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print the version",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Fprintln(os.Stdout, "ssf version", version)
			return nil
		},
	}
}

// rootAction is the default Action.
//
//   - `ssf`            → open the TUI, do not touch the registry.
//   - `ssf <dir>`      → validate, register/touch <dir>, then open the TUI.
//
// Bare `ssf` deliberately does NOT register cwd — otherwise every launch
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
		if info, err := repoinfo.Inspect(abs); err == nil {
			if err := hooks.Install(info.RepoRoot, slug.Slug(info.RepoRoot)); err != nil {
				slog.Warn("install hooks", "err", err)
			}
		}
	}

	entries, err := buildEntries(reg)
	if err != nil {
		return err
	}

	if isTerminal(os.Stdout) {
		return launchTUI(ctx, reg, entries)
	}
	for _, e := range entries {
		fmt.Fprintln(os.Stdout, e.Display)
	}
	return nil
}

// hookCommand returns the hidden hook subcommand tree.
//
// IMPORTANT: the literal command string "ssf hook write <slug> <kind>" is
// hard-coded into every .claude/settings.json file ssf installs (see
// internal/hooks/hooks.go). Renaming this subcommand or its arguments will
// break every existing installation. Lock the name.
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
		return fmt.Errorf("usage: ssf hook write <slug> <state>")
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
		Session: "ssf-" + hookSlug,
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

func launchTUI(parent context.Context, reg *registry.Registry, entries []tui.Entry) error {
	tmux := session.NewTmux()
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
	}
	cfg, _ := reg.Load()
	deps.FileManager = cfg.Settings.ResolveFileManager()
	deps.Editor = cfg.Settings.ResolveEditor()

	model := tui.NewModel(entries).WithDeps(deps)
	program := tea.NewProgram(model, tea.WithAltScreen())

	repoRoots := make([]string, 0, len(entries))
	for _, e := range entries {
		repoRoots = append(repoRoots, e.Path)
		_ = os.MkdirAll(state.DirPath(e.Path), 0o755)
	}
	ctx, cancel := context.WithCancel(parent)
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
