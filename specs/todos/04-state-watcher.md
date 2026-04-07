# 04 — State files + filesystem watcher

## Goal
Define the state file format, read/write atomically, and watch a directory
of state files for changes with debounce. This is the data substrate the
TUI will render and the notifier will react to.

## Deliverables

### `internal/state`

```go
type Kind string

const (
    KindNone         Kind = "none"          // no instance
    KindIdle         Kind = "idle"          // attached but no activity
    KindWorking      Kind = "working"       // mid-prompt
    KindReady        Kind = "ready"         // green: result ready
    KindWaitingInput Kind = "waiting_input" // red: needs user
)

type State struct {
    Kind    Kind      `json:"kind"`
    Ts      time.Time `json:"ts"`
    Session string    `json:"session"`
}
```

- `Read(repoRoot, slug string) (State, error)` — missing file → `KindNone`,
  no error. Corrupt JSON → `KindNone` + logged warning, no error.
- `Write(repoRoot, slug string, s State) error` — atomic (temp + rename),
  creates `<repoRoot>/.ssf/state/` if missing.

### `internal/state/watcher`

- `Watch(ctx context.Context, dirs []string) <-chan Event` where
  `Event{RepoRoot, Slug, State}`.
- Uses `github.com/fsnotify/fsnotify`.
- **Debounce:** coalesces multiple writes to the same `(repoRoot, slug)`
  within a 250ms window into a single event.
- **Heuristic for green:** if `KindReady` is observed and no
  `KindWorking`/`KindWaitingInput` arrives for the same session within 2s,
  emit `KindReady` downstream. (Implements the "Stop + no follow-up =
  green" rule from CLAUDE.md.) Configurable via package var so tests can
  shorten it.
- Robust to dirs that don't exist yet (created later by hooks).

## Tests (`go test ./internal/state/...`)

- `Write` then `Read` round-trips.
- `Read` on missing file returns `KindNone`.
- `Read` on corrupt JSON returns `KindNone` + no error.
- Watcher: write a state file → event arrives within 500ms.
- Watcher: write 5 times rapidly → exactly 1 event (debounce works).
- Heuristic: emit `Ready` then nothing → downstream sees `Ready` after the
  debounce window. With test-shortened debounce of 50ms.
- Heuristic: emit `Ready` then `Working` within 50ms → downstream sees
  `Working`, not `Ready`.

All tests use `t.TempDir()` and an injected clock or shortened windows.

## Acceptance

```
go test ./internal/state/...
```

Green.

## Notes

The watcher must survive `os.Remove` of the state dir without crashing
(it can happen during `r` cleanup). On removal, log a warning, drop the
events, keep watching the parent.

## Implementation Plan

1. `internal/state/state.go` — Kind constants, State struct, Read/Write with atomic writes; missing/corrupt → KindNone, no error.
2. `internal/state/state_test.go` — round-trip, missing, corrupt.
3. `internal/state/watcher/watcher.go` — fsnotify-based Watch returning a channel; debounce window + green-confirm window; survives parent dir removal.
4. `internal/state/watcher/watcher_test.go` — write→event, debounce coalescing, green confirmation, working-overrides-ready.
5. QA.
