# ssf — SuperSetFixed

Orchestrate a set of Claude Code instances. Roughly the same goal as
[superset.sh](https://superset.sh) and [claude-squad](https://github.com/smtg-ai/claude-squad),
but sitting between the two:

- Superset is great but eats all the machine's power.
- claude-squad is lightweight but has zero cool features.

`ssf` aims to be lightweight *and* feature-rich.

---

## Core concepts

- **Registered directory**: a repo, or a subdirectory of a repo, that the user
  cares about. Stored in `~/.config/ssf/config.toml`.
- **Worktree**: a `git worktree` created under `<repo>/.ssf/worktrees/<repo>-<branch-slug>`.
  `.ssf/` should be added to the user's global gitignore (or we add it to the
  repo's `.git/info/exclude` automatically on first use).
- **Session**: a `tmux` session running a single `claude` instance bound to a
  registered directory or worktree. Session name: `ssf-<slug>`. One per
  directory by default; users can open additional ones explicitly.
- **State file**: `<repo>/.ssf/state/<slug>.json`, written by Claude Code hooks
  (see "State detection" below). This is the source of truth for the colored
  status in the TUI.

Subdirectories of a repo are a *scoping hint* for Claude's CWD only — they do
**not** get their own worktrees. Worktrees always belong to the parent repo.

### Display names: abbreviated org prefix

In the TUI, GitHub-backed repos are rendered as `<org-prefix>/<repo>` where
`<org-prefix>` is the **shortest unique prefix** of the GitHub org name across
all currently registered repos:

- `stonal-tech/datalake` → `s/datalake`
- `fclairamb/solidping` → `f/solidping`
- `dir2 (s/repo1)` for a subdir of an abbreviated repo

Rules:

- **Display-only.** Tmux session names, state-file paths, and config keys
  always use a stable internal slug (full org + repo, or a hash of the
  absolute path). The abbreviation must never appear in any persistent
  identifier — otherwise a future collision-driven rename orphans running
  sessions and state files. This separation is non-negotiable.
- **Auto-derived, shortest unique prefix, minimum 1 char.** One known org →
  1 letter. Adding a colliding org expands both to the shortest
  disambiguating prefix (`stonal-tech` + `some-org` → `st` and `so`).
  Predictable, no config required, deterministic.
- **Manual override per org** in `config.toml` for cases where the auto choice
  is ugly or ambiguous (e.g. `microsoft` vs `meta` — user may prefer `ms`/`me`
  rather than `mi`/`me`).
- **Multiple remotes**: prefer `upstream` over `origin` when deriving the org,
  so forks display as the canonical project.
- **Non-GitHub local dirs**: no prefix, render the basename only (e.g.
  `~/scratch/notes` → `notes`). Don't invent a fake org.
- **No clever shortenings.** No vowel stripping, no `stnl/datalake`. Single
  letter with fallback expansion is the sweet spot; anything fancier is a
  memorability tax.

---

## CLI

```
ssf [<dir>]      # Register <dir> (or cwd) if new, then open the TUI.
ssf              # Same, with cwd.
```

No other subcommands in the MVP. Everything else happens in the TUI.

---

## TUI

### Home page

Shows the list of registered directories and their worktrees, hierarchically:

```
- repo1 [main] 🔴            // waiting for our input
- repo2 [main] 🟢            // output ready
- repo3 [main] 🟡            // working
- repo2 [main] ⚪️            // idle Claude instance attached
- dir2 (repo1)               // a subdir of repo1
  - [feat/new-stuff]         // a worktree of repo1
- repo1 [main]               // no Claude instance
```

### Status colors

| Color    | Meaning                                                  |
|----------|----------------------------------------------------------|
| 🔴 Red   | Claude is waiting for our input (permission or question) |
| 🟢 Green | Claude finished and the result is ready                  |
| 🟡 Yellow| Claude is currently working (spinner)                    |
| ⚪️ White | A Claude instance is attached but idle                   |
| (none)   | No Claude instance running                               |

### Sort order

1. Red
2. Green
3. Yellow
4. White
5. No instance — most recently opened first

### Keybindings

| Key     | Action                                                          |
|---------|-----------------------------------------------------------------|
| `↑/↓`   | Move selection                                                  |
| `Enter` | Attach to the Claude session (spawn it lazily if not running)   |
| `/`     | Search/filter directories by name                               |
| `n`     | Create a new worktree from the selected repo (prompts branch)   |
| `o`     | Open file navigator (`$FILE_MANAGER`, default `open`)           |
| `e`     | Open editor (`$VISUAL` → `$EDITOR` → fallback `zed`)            |
| `r`     | Remove the selected entry (with confirmation)                   |
| `q`     | Quit the TUI                                                    |

`Enter` behavior: if no Claude session exists yet for the entry, `ssf` starts
one lazily (`tmux new-session -d -s ssf-<slug> "claude"`) and then attaches.
Registering 10 directories does **not** spawn 10 Claudes upfront.

`r` confirmation must be explicit about what's being removed:
- For a registered dir: "Unregister <dir>? (sessions and worktrees are kept)"
- For a worktree: "Remove worktree <branch> AND kill its session?"
- Refuse to remove a worktree with uncommitted changes unless `--force` is
  passed in the confirmation prompt.

---

## State detection (the hard part)

This is the whole product. If status is unreliable, nothing else matters.

**Approach: Claude Code hooks write a state file.** On first run in a
directory, `ssf` injects the following hooks into the local
`.claude/settings.json` (merging, not overwriting):

- `UserPromptSubmit` → state = `working`
- `Stop` → state = `ready` (green)
- `Notification` → state = `waiting_input` (red)
- `SessionStart` → state = `idle` (white)

Each hook writes `<repo>/.ssf/state/<slug>.json`:

```json
{ "state": "working", "ts": "2026-04-08T12:34:56Z", "session": "ssf-repo1-main" }
```

The TUI watches this directory (fsnotify) and re-renders on change.

**Heuristic fallback for "waiting for input":** `Notification` fires for
permission prompts but not necessarily for a plain idle-waiting-on-user
state. If `Stop` fired and no `UserPromptSubmit` follows within N seconds, we
treat the session as `ready` (green), not `waiting_input`. This assumption
must be **validated on day 1 of the prototype** before the four-color model
is committed to.

Tmux pane scraping and JSONL transcript tailing were considered and rejected:
fragile and laggy respectively. Hooks are the API.

---

## Process model

- **tmux** for session management. Battle-tested, attach/detach is free,
  sessions survive `ssf` crashes. claude-squad already proves this works.
- One tmux session per registered dir / worktree, named `ssf-<slug>`.
- `ssf` shells out to `tmux` rather than embedding a PTY library — simpler,
  fewer footguns.

Rejected alternatives: zellij (smaller user base), raw PTYs via `creack/pty`
(more plumbing for no real win at MVP stage).

---

## Worktree lifecycle

- Created under `<repo>/.ssf/worktrees/<repo>-<branch-slug>`.
- `.ssf/` is added to `.git/info/exclude` on first use so it never pollutes
  the repo's tracked files.
- On `r` (remove): kill the tmux session, then `git worktree remove`. Refuse
  if there are uncommitted changes; the confirmation prompt offers a `--force`
  toggle.
- If the underlying branch is deleted or merged upstream, the worktree is
  flagged in the TUI (dim color + marker) but not auto-removed. User decides.

---

## Stack

- **Language: Go**. Single static binary, easy install, fast iteration.
- **TUI: [Bubble Tea](https://github.com/charmbracelet/bubbletea)** + Lipgloss
  for styling, Bubbles for the list/textinput components.
- **Git ops**: shell out to `git` (worktree commands are simple enough — no
  need for `go-git`).
- **File watching**: `fsnotify` for the state directory.
- **Config**: TOML via `BurntSushi/toml`.

Rust + ratatui was considered. Go ships this kind of tool faster.

---

## Config

`~/.config/ssf/config.toml`:

```toml
[[dirs]]
path = "/Users/florent/code/fclairamb/supersetfixed"
last_opened = "2026-04-08T12:34:56Z"

[[dirs]]
path = "/Users/florent/code/stonal/brain"
last_opened = "2026-04-07T09:00:00Z"

[settings]
file_manager = "open"   # overrides $FILE_MANAGER
editor = "zed"          # overrides $VISUAL/$EDITOR
```

---

## MVP slices (build in this order)

Each slice should be independently demoable. **Do not build the worktree UI
before status detection works** — status is the differentiator.

1. `ssf <dir>` registers a directory in `~/.config/ssf/config.toml` and opens
   a Bubble Tea TUI listing registered dirs (hardcoded white status). Search
   with `/`, quit with `q`.
2. `Enter` spawns `tmux new-session -d -s ssf-<slug> "claude"` and attaches.
   Detaching returns to the TUI.
3. **Wire Claude Code hooks** (`Stop`, `Notification`, `UserPromptSubmit`,
   `SessionStart`) to write `.ssf/state/<slug>.json`. Read state files in the
   TUI and color rows accordingly. **Validate the four-color model on a real
   session before moving on.**
4. Worktree creation (`n`) — prompt for a branch name, run `git worktree add`,
   show the new worktree nested under its repo in the TUI.
5. `r` removal with confirmation, `o` file navigator, `e` editor.
6. Sorting by status + last-opened. Hierarchical rendering (subdirs of a repo,
   worktrees nested under repos).
7. **macOS system notifications on state transitions** (core feature, not
   optional). Fire a native notification whenever a session enters `red`
   (waiting for input) or `green` (ready). Use `osascript -e 'display
   notification ...'` for the MVP; upgrade to `terminal-notifier` or a
   `UNUserNotificationCenter` binding later if we need actionable buttons
   ("Attach", "Dismiss"). Notifications must include the repo + branch slug
   so the user knows which session needs them without opening the TUI.

---

## Open questions for later

- Multi-machine sync of the registry? Probably not — keep it local.
- Per-dir custom Claude flags or model selection? Add to config when needed.
