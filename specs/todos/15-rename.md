# 15 — Rename to `muster`

## Goal

Rename the project end-to-end from `ssf` / `supersetfixed` to **`muster`**.
Drop the "supersetfixed" framing entirely. Provide a one-shot migration so
existing users don't lose their registry, hooks, or state files when the
binary changes name underneath them.

## What changes

| Layer                | Before                                | After                                    |
|----------------------|---------------------------------------|------------------------------------------|
| GitHub repo          | `fclairamb/supersetfixed`             | `fclairamb/muster` (manual: `gh repo rename`) |
| Go module path       | `github.com/fclairamb/ssf`            | `github.com/fclairamb/muster`            |
| Binary               | `ssf`                                 | `muster` (with `mst` symlink for fast typing) |
| Tmux socket          | `tmux -L ssf …`                       | `tmux -L muster …`                       |
| Tmux session prefix  | `ssf-<slug>`                          | `muster-<slug>`                          |
| Per-repo state dir   | `<repo>/.ssf/state/`                  | `<repo>/.muster/state/`                  |
| Config dir           | `~/.config/ssf/config.toml`           | `~/.config/muster/config.toml`           |
| Hook command         | `ssf hook write <slug> <kind>`        | `muster hook write <slug> <kind>`        |
| Env override         | `$SSF_CLAUDE_BINARY`                  | `$MUSTER_CLAUDE_BINARY`                  |
| README.md / CLAUDE.md| references to "ssf" / "supersetfixed" | references to "muster" only              |

The acronym `ssf` is fully retired. No backronym, no "ssf legacy" notes
in the docs. New users should never encounter the old name.

## One-shot migration

New subcommand: **`muster migrate`**. Also runs **automatically** the first
time `muster` starts and detects an `ssf` install with no `muster` install.

### Algorithm

1. **Config**: if `$XDG_CONFIG_HOME/ssf/config.toml` exists and
   `$XDG_CONFIG_HOME/muster/config.toml` does not:
   - Read the old config.
   - Write it verbatim to the new path.
   - **Do not delete** the old config — leave it as a recovery breadcrumb.
2. **Per-repo migration**: for each `Dir` in the new config:
   - If `<repo>/.ssf/state/` exists and `<repo>/.muster/state/` does not,
     `os.Rename` it. Best-effort — log a warning if the parent dir is
     missing or the move fails.
   - Re-run `hooks.Install(repoRoot, slug)` so `.claude/settings.json`
     gets the new `muster hook write` commands.
   - Strip any leftover `ssf hook write <slug>` entries via a new
     `hooks.UninstallLegacy(repoRoot)` helper that scrubs commands
     starting with the literal `ssf hook write`. Independent of slug.
3. **Print a summary**: `migrated 3 repos, 3 hooks reinstalled`.

### Idempotency

Running `muster migrate` twice is a no-op the second time:

- The config check sees the new file exists and skips.
- Re-installing hooks is already idempotent.
- Re-renaming `.ssf` → `.muster` only happens when `.ssf` still exists.

### What is NOT migrated

- **Running tmux sessions on the `-L ssf` socket** are left alone. They
  die naturally when the user quits them. Document this in the migrate
  command's output: `note: existing 'ssf' tmux sessions are still
  reachable via 'tmux -L ssf attach' and will be cleaned up on quit.`
- **The old `.ssf/` directories** are renamed (not deleted), so the user
  can verify the migration before manual cleanup.
- **The `~/.local/bin/ssf` binary** is not removed and not symlinked.
  Users uninstall it manually with `rm ~/.local/bin/ssf`.

## Deliverables

### Repository surgery

1. `go mod edit -module github.com/fclairamb/muster`.
2. Update every Go import path: `github.com/fclairamb/ssf` →
   `github.com/fclairamb/muster`.
3. Rename `cmd/ssf/` → `cmd/muster/`.
4. Update `Makefile`:
   - `build` target writes `bin/muster`.
   - `install` target copies `bin/muster` to `$PREFIX/bin/muster` AND
     creates a relative symlink `$PREFIX/bin/mst → muster` so the
     three-letter shortcut is available without requiring a separate
     binary build.
   - `uninstall` (if added later) removes both files.

### Constant renames

1. `internal/session/session.go`:
   - `SocketName = "ssf"` → `"muster"`
   - `SessionPrefix = "ssf-"` → `"muster-"`
2. `internal/session/session.go`: `claudeBinary()` reads
   `$MUSTER_CLAUDE_BINARY`.
3. `internal/state/state.go`: `DirPath` and `FilePath` use `.muster`
   instead of `.ssf`.
4. `internal/config/config.go`: `DefaultPath()` uses `muster` instead
   of `ssf` in the path.
5. `internal/hooks/hooks.go`: `command()` builds the literal
   `"muster hook write " + slug + " " + kind`. Update the don't-rename
   warning comment to reflect that `muster hook write` is now the
   public contract — and that this rename is the one-time exception.
6. `cmd/muster/main.go`: `hookCommand()` keeps the same shape, name
   `hook`, hidden, with `write` subcommand. Same arg shape.
7. `cmd/muster/main.go`: app name `"muster"`, usage strings updated.

### `mst` shortcut

The `mst` symlink is the only place we expose the three-letter form. It
must behave identically to `muster` in every situation:

- All subcommands work via the symlink (`mst list`, `mst rm`, etc).
- `mst --help` and `mst --version` produce the same output as the long
  form. urfave/cli's `Name` field is hardcoded to `"muster"`, so the
  help text always says `muster` regardless of how the binary was
  invoked. That's intentional — one canonical name in docs, one short
  alias for typing. Don't try to make the binary self-rename based on
  `os.Args[0]`; the inconsistency would be more confusing than helpful.
- The hook command literal stays `muster hook write` even when the user
  invoked `mst` to register the dir. The slug must round-trip through
  the long form so existing installations don't break if the symlink
  is later removed.

### Migration code

1. New `cmd/muster/migrate.go`:
   - `migrateCommand() *cli.Command` returning a visible (not hidden)
     `migrate` command. Visible so users can re-run it manually.
   - `runMigrate(ctx, cmd)` performs the algorithm above.
   - `autoMigrateIfNeeded()` is called from `main()` *before*
     `app.Run()` and runs the migration silently if the trigger
     condition holds (old config exists, new doesn't). On success,
     prints a single stderr line: `muster: migrated configuration
     from ssf`.
2. `internal/hooks/hooks.go`: new `UninstallLegacy(repoRoot string) error`
   that walks `.claude/settings.json` and removes any inner hook whose
   command starts with `ssf hook write `. Reuses the existing
   `removeHook` plumbing with a different prefix.

### Documentation

1. `README.md`: rewrite the title, install steps, all examples. Drop
   "supersetfixed". Add a one-paragraph "Migrating from ssf" section
   explaining `muster migrate`.
2. `CLAUDE.md`: replace every `ssf` with `muster`. Update the
   "ssf hook write is a public contract" section to reference
   `muster hook write` and add a note that the one-time rename used
   slice 15's migration command.
3. The archived spec `specs/done/2026/04/05-hooks.md` is **not**
   modified — frozen history.

## Tests

### New

- `cmd/muster/migrate_test.go`:
  - Build the binary.
  - Stage a fake `~/.config/ssf/config.toml` with two registered dirs.
  - Each dir is a real git repo with `.ssf/state/<slug>.json` and a
    `.claude/settings.json` containing legacy `ssf hook write` commands.
  - Run `muster migrate`.
  - Assert: new config exists, old config still exists, `.muster/state`
    exists, `.ssf/state` is gone (renamed), `.claude/settings.json`
    references `muster hook write` and contains no `ssf hook write`.
  - Run `muster migrate` again, assert no errors and no changes.

- `internal/hooks/hooks_test.go`:
  - `TestUninstallLegacy`: settings.json with two hook entries (one
    legacy `ssf hook write`, one current `muster hook write`); call
    `UninstallLegacy`; assert only the current one remains.

### Updated

Every test that hardcodes `"ssf"`, `"ssf-"`, `"ssf hook write"`,
`".ssf/state"`, or `"github.com/fclairamb/ssf"` needs updating.
Notable hot spots:

- `internal/session/session_test.go` (`buildStartArgs` golden uses `"ssf"`)
- `internal/session/tmux_test.go` (build tag `tmux`)
- `internal/state/state_test.go`
- `internal/state/watcher/watcher_test.go`
- `internal/hooks/hooks_test.go`
- `cmd/muster/main_test.go`
- `test/e2e/e2e_test.go`

### Build tags unchanged

Default + `tmux` + `e2e` + `manual` still apply.

## Acceptance

```sh
make build && make vet && make test && make test-tmux
make install
./bin/muster --version
./bin/muster --help

# Symlink works and resolves to the same binary:
test -L $HOME/.local/bin/mst
mst --version            # same output as muster --version
mst list                 # same as muster list

# Migration smoke test:
mkdir -p /tmp/ssf-rename-test
XDG_CONFIG_HOME=/tmp/ssf-rename-test mkdir -p /tmp/ssf-rename-test/ssf
echo '[[dirs]]' > /tmp/ssf-rename-test/ssf/config.toml
echo 'path = "/tmp"' >> /tmp/ssf-rename-test/ssf/config.toml
echo 'last_opened = 2026-04-08T00:00:00Z' >> /tmp/ssf-rename-test/ssf/config.toml
XDG_CONFIG_HOME=/tmp/ssf-rename-test ./bin/muster migrate
test -f /tmp/ssf-rename-test/muster/config.toml
```

## Notes / gotchas

- **Don't break the build during the rename.** Each commit must leave
  the tree green. Suggested order:
  1. `go mod edit -module` + import path rewrite (one mechanical commit
     via `gofmt -r` or `find … sed`).
  2. Rename `cmd/ssf/` → `cmd/muster/`.
  3. Constant renames (socket, prefix, dirs, env var).
  4. Hook command literal rename + `UninstallLegacy` helper.
  5. Migration command + auto-migration glue.
  6. Documentation rewrite.
- **The hook command literal is the one place where the don't-rename
  rule is being broken on purpose.** Update the warning comment in
  both `internal/hooks/hooks.go` and `cmd/muster/main.go` to reflect
  the new contract and reference this spec.
- **Auto-migration runs unconditionally before the CLI dispatches**,
  so it must be cheap and silent in the steady state. The check is
  two `os.Stat` calls — old config exists AND new config doesn't.
  Anything else is a no-op.
- **GitHub repo rename** (`gh repo rename muster`) is a manual step
  outside this slice. The README and CLAUDE.md should already use the
  new URL, on the assumption the user runs it after the merge.
- **Local checkout directory** stays `~/code/fclairamb/supersetfixed`
  until the user runs `mv` themselves. Out of scope.
- **`go install github.com/fclairamb/ssf/cmd/ssf@latest` will stop
  working** the moment the GitHub repo is renamed. README must show
  the new install path immediately.

## Implementation Plan

(filled in by /implement-todos)
