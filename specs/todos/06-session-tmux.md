# 06 — Session manager (tmux wrapper)

## Goal
Spawn, attach to, query, and kill tmux sessions running `claude`. Test it
end-to-end with real tmux but a fake `claude` binary so no real Claude is
launched.

## Deliverables

### `internal/session`

```go
type Manager interface {
    Start(slug, cwd string) error  // tmux new-session -d -s ssf-<slug> -c <cwd> $SSF_CLAUDE_BINARY
    Has(slug string) bool          // tmux has-session -t ssf-<slug>
    Attach(slug string) error      // exec tmux attach -t ssf-<slug>; replaces current process
    Kill(slug string) error        // tmux kill-session -t ssf-<slug>
    List() ([]string, error)       // tmux ls, filtered to ssf- prefix, returns slugs
}
```

- `tmuxManager` real impl, shells out via `os/exec`.
- `fakeManager` in-memory, used by other packages' unit tests.
- Session name format: `ssf-<slug>` (slug from `internal/slug`).
- `Start` is a no-op if `Has(slug)` is true (idempotent).
- `Attach` uses `syscall.Exec` so the user's terminal *becomes* the tmux
  client; on detach, the parent process resumes. The TUI must save and
  restore its own state around this.
- `claude` binary path: `os.Getenv("SSF_CLAUDE_BINARY")`, default `"claude"`.

## Tests

### Unit tests (no tmux required)

- `fakeManager` round-trips Start/Has/Kill/List.
- Command builder: `buildStartArgs(slug, cwd, binary)` returns the right
  argv. Pure function, table-driven.

### Integration tests (`-tags=tmux`)

`internal/session/tmux_test.go`:

1. Skip if `tmux` not on PATH.
2. Create a fake claude script in `t.TempDir()`:
   ```sh
   #!/bin/sh
   exec sleep infinity
   ```
   `chmod +x`. Set `SSF_CLAUDE_BINARY` to its path via `t.Setenv`.
3. `m := NewTmux()`
4. `m.Start("test-"+random, tmpdir)` → no error.
5. `m.Has("test-"+random)` → true.
6. `m.List()` contains `"test-"+random`.
7. `m.Kill("test-"+random)` → `Has` now false.
8. Cleanup `t.Cleanup(func() { m.Kill(...) })` always runs.

Use a random suffix in slugs so parallel `go test` runs don't collide.

`Attach` is not unit-tested (it replaces the process); slice 09's TUI
integration test exercises it indirectly.

## Acceptance

```
go test ./internal/session/...                  # unit tests
make test-tmux                                  # tmux integration tests
```

Both green. The tmux integration test proves I can drive a real session
end-to-end with the fake claude.

## Notes

- Use `tmux -L ssf` (custom socket name) so we don't pollute the user's
  default tmux server. Easier teardown in tests too.
- Capture `tmux` stderr on errors and wrap into the returned error.
