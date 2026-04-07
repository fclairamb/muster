# 08 — TUI rendering: list, search, sort, hierarchy

## Goal
Bubble Tea model that renders the registered dirs as a hierarchical,
color-coded, sortable, searchable list. No actions yet (slice 09 wires
those). Fully tested with `teatest`.

## Deliverables

### `internal/tui`

- `Model` (Bubble Tea):
  - `entries []Entry` (loaded from registry + repoinfo + state)
  - `cursor int`
  - `search string`
  - `searching bool`
- `Entry`:
  ```go
  type Entry struct {
      Display  string      // "s/datalake [main]"
      Indent   int         // 0 for repos, 1 for subdirs/worktrees
      Kind     state.Kind
      Path     string
      Slug     string
      LastOpen time.Time
  }
  ```
- Render:
  - Status emoji prefix per row, per the table in CLAUDE.md.
  - Indent worktrees and subdirs under their parent repo.
  - Highlight the cursor row.
  - Footer line: `↑↓ move  / search  q quit` (etc).
- Sort order (per CLAUDE.md):
  1. Red (`waiting_input`)
  2. Green (`ready`)
  3. Yellow (`working`)
  4. White (`idle`)
  5. None — by `LastOpen` desc
- Search: `/` enters search mode, typing filters entries by
  case-insensitive substring on `Display`. `Esc` exits search.
- Quit: `q` or `Ctrl+C`.

### `internal/tui/feed`

- `Feed(ctx, cfgPath) <-chan Model` — combines registry load + state
  watcher into a stream of model updates. Lets the dispatcher and TUI
  share the same data plane.

## Tests (`go test ./internal/tui/...`)

Use `github.com/charmbracelet/x/exp/teatest`:

1. **Initial render golden test.** Seed model with 5 hand-crafted entries
   covering all 5 status kinds. Take a final snapshot, compare against
   `testdata/initial.golden`. Update goldens with `-update`.
2. **Sort order test.** Same 5 entries, shuffled input → asserts the
   render order matches the spec.
3. **Search test.** Send `/`, then `dat`, then `Enter`. Snapshot shows
   only entries containing `dat`.
4. **Cursor movement.** `↓↓↓` then snapshot shows cursor on row 4.
5. **Quit test.** `q` → `WaitFinished` returns.
6. **Hierarchy test.** Seed with a repo + a worktree + a subdir. Render
   shows worktree indented under the repo, subdir as a separate top-level
   entry per the example in CLAUDE.md.

Goldens live in `internal/tui/testdata/`. Use `teatest.RequireEqualOutput`
with `tea.WithFinalOutput`.

## Acceptance

```
go test ./internal/tui/...
```

Green. The goldens prove the TUI renders correctly without ever opening
a real terminal.

## Notes

`teatest` is the official Charm test harness. It writes input bytes to
the program and captures rendered output. This is *the* mechanism that
lets me verify the TUI without a human.

## Implementation Plan

1. `internal/tui/entry.go` — Entry struct + sort comparator (status, then last-opened desc).
2. `internal/tui/model.go` — Bubble Tea Model with cursor, search state, Update, View. Tests drive Update directly with `tea.KeyMsg`s rather than spinning up `teatest` (faster, no goroutine timing).
3. `internal/tui/model_test.go` — sort, filter (`/`), cursor movement, hierarchy indent, quit message.
4. QA.
