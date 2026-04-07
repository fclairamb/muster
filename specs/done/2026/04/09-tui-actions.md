# 09 — TUI actions: Enter, o, e, n, r

## Goal
Wire the keybindings to the underlying systems built in slices 01–07.

## Deliverables

### Action handlers in `internal/tui`

| Key     | Handler                                                                                |
|---------|----------------------------------------------------------------------------------------|
| `Enter` | `session.Has(slug)` ? `session.Attach` : `session.Start` then `session.Attach`         |
| `o`     | `exec.Command(settings.FileManager(), entry.Path).Start()` (default `open`)            |
| `e`     | `exec.Command(settings.Editor(), entry.Path).Start()` ($VISUAL → $EDITOR → `zed`)      |
| `n`     | Modal: prompt branch name → `git worktree add .ssf/worktrees/<repo>-<slug> -b <name>`  |
| `r`     | Confirmation modal → unregister / kill session / `git worktree remove`                 |

### Pure builders (testable)

- `BuildWorktreeAddArgs(repo, branch string) []string`
- `BuildWorktreeRemoveArgs(worktreePath string, force bool) []string`
- `IsDirty(repo string) (bool, error)` — wraps `git status --porcelain`.

### Modal flows

- `BranchPromptModel` — single-line text input with validation
  (no whitespace, no slashes that would escape the worktree dir).
- `ConfirmModel{Question, Detail, OnYes, OnNo}` — generic.
- `r` confirmations spell out exactly what's being removed:
  - Registered dir: `Unregister <display>? Sessions and worktrees are kept.`
  - Worktree: `Remove worktree <branch> AND kill its session?`
  - Dirty worktree: refuses unless a `[ ] force` checkbox is toggled.

### Attach lifecycle

`session.Attach` uses `syscall.Exec`, which would replace the TUI
process. To preserve the TUI:

1. On `Enter`, call `tea.ExecProcess` with the tmux attach command.
2. Bubble Tea suspends, runs tmux attach in the foreground, resumes
   when the user detaches.

`tea.ExecProcess` is the documented mechanism for exactly this.

## Tests (`go test ./internal/tui/...`)

### Pure unit tests

- `BuildWorktreeAddArgs("/repo", "feat/x")` → expected argv.
- `BuildWorktreeRemoveArgs(...)` with and without `--force`.
- Branch name validation: rejects empty, whitespace, `..`, `/foo/../bar`.

### teatest flows

- Press `n`, type `feat/foo`, `Enter` → modal closes, action recorded
  via a `fakeGitRunner` injected into the model.
- Press `r` on a clean worktree → confirmation modal → `y` → session
  killed (via `fakeManager`), worktree-remove command issued (via
  `fakeGitRunner`).
- Press `r` on a dirty worktree → modal shows the dirty warning,
  refuses to proceed unless `force` is toggled.
- Press `o` → `fakeOpener` records the call with the right path.
- Press `e` → `fakeOpener` records `$VISUAL` → `$EDITOR` → `zed`
  fallback chain. Use `t.Setenv` to test each path.

### Real tmux integration test (`-tags=tmux`)

- Build a model with a registered fake-claude entry.
- Drive `Enter` via teatest (which can intercept `tea.ExecProcess`).
- Assert `session.Has(slug)` is true after the action.
- Call `Kill` directly to clean up.

## Acceptance

```
go test ./internal/tui/...           # unit + teatest
make test-tmux                       # integration
```

Both green.

## Notes

Every external command goes through an injected interface
(`gitRunner`, `opener`, `Manager`, `Notifier`). The model is constructed
with real impls in `main.go` and fakes in tests. No globals.

## Implementation Plan

1. `internal/tui/actions.go` — Deps struct (Session, Git, Opener), pure builders (BuildWorktreeAdd/RemoveArgs, ValidateBranchName).
2. `internal/tui/fakes.go` — fakeGit, fakeOpener for tests.
3. Extend `internal/tui/model.go` — modal state (branch prompt + confirm), Enter/o/e/n/r key routing through Deps.
4. `internal/tui/actions_test.go` — pure builder + validation tests.
5. `internal/tui/model_actions_test.go` — drive Update with fake Deps; assert calls.
6. QA.
