# muster

Orchestrate a set of [Claude Code](https://docs.claude.com/en/docs/claude-code)
instances across directories and worktrees. A lightweight alternative to
[superset.sh](https://superset.sh) and [claude-squad](https://github.com/smtg-ai/claude-squad),
sitting between the two — featureful without eating all your power.

## What it does

- **One binary, one tmux socket**: every claude session runs in a dedicated
  tmux session on the `-L muster` socket. Nothing touches your default tmux.
- **Live status indicators** (🔴 waiting for input, 🟢 ready, 🟡 working,
  ⚪ idle) driven by Claude Code hooks. Installed automatically when you
  register a directory.
- **macOS notifications** on state transitions (`Needs input` / `Ready`),
  with grouping and click-to-focus when `terminal-notifier` is installed.
- **Right-pane file list** showing modified / staged / untracked files
  live next to claude, refreshed via fsnotify.
- **Subdirs and worktrees as first-class entries** — register a subdir
  and it stays as-is instead of collapsing to the repo root.
- **Terminal-title updates** so `muster: list` or `s/datalake [main] ⚪`
  shows in your window title.

## Install

```sh
git clone https://github.com/fclairamb/muster.git
cd muster
make install        # installs ~/.local/bin/muster + a `mst` symlink
```

`mst` is a three-letter alias for the same binary — `mst list`, `mst <dir>`,
and so on all work identically to `muster …`.

Or via `go install`:

```sh
go install github.com/fclairamb/muster/cmd/muster@latest
```

### Migrating from ssf

If you used the previous incarnation of this project (`ssf` /
`supersetfixed`), the first time you launch `muster` it auto-detects an
existing `~/.config/ssf/config.toml` and runs the migration silently.
You can also run it explicitly:

```sh
muster migrate
```

It copies the registry to `~/.config/muster/config.toml`, renames each
repo's `.ssf/state/` to `.muster/state/`, and rewrites every repo's
`.claude/settings.json` to use `muster hook write` instead of
`ssf hook write`. The old config is left in place as a recovery
breadcrumb. Idempotent — re-running is a no-op.

Requirements:

- Go 1.22+ (for building)
- tmux 3.2+ (for the session manager)
- Claude Code (`claude` on PATH, or set `$MUSTER_CLAUDE_BINARY`)
- macOS for the native notification sounds (linux falls back to silent)
- **Recommended**: `brew install terminal-notifier` for notification
  grouping and click-to-activate

## Quickstart

```sh
muster ~/code/stonal/datalake      # register + immediately attach to claude
muster                             # open the TUI list (does NOT re-register cwd)
muster list                        # print the registered entries
muster list --json                 # machine-readable
muster rm /abs/path/to/dir         # unregister by path
muster rm <12-char-slug>           # or by slug (from `muster list --json`)
muster completion zsh              # emit a completion script
```

In the TUI:

| Key       | Action                                                      |
|-----------|-------------------------------------------------------------|
| `↑` / `↓` | Move cursor                                                 |
| `Enter`   | Attach to (or start) the claude session for this entry     |
| `/`       | Filter entries by substring                                 |
| `n`       | Create a new git worktree from the selected repo            |
| `o`       | Open the directory in your file manager (`$FILE_MANAGER` or `open`) |
| `e`       | Open the directory in your editor (`$VISUAL`, `$EDITOR`, or `zed`) |
| `r`       | Remove entry (with confirmation; worktrees vs unregister differ) |
| `q`       | Quit                                                        |

Detach from an attached session with the standard tmux key: `Ctrl+b d`.
The TUI resumes exactly where you left it.

## Display abbreviation

GitHub-backed repos render with a shortest-unique-prefix org abbreviation:

```
s/datalake [main]                            stonal-tech/datalake
f/solidping [main]                           fclairamb/solidping
s/datalake apps/api [feat/x]                 stonal-tech/datalake, subdir
```

`upstream` remote takes precedence over `origin` so forks display under the
canonical project. Non-GitHub directories render as the basename. Overrides
can be set per-org in `~/.config/muster/config.toml`.

## Config

Lives at `$XDG_CONFIG_HOME/muster/config.toml` (default `~/.config/muster/config.toml`).

```toml
[[dirs]]
  path = "/Users/florent/code/stonal/datalake"
  last_opened = 2026-04-08T00:15:00Z

[settings]
  file_manager = ""          # defaults to $FILE_MANAGER → "open"
  editor = ""                # defaults to $VISUAL → $EDITOR → "zed"
  side_panel = true          # right-pane file list (disable with false)
  claude_args = ["--dangerously-skip-permissions"]  # default; use [] for no args

  [settings.org_overrides]
  microsoft = "ms"
  meta = "me"
```

## How it works

- **Registration** writes the absolute path into `config.toml` and installs
  a set of Claude Code hooks into `.claude/settings.json` at the repo root.
  The hooks are keyed by a sha256-based slug of the registered path so
  subdirs of the same repo get distinct sessions.
- **Session lifecycle** is driven by `tmux new-session -L muster -s muster-<slug>`.
  Each session runs `claude` (or `$MUSTER_CLAUDE_BINARY`) with the configured
  args. Optionally a right pane runs `muster files <dir>` for the live file list.
- **Status detection** happens via Claude Code hooks:
  - `SessionStart` → idle (⚪)
  - `UserPromptSubmit` → working (🟡)
  - `Stop` → ready (🟢)
  - `Notification` → waiting_input (🔴)
  - `PreToolUse[matcher=AskUserQuestion]` → waiting_input (🔴)
  - `PostToolUse[matcher=AskUserQuestion]` → working (🟡)
  Each hook invokes `muster hook write <slug> <kind>` which atomically writes
  the state to `<repo-root>/.muster/state/<slug>.json`.
- **Staleness decay**: if a `working` or `waiting_input` state is older
  than 30 seconds and no update arrived, it decays to `idle`. This handles
  the case where claude went quiet without firing a `Stop` hook.
- **Reconciliation on refresh** (every second): an entry's color is
  whatever the state file says, capped by `tmux list-sessions` membership.
  No session running → no bubble, regardless of stale files.

## Keyboard at a glance

```
muster              # open the TUI (does not register)
muster ~/code/foo   # register + immediately attach to claude
muster list         # print entries, no TUI
muster rm <path>    # unregister
muster version      # version info
muster --help       # full help
```

## Status

This is a personal project. The core is working end-to-end; it's what I use
daily. Contributions welcome, but expect churn.
