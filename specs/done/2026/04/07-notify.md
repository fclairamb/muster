# 07 — macOS notifications + transition dispatcher

## Goal
Fire a native macOS notification when any session transitions to `ready`
(green) or `waiting_input` (red). This is a core feature, not optional —
the user uses `ssf` precisely because they don't want to babysit the TUI.

## Deliverables

### `internal/notify`

```go
type Notifier interface {
    Notify(title, body string) error
}
```

- `OsascriptNotifier` real impl. Calls
  `osascript -e 'display notification "<body>" with title "<title>"'`.
  Escape quotes safely (no string interpolation footguns).
- `RecordingNotifier` for tests:
  ```go
  type Recording struct{ Title, Body string }
  type RecordingNotifier struct{ Calls []Recording }
  ```

### `internal/notify/dispatch`

```go
type Dispatcher struct {
    notifier Notifier
    last     map[string]state.Kind  // slug → last seen kind
    name     func(slug string) string  // display name lookup
}

func (d *Dispatcher) Handle(ev watcher.Event)
```

Rules:

- On first event for a slug, record `last` but do not notify.
- On transition `* → ready`: notify with title `ssf · <display>`,
  body `Ready`.
- On transition `* → waiting_input`: notify with title `ssf · <display>`,
  body `Needs input`.
- All other transitions: silent.
- Same kind twice in a row: silent.
- Display name comes from the same `orgprefix` machinery as the TUI, so
  notifications read like `ssf · s/datalake [main]`.

## Tests (`go test ./internal/notify/...`)

Dispatcher tests, table-driven:

| Sequence of (slug, kind)                 | Expected notifications                                  |
|------------------------------------------|---------------------------------------------------------|
| `(a, working) → (a, ready)`              | 1 notif: `Ready`, body contains display name for `a`    |
| `(a, ready) → (a, ready)`                | 0 notifs                                                |
| `(a, working) → (a, waiting_input)`      | 1 notif: `Needs input`                                  |
| `(a, idle) → (a, working) → (a, ready)`  | 1 notif: `Ready` (working transition is silent)         |
| First-ever `(a, ready)`                  | 0 notifs (no prior state to transition from)            |

Use `RecordingNotifier`. Assert exact `Calls` slice.

`OsascriptNotifier` test (default tag, mocked): inject a fake `osascript`
binary via `$PATH` manipulation that records its argv to a tempfile.
Assert the right argv was passed and dangerous chars are escaped.

## Manual smoke test (`-tags=manual`)

```go
//go:build manual
func TestRealOsascript(t *testing.T) {
    n := notify.OsascriptNotifier{}
    if err := n.Notify("ssf test", "if you can read this, notifications work"); err != nil {
        t.Fatal(err)
    }
}
```

Run via `make smoke`. User confirms a notification appeared.

## Acceptance

```
go test ./internal/notify/...
```

Green. Manual smoke test executed once and confirmed visually.

## Implementation Plan

1. `internal/notify/notify.go` — Notifier interface, Recording fake, OsascriptNotifier real impl with safe escaping.
2. `internal/notify/dispatch.go` — Dispatcher with `Handle(Event)`, transition rules, name resolver injected.
3. `internal/notify/notify_test.go` — Dispatcher table tests + osascript escaping unit tests.
4. `internal/notify/manual_test.go` (//go:build manual) — real osascript smoke test.
5. QA.
