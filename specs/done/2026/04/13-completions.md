# 13 — Shell completions

## Goal
Generate shell completions for bash, zsh, and fish via urfave/cli/v3's
built-in support. Users should be able to tab-complete subcommand names
and registered dir paths.

## Deliverables

### `ssf completion <shell>`

```
ssf completion bash
ssf completion zsh
ssf completion fish
```

Print the appropriate completion script to stdout. Conventional install
flow:

```sh
ssf completion bash > ~/.local/share/bash-completion/completions/ssf
```

### Custom completers

For these arguments, hook into urfave/cli's `ShellComplete` mechanism:

| Where               | Suggest                                              |
|---------------------|------------------------------------------------------|
| `ssf <TAB>`         | Directories under cwd (default file completion).     |
| `ssf rm <TAB>`      | Registered dir paths from the registry.              |
| `ssf list <TAB>`    | (no completion — no positional args)                 |

The custom completer for `ssf rm` calls `registry.New(...).List()` and
prints one path per line. Make sure it doesn't error if the registry
is empty.

### README snippet

Update `README.md` (or create one) with:

- One-line description of ssf.
- Install: `go install github.com/fclairamb/ssf/cmd/ssf@latest`.
- The three completion install commands.

(Skip the README if the user doesn't want one — confirm in chat. The
spec author should not silently scope-create a README.)

## Tests

- `cmd/ssf/completion_test.go` — run `ssf completion bash`, assert the
  output starts with `_ssf_bash_autocomplete` (or whatever urfave/cli
  prints) and is non-empty for all three shells.
- For `ssf rm` completion: register two dirs, drive completion via the
  documented urfave/cli mechanism (typically setting
  `SHELL_COMPLETE=true` env var or calling
  `cmd.Command("rm").ShellComplete(ctx, cmd)` in-process), assert the
  output contains both paths.

## Acceptance

```
make test
./bin/ssf completion bash | head -5
./bin/ssf completion zsh  | head -5
./bin/ssf completion fish | head -5
```

All produce non-empty completion scripts. Manual smoke: source one in
your shell and tab-complete `ssf rm <TAB>`.

## Notes

- Don't write completions to disk from the binary. Always print to
  stdout and let the user redirect. Writing to `~/.local/share/...`
  silently is surprising and breaks idempotency.
- urfave/cli/v3's completion API differs from v2's. Read the v3 docs
  before implementing — don't copy v2 examples.
- Manual smoke test (gated under `manual` build tag): no, completions
  are testable in isolation. The full shell-integration check is the
  user's job once.

## Implementation Plan

1. Set `EnableShellCompletion: true` on the root Command. urfave/cli/v3 auto-builds a hidden `completion` Command.
2. Use `ConfigureShellCompletionCommand` to un-hide it so users discover `ssf completion`.
3. Add `ShellComplete` to `rmCommand` that loads the registry and prints registered paths to `cmd.Writer`.
4. Tests in `cmd/ssf/completion_test.go`: assert each shell script is non-empty, and assert that `ssf rm --generate-shell-completion` lists registered paths after registering two dirs.
5. Skip the README — user can ask for it later if needed.
