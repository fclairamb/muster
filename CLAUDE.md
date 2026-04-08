# muster — context for future Claude sessions

See `README.md` for the user-facing description. This file is the
internal reference for working on the code.

## What this project is

A Go CLI + Bubble Tea TUI that orchestrates Claude Code instances across
registered directories via tmux sessions. Status indicators (red/green/
yellow/white) are driven by Claude Code hooks writing to on-disk state
files which muster polls and reconciles with the live tmux session set.

## Package layout

```
cmd/muster/              CLI entry point (urfave/cli/v3). Subcommands: list, rm,
                       version, completion, files (hidden), hook write (hidden).
                       Real implementations of GitRunner, Opener, Tmux are
                       wired here; nothing else knows about urfave/cli.

internal/config/      TOML config load/save + Settings with nilable override
                       pattern (ClaudeArgs, SidePanel are *T for default-vs-explicit-empty).
internal/registry/    Thin CRUD over config.Dirs: Add (idempotent touch),
                       Remove, Touch, List.
internal/slug/        sha256-based 12-char stable path slug. Never leaks the
                       org abbreviation — it's a stable internal identifier.
internal/orgprefix/   Shortest-unique-prefix abbreviation with manual overrides.
                       Display-only. Never used as an identifier.
internal/repoinfo/    `git -C <dir> rev-parse --show-toplevel` + remote URL
                       parser (SSH + HTTPS GitHub). Prefers upstream over origin.
internal/render/      Pure function building display strings from (Dir, Info,
                       prefix). Used by both the list subcommand and the TUI.
internal/state/       On-disk state file format and atomic read/write.
internal/state/watcher/ fsnotify-based watcher with debounce + green-confirm
                       heuristic. Dispatches to tui.StateMsg via program.Send.
internal/hooks/       Claude Code hook installer. Merges into the gitignored
                       .claude/settings.local.json (NEVER the committed
                       settings.json), preserving unrelated keys. Auto-scrubs
                       any stale slug entries from settings.json on (re)install
                       and ensures .gitignore covers the new file. Supports
                       the optional matcher field for PreToolUse/PostToolUse
                       on AskUserQuestion.
internal/session/     tmux Manager interface + Tmux impl (dedicated -L muster socket)
                       + FakeManager. Handles the split-window for the side panel.
internal/notify/      Notifier interface + OsascriptNotifier + TerminalNotifier
                       + Dispatcher (transitions → notifications with sound,
                       subtitle, group, click-to-activate).
internal/tui/         Bubble Tea Model + Deps injection + StatusEmoji +
                       sort/filter/hierarchy. All side effects go through Deps.
internal/files/       `muster files` rendering engine: git status parser, fsnotify
                       watcher with skip list, ANSI-colored output.
test/e2e/             Full-stack integration test gated behind e2e+tmux tags.
```

## Key design decisions (read before making changes)

### Slugs are stable identifiers, abbreviations are display-only

Every registered path has two representations:

- **Slug** (`sha256(absPath)[:12]`, lowercase hex). Stable across runs.
  Used for tmux session names (`muster-<slug>`), state file names
  (`.muster/state/<slug>.json`), and hook commands. **Never changes.**
- **Display** (`<prefix>/<repo> [<branch>]` or similar). Derived from
  `orgprefix.Derive()` which can rename abbreviations when new orgs are
  registered. **Never appears in any persistent identifier.**

Violating this rule silently orphans running sessions and state files on
the first collision-driven rename. Enforced by the slice 02 spec.

### `muster hook write <slug> <kind>` is a public contract

Every `.claude/settings.local.json` muster installs hard-codes this literal
command string (older installs landed in `.claude/settings.json`; the
installer scrubs them automatically on the next muster run). Renaming the subcommand or reordering arguments breaks every
existing installation. There's a warning comment in
`internal/hooks/hooks.go` next to `command()` and in
`cmd/muster/main.go`'s `hookCommand()`. Don't rename, don't reorder. If you
add new hook events, append — don't reshuffle existing ones.

### Side-effect injection via Deps

`tui.Model` takes a `Deps` struct containing `Session`, `Git`, `Opener`,
`AttachCmdFunc`, `Unregister`, `ReadState`, `ClearState`. Tests pass
fakes (`session.FakeManager`, `tui.FakeGit`, `tui.FakeOpener`,
`notify.RecordingNotifier`). Real implementations live in `cmd/muster/main.go`
and are never imported from other packages.

Consequence: the model is trivially unit-testable without spinning up a
real terminal. Tests drive Update with `tea.KeyMsg` values directly and
assert on the resulting model + its recorded fake calls.

### State reconciliation rules

`tui.Model.applyRefresh` runs every second (via a goroutine in `launchTUI`
calling `program.Send(tui.RefreshMsg{})`). For each entry it:

1. Calls `deps.ReadState(repoRoot, slug)` to get the on-disk State.
2. Batches `deps.Session.List()` once per tick and builds a slug set for
   membership checks.
3. Applies the reconciliation rules:
   - session present + state file says X → X (the on-disk state is trusted
     as-is while the session is alive — earlier versions decayed
     `working`/`waiting_input` after 30s, which incorrectly flipped long
     turns to white; the user can press `x` if a hook ever wedges)
   - session present + no state file → **idle** (we have a session, claude
     hasn't fired any hook yet, default to idle)
   - session absent → **none**, regardless of any stale state file

When the user detaches from a session via `tea.ExecProcess`, the callback
emits `attachExitedMsg{slug}`. If that entry was sitting on `KindReady`,
the handler calls `deps.ClearState` to overwrite it with `idle` — so
"green" is a nag, not a sticky state.

### Bare `muster` does NOT register cwd

Only `muster <dir>` (including `muster .`) registers. Bare `muster` just opens
the TUI. Otherwise every launch from a working directory re-adds it, and
entries the user removed with `r` reappear on the next run.

### Path validation before registration

`rootAction` stat-checks the target path before calling `reg.Add`. Non-
existent or non-directory args are rejected with a helpful error. Without
this, typos like `muster --help` (before we had a real `--help` flag) got
registered as phantom directories.

### Subdirs are first-class entries

`muster ~/code/datalake/apps/api` keeps `apps/api` as the registered path,
slug, and display. Hooks install at the **repo root** (so Claude finds
them) but are **keyed by the subdir's slug**, so each subdir has its own
session and state file. `Entry.RepoRoot` carries the git toplevel for
state file addressing; `Entry.Path` is the registered subdir.

### Side panel is opt-out with width gating + hook-locked width

`Tmux.Start` splits the window horizontally and runs `muster files <cwd>` in
the right pane when `SidePanel` is true AND terminal width ≥ 100 columns.
Both settings are in `internal/session/tmux.go`; terminal width is
captured in `cmd/muster/main.go` before tmux suspends muster.

The right pane is locked to `session.SidePanelWidth` (20 cols) via tmux
session hooks installed by `installSidePanelHooks`:

- `window-resized` → `resize-pane -x 20`
- `client-attached` → `resize-pane -x 20`
- `client-resized` → `resize-pane -x 20`

Without these hooks, tmux scales pane sizes proportionally when the window
grows (e.g. when an attaching client has a wider terminal than the
detached default of 80×24). A pane created with `split-window -l 20`
balloons to 80+ cols on first attach. The hooks self-heal on every
relevant event, and `Start` also calls them on existing sessions on
re-attach so older installs without the hooks get fixed automatically.

### Shell sessions live under a `-sh` slug suffix

`actionShell` (bound to `s`) starts a parallel tmux session running
`$SHELL` for the selected entry. The session name is
`muster-<slug>-sh`, distinct from the claude session's `muster-<slug>`,
so it never collides with — or appears in — the entry's status
reconciliation. The shell session has no side panel; it's a plain
interactive shell. Killing or restarting one does not affect the other.

### Branch picker (`b`) and in-flight op indicator

`actionBranchPicker` opens a filterable list of local branches loaded
asynchronously via `git branch --format=%(refname:short)`. Enter checks
out the highlighted match (or `git checkout -b <name>` when nothing
matches). On success the row's `Display` is updated in place via
`setDisplayBranch` so the new branch is visible immediately without
restarting muster.

All long-running git operations (pull/push/merge-main, worktree
add/remove, branch list, checkout) register an in-flight entry on the
Model via `beginOp(label)` and clear it via `endOp(id)`. The currently
active ops render as a `⏳ <label1>, <label2>` line above the footer.
The wrapping is generic — `wrapOp(id, cmd)` envelopes any `tea.Cmd`'s
result in `opMsg{id, msg}`, which `Update` unwraps before re-dispatching
the inner message. `statsCmd` is intentionally NOT wrapped: it fires
once per refresh tick per entry and would just spam the indicator.

### Worktrees added in-TUI nest under their parent

Pressing `n` and naming a branch runs `git worktree add` and then —
on success — inserts a fresh `Entry{Indent: 1, IsWorktree: true,
Display: "[<branch>]"}` directly after the parent in `m.entries` via
`insertAfterSlug`. `SortEntries` keeps `Indent==0` parents grouped with
their `Indent>0` children, so the new worktree appears nested without
restarting muster. The worktree path is the single source of truth in
`WorktreePathFor(repo, branch)`, used by both the git arg builder and
the post-create entry insertion.

## Testing philosophy

- **All side effects behind interfaces, tests use fakes.**
- **TUI tested via direct `Update(tea.KeyMsg)` calls**, not `teatest`.
  Faster and more reliable.
- **tmux integration tests use a fake `claude` shell script** (`sleep 30`)
  via `$MUSTER_CLAUDE_BINARY`. Tests spawn real tmux sessions on the `-L muster`
  socket, verify via `Has()`, and kill in `t.Cleanup`.
- **Build tags**:
  - default: pure unit tests (CI runs these)
  - `tmux`: integration tests that need real tmux
  - `e2e`: full-stack integration in `test/e2e/`
  - `manual`: smoke tests that need a human to observe the result
- **The only unavoidable human gate** is verifying real Claude Code hooks
  fire as expected. Documented in `specs/done/2026/04/05-hooks.md`.

## Architectural rules of thumb

- `cmd/muster` is the only package allowed to know about `urfave/cli`. All
  business logic lives in `internal/*`.
- The `tui` package **does not import `watcher` or `session.Tmux` concrete
  types**. It talks to interfaces in `Deps` only.
- The `hooks` package **is the public contract layer** for Claude Code
  integration. Be careful. See "muster hook write is a public contract".
- State files live at the **repo root**, not the subdir. `Entry.RepoRoot`
  is what `state.Read/Write` takes.
- **Never poll via `watch` or depend on external tools**. muster is
  self-contained on purpose. The `files` package does fsnotify + git status.

## What's still rough

- **No per-session side-panel toggle.** The spec mentions `prefix + f`
  but it's not implemented. Users get the split at session start and
  can close the right pane manually with `tmux kill-pane`.
- **fsnotify on macOS uses kqueue** which doesn't recurse. New subdirs
  created after `muster files` launches aren't watched until the 5s safety
  poll catches them.
- **Multi-line-wide east-asian chars in paths** can misalign the list.
  Only handled at the emoji width level, not for arbitrary path content.
- **Claude Code hook event names are not stable across versions.** If
  Claude renames `PreToolUse` or removes the matcher field, hooks break.
- **Display abbreviations are recomputed on every launch**, not persisted.
  That's fine because they're deterministic, but it means the rendered
  display of an entry can change when a new org is registered.

## History

Implementation specs live in `specs/done/YYYY/MM/`. Each one has a
timestamped implementation plan appended to the bottom. Read them in
order (00 → 14) to understand how the project evolved. Specs are frozen
once archived — don't edit them, write a new one.

## When adding a new spec

Follow the shape of `specs/done/2026/04/*.md`:
- Goal (one paragraph)
- Deliverables (code + tests)
- Acceptance criteria (the exact `make test` invocation that proves it works)
- Notes / gotchas
- Implementation Plan section left blank, filled in when implementation starts

Run `/loop /implement-todos ultrathink` to have the specs picked up
automatically in order.
