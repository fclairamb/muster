# 12 — Subcommands: list, rm, version

## Goal
Add explicit subcommands for the operations that currently exist only
inside the TUI or have no entry point at all. After slice 11 lays the
urfave/cli foundation, this slice fills in the verbs.

## Subcommands to add

| Command                    | Behavior                                                         |
|----------------------------|------------------------------------------------------------------|
| `ssf list`                 | Print the registered dirs as the TUI would render them, no TUI. |
| `ssf rm <path-or-slug>`    | Unregister a dir (calls the same path as TUI's `r` action).      |
| `ssf version`              | Same as `--version`. Conventional ergonomic alias.               |
| `ssf hook write` (hidden)  | Already exists — leave it alone.                                 |

The bare `ssf` and `ssf <dir>` invocations remain primary and unchanged.

## Why no `ssf register <dir>` subcommand?

Because `ssf <dir>` already does that. Adding `ssf register <dir>` as an
alias would split user muscle memory and add no value. Resist.

## Deliverables

### `ssf list`

```
ssf list [--json]
```

- Default: same line-per-entry format the non-TTY fallback already uses.
- `--json` flag: print a JSON array of `{path, display, kind, last_opened}`.
  Useful for shell scripting and future shell completion.
- No registry mutation, no hook install.
- Exits 0 with empty output if the registry is empty.

### `ssf rm <path-or-slug>`

```
ssf rm <path-or-slug>
ssf rm <path-or-slug> --force   # for worktree entries with uncommitted changes
```

- Argument can be either an absolute path or a 12-char slug. The handler
  must resolve a slug back to a path by iterating the registry.
- Uses the same code path as the TUI's `r` action — refactor it into a
  shared `internal/actions` helper if necessary, OR have the cli command
  drive a small headless `tui.RemoveAction` that takes Deps. Don't
  duplicate the worktree-vs-registered-dir branching logic.
- Refuses ambiguous arguments (slug prefix that matches more than one
  entry) with a clear error.
- Prints what was removed on success.

### `ssf version`

Just calls into urfave/cli's version printer. One-line subcommand.

## Tests

- `cmd/ssf/list_test.go` — build the binary, register two dirs, run
  `ssf list`, assert two lines. Run `ssf list --json`, assert valid JSON
  with two elements.
- `cmd/ssf/rm_test.go` — register a dir, run `ssf rm <abs-path>`,
  assert registry is empty. Run `ssf rm <slug>`, same assertion.
- `cmd/ssf/rm_test.go` — register two dirs whose slugs share a prefix,
  run `ssf rm <ambiguous-prefix>`, assert non-zero exit and error message.
  (If slugs are 12-char hex they almost never share prefixes naturally;
  the test can use a stub slug function or just an exact-match policy.)

All tests use `XDG_CONFIG_HOME` in `t.TempDir()` and shell out to the
built binary like the existing CLI tests.

## Acceptance

```
make test
./bin/ssf list
./bin/ssf rm /tmp
./bin/ssf rm dead-beef-cafe
./bin/ssf rm too-short        # error: ambiguous or not found
./bin/ssf version
```

All commands behave as documented; help text reflects them.

## Notes

- `ssf rm` should only target *registered top-level dirs*, not worktrees.
  Worktree removal stays a TUI-only operation in this slice — adding
  worktree removal to the CLI is scope creep until worktrees are
  actually creatable from the CLI too. Document this in `--help`.
- Slug matching is exact-only by default. Don't add prefix matching
  unless the user asks; ambiguity is a footgun.
- Refactor the TUI's `handleConfirmRemoveKey` so that the
  registered-dir branch lives in a function the CLI can call directly.
  Keep the worktree branch where it is — it's TUI-shaped (modal,
  cursor-driven).

## Implementation Plan

(filled in by /implement-todos)
