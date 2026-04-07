# 03 — CLI: `ssf <dir>` registers and prints

## Goal
End-to-end CLI smoke: `ssf <dir>` adds the dir to the registry and prints
the rendered list. No TUI yet — stdout only. This proves slices 01 and 02
are wired together correctly.

## Deliverables

### `cmd/ssf/main.go`

- Argv handling:
  - `ssf` (no args) → register cwd, print list.
  - `ssf <dir>` → register `<dir>` (resolved to abs path), print list.
  - `ssf hook write <slug> <state>` — placeholder for slice 05; errors with
    "not implemented" for now but the routing exists.
- After registration, build the rendered list:
  1. Load registry.
  2. For each entry, call `repoinfo.Inspect`.
  3. Collect orgs, run `orgprefix.Derive`.
  4. Render each line: `<prefix>/<repo> [<branch>]` for GitHub repos,
     basename for others.
  5. Print to stdout, newest `LastOpened` first (no status colors yet).

### `internal/render` (new package)

- `Line(dir Dir, info repoinfo.Info, prefix string) string` — pure function,
  unit-testable. Used both by the CLI and (later) the TUI.

## Tests

### `internal/render` unit tests

- GitHub repo with prefix `s` → `s/datalake [main]`.
- Non-GitHub local dir `/tmp/notes` → `notes`.
- Detached HEAD → `s/datalake [HEAD detached]`.

### `cmd/ssf` integration test

`cmd/ssf/main_test.go` builds the binary into `t.TempDir()`, sets
`XDG_CONFIG_HOME` to a temp dir, and runs:

1. `ssf /tmp/foo` → asserts stdout contains `foo`.
2. `ssf /tmp/bar` → asserts stdout contains both `foo` and `bar`, with
   `bar` first (newest).
3. Re-run `ssf /tmp/foo` → `foo` is now first (touched).

Use `os/exec` from the test to run the built binary. No TUI involved, so
stdout capture is trivial.

## Acceptance

```
make build && go test ./cmd/ssf/... ./internal/render/...
```

Green.

I can also manually run:

```
XDG_CONFIG_HOME=$(mktemp -d) ./bin/ssf /tmp/foo
```

and see the entry. (I'll do this once after implementing.)

## Notes

The render package is intentionally split out so the TUI in slice 08 can
reuse it instead of duplicating layout logic.

## Implementation Plan

1. `internal/render/render.go` — `Line(dir, info, prefix)` pure function.
2. `internal/render/render_test.go` — table-driven cases.
3. `cmd/ssf/main.go` — argv routing (`ssf`, `ssf <dir>`, `ssf hook write` placeholder), register, build prefix map, print sorted list.
4. `cmd/ssf/main_test.go` — build the binary into `t.TempDir()`, set `XDG_CONFIG_HOME`, run via `os/exec`, assert stdout.
5. QA.
