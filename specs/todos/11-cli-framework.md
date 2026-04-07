# 11 — Adopt urfave/cli/v3

## Goal
Replace the hand-rolled argv parsing in `cmd/ssf/main.go` with
[urfave/cli/v3](https://github.com/urfave/cli). Get free `--help`,
`--version`, error messages on unknown flags, and a coherent place to
attach subcommands in slice 12.

The user-facing default behavior must not change: `ssf`, `ssf .`, and
`ssf ~/code/foo` continue to register the dir and open the TUI. Anything
else is a subcommand.

## Why this slice exists

We just shipped a fix for a stray `--help` directory entry. Root cause:
the binary had no flag parser, so any first argument was a path. Manual
`os.Args` slicing doesn't scale past two cases and will keep producing
bugs of the same shape (`ssf -v`, `ssf foo bar`, etc).

A CLI framework gives us:

- `--help` and per-command help text for free.
- Unknown-flag rejection with helpful errors.
- A `Command.Action` hook so the TUI launch lives in a single, named place.
- A natural extension point for slice 12's subcommands.
- Built-in completion generation (slice 13).

## Deliverables

### Dependency

```
go get github.com/urfave/cli/v3@latest
```

Pin the resulting version in `go.mod`. Do **not** rely on `@latest` in
follow-up commits — v3 has had API churn, locking the version is the
safer call. Re-test after any future bump.

### `cmd/ssf/main.go` rewrite

Structure:

```go
app := &cli.Command{
    Name:    "ssf",
    Usage:   "orchestrate Claude Code instances across worktrees",
    Version: version, // populated from -ldflags
    Action:  rootAction,
    Commands: []*cli.Command{
        hookCommand(), // hidden, see below
    },
    ArgsUsage: "[dir]",
}
if err := app.Run(context.Background(), os.Args); err != nil {
    fmt.Fprintln(os.Stderr, "ssf:", err)
    os.Exit(1)
}
```

`rootAction`:

1. If `cmd.Args().Len() > 1`, return an error ("expected at most one
   directory argument").
2. Resolve target = `cmd.Args().First()` or cwd if empty.
3. `os.Stat` check: must exist, must be a directory. (Same logic we
   added in the previous fix.)
4. `registry.Add` + `hooks.Install`.
5. Build entries, then either `launchTUI` (TTY) or print (pipe).

`hookCommand()`:

```go
return &cli.Command{
    Name:      "hook",
    Hidden:    true, // not for humans
    ArgsUsage: "write <slug> <state>",
    Commands: []*cli.Command{
        {
            Name:      "write",
            ArgsUsage: "<slug> <state>",
            Action:    runHookWrite,
        },
    },
}
```

`runHookWrite` is the existing logic, lifted into a `cli.ActionFunc`
signature.

### Hidden but stable

`ssf hook write <slug> <state>` is a hidden subcommand because hooks are
not user-facing — but the literal string is hard-coded into every
`.claude/settings.json` file ssf installs. **Renaming it breaks every
existing installation.** Lock the name and the argument order. Add a
comment in `internal/hooks/hooks.go` next to the `command()` helper
saying so.

### Version

Add a `version` package var in `cmd/ssf/main.go`:

```go
var version = "dev"
```

Set via `-ldflags="-X main.version=$(git describe --tags --always)"` in
the Makefile's `build` target.

### `--help` output

Manually verify the rendered help text covers:

- `ssf [dir]`: register and open TUI.
- `--help`, `--version`.
- The hidden `hook` subcommand should NOT appear in the top-level help.
- `ssf hook --help` should still work for completeness.

## Acceptance

```
make build && make test
./bin/ssf --help                    # prints help, exits 0
./bin/ssf --version                 # prints "ssf version <something>"
./bin/ssf /nonexistent              # prints helpful error, exits non-zero
./bin/ssf hook write abc ready      # still works (cwd must be a git repo)
./bin/ssf foo bar                   # rejects extra arg with a clear message
```

Existing `cmd/ssf/main_test.go` updated to drive the new entry point but
asserts the same end-to-end behavior.

## Notes

- The `tui` and `hooks` packages don't import urfave/cli — they stay
  independent of the CLI framework. Only `cmd/ssf` knows about it.
- Don't move helpers like `realGit`, `realOpener`, `displayNameLookup`,
  or `launchTUI` into the cli framework. They're plain Go and stay that way.
- urfave/cli/v3's `Command.Action` signature is
  `func(ctx context.Context, cmd *cli.Command) error`. Make sure to
  honor `ctx` for the watcher pump goroutine in `launchTUI`.

## Implementation Plan

(filled in by /implement-todos)
