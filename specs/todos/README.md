# specs/todos — implementation plan for ssf

11 slices, built in order. Each slice ends with a green test command runnable
without a human at the keyboard. Manual smoke tests are flagged explicitly.

## Self-testability principles

1. All side-effecting code sits behind a Go interface; tests use fakes.
2. The TUI is tested with [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest)
   (programmatic input + golden output). No real terminal required.
3. tmux is real in tmux-tagged tests, but the `claude` binary is replaced
   with a `sleep infinity` shell script via `$SSF_CLAUDE_BINARY`. We can
   drive a real tmux session end-to-end without ever launching real Claude.
4. macOS notifications dispatch through a `Notifier` interface. The
   dispatcher logic is fully unit-testable with a recording fake. Real
   `osascript` calls are gated behind a `manual` build tag.
5. **The only unavoidable human gate is slice 05** — verifying that real
   Claude Code hooks actually fire and produce the expected state
   transitions. Until that smoke test passes, the four-color model is
   unverified. Do not move past slice 05 without it.

## Build tags

| Tag       | Meaning                                                  |
|-----------|----------------------------------------------------------|
| (default) | Pure unit tests. CI runs these.                          |
| `tmux`    | Integration tests that shell out to a real tmux server.  |
| `e2e`     | Full-stack integration tests (slice 10).                 |
| `manual`  | Smoke tests that need a human to observe the result.     |

## Slices

- [00-skeleton.md](00-skeleton.md) — Go module, layout, Makefile, CI
- [01-config-registry.md](01-config-registry.md) — TOML config + add/remove/touch
- [02-slug-orgprefix.md](02-slug-orgprefix.md) — stable slugs + shortest-unique-prefix abbreviation
- [03-cli-register.md](03-cli-register.md) — `ssf <dir>` wires config to a printable list
- [04-state-watcher.md](04-state-watcher.md) — state file model + fsnotify watcher with debounce
- [05-hooks.md](05-hooks.md) — Claude Code hook installer + `ssf hook write` (**manual gate**)
- [06-session-tmux.md](06-session-tmux.md) — tmux session manager with fake-claude tests
- [07-notify.md](07-notify.md) — macOS notifier + state-transition dispatcher
- [08-tui-render.md](08-tui-render.md) — Bubble Tea list, search, sort, hierarchy
- [09-tui-actions.md](09-tui-actions.md) — Enter, o, e, n, r with modal flows
- [10-e2e.md](10-e2e.md) — full-stack integration test using fake claude + fake notifier
