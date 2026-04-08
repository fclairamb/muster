# 14 — Right-pane file list

## Goal
When ssf attaches a user to a claude session, show a live list of modified
files in a right tmux split pane next to claude. Glance value: at any moment
the user can see which files claude has touched without leaving the session.

The pane is **opt-out** via settings, **suppressed on narrow terminals**, and
**self-contained** (no `lazygit`, no `watch`, no third-party tools).

## Behavior

- On `Tmux.Start(slug, cwd)`:
  1. `tmux new-session -d -s ssf-<slug> -c <cwd> claude ...` (existing).
  2. **If** the side panel is enabled AND the terminal width is ≥ 100 columns:
     `tmux split-window -h -p 30 -t ssf-<slug>:0 -c <cwd> "ssf files <cwd>"`.
  3. `tmux select-pane -t ssf-<slug>:0.0` so claude has focus on attach.
- The right pane runs `ssf files <cwd>` which prints a live file list.
- A tmux key binding (`prefix + f`) toggles the right pane on the running
  session: kills it if present, re-splits if absent. Bound at session start
  via `tmux set-option -t ssf-<slug> ... bind-key`.
- The split is per-session: the `ssf` list-view TUI is unaffected.

## Settings

```toml
[settings]
side_panel = true   # default; set to false to disable everywhere
```

`Settings.SidePanel *bool` — nil → default true. Same nilable pattern as
`ClaudeArgs` so an explicit `false` is honored.

## Width gating

Detect terminal width at session start time via `term.GetSize` (the user's
own terminal, before tmux suspends ssf for the attach). If width < 100,
skip the split. The user can still toggle it on with `prefix + f` later.

This avoids forcing people on a 13" laptop to disable the setting globally.

## `ssf files <dir>` subcommand

Hidden subcommand. Runs forever, prints a live file list, never returns.

### Refresh strategy

**fsnotify on the working tree, debounced 250ms, with a 5-second safety
poll.** fsnotify catches edits the instant they happen; the polling tick is
purely a safety net for cases where fsnotify silently misses events
(network mounts, weird containers, kqueue limits).

### Path scoping

Run `git -C <dir> status -s --porcelain -- .` so the listing is scoped to
the registered path, not the whole repo. Critical for:

- **Worktrees**: each worktree should show its own changes, not the union.
- **Subdirs**: `ssf files /repo/apps/api` should show only `apps/api`'s
  changes, not the whole repo.

### Skip list

The fsnotify watcher should NOT recurse into:

- `.git/`
- `node_modules/`
- `vendor/`
- `target/` (Rust)
- `dist/`, `build/`
- Anything in the repo's `.gitignore` (best-effort; don't reimplement git)

The polling tick uses `git status` directly so the gitignore is honored
naturally for the actual file listing.

### Output format

```
s/datalake apps/api [main ↑2 ↓0]

modified
  M src/handler.go      +12 -4
  M src/handler_test.go +18 -2
staged
  A docs/api.md
untracked
  ? scratch.txt
```

- Header: display name + branch + ahead/behind from `git status -b`.
- Three sections: modified (M, red), staged (A/M, green), untracked (?, yellow).
- Diff line counts via `git diff --numstat` and `git diff --cached --numstat`.
- Truncate paths from the left when they exceed pane width.
- No boxes, no borders, no spinners. Boring is the goal.

## Tests

- `cmd/ssf/files_test.go`:
  - Build the binary, init a git repo, register, run `ssf files <dir>` in a
    pipe with a context cancellation after 500ms. Assert the output contains
    expected sections after touching a file.
  - Skip if `git` missing.
- `internal/session/tmux_test.go` (under `tmux` build tag):
  - Test that `Start` with `SidePanel=true` creates a session with two panes.
  - Test that `SidePanel=false` creates a session with one pane.
- `internal/config/config_test.go`:
  - `ResolveSidePanel` defaults to true; explicit false is honored.

## Acceptance

```sh
make test
make test-tmux
./bin/ssf <some-repo>     # in a >= 100 col terminal: claude on left, files on right
./bin/ssf <some-repo>     # in a < 100 col terminal: claude only, no split
```

The `ssf files` subcommand should exit on Ctrl+C and not leak goroutines.

## Notes

- Use `golang.org/x/term.GetSize(int(os.Stdout.Fd()))` for the width check.
- The `prefix + f` toggle is implemented via a small inline shell command
  that checks pane count and either splits or kills. Not pretty but
  self-contained.
- `ssf files` is hidden from the top-level help, like `ssf hook write`.
  It's an implementation detail, not a user-facing verb.
- Don't try to render a header that requires knowing the entry's display
  name from the registry — `ssf files` runs as a child of tmux without
  config context. Just use the repo basename + branch from git, e.g.
  `apps/api [main ↑2 ↓0]`.

## Implementation Plan

(filled in by /implement-todos)
