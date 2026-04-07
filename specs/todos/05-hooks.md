# 05 — Claude Code hook installer + `ssf hook write`

## Goal
Make Claude Code itself report state transitions into `.ssf/state/<slug>.json`.
This is the differentiator of the whole product. **Do not move past this
slice without the manual smoke test passing** — until then, the four-color
model is unverified speculation.

## Deliverables

### `ssf hook write <slug> <kind>` subcommand

- Hidden CLI subcommand. Resolves the repo root from cwd
  (`git rev-parse --show-toplevel`), then writes a `state.State` via
  `state.Write`. Designed to be invoked from Claude Code hooks.
- Exits 0 on success, non-zero with stderr on error.
- Must be fast (no config load, no network) — Claude waits on hooks.

### `internal/hooks`

- `Install(repoRoot string, slug string) error` — merges into
  `<repoRoot>/.claude/settings.json`:
  ```jsonc
  {
    "hooks": {
      "SessionStart":     [{"hooks": [{"type": "command", "command": "ssf hook write <slug> idle"}]}],
      "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "ssf hook write <slug> working"}]}],
      "Stop":             [{"hooks": [{"type": "command", "command": "ssf hook write <slug> ready"}]}],
      "Notification":     [{"hooks": [{"type": "command", "command": "ssf hook write <slug> waiting_input"}]}]
    }
  }
  ```
- Merge semantics:
  - If the file doesn't exist, create it.
  - If it exists, parse, append our hook entries to existing arrays under
    each key, dedupe by exact command string.
  - Preserve all unrelated keys verbatim. Use `encoding/json` with
    `json.RawMessage` for unknown fields.
- `Uninstall(repoRoot, slug string) error` — removes only the entries
  whose command matches our slug.
- Idempotent: `Install` twice produces the same file as `Install` once.

### Wiring

- `registry.Add` (slice 01) calls `hooks.Install` after a successful add.
  Failures are logged but don't abort registration.
- `registry.Remove` calls `hooks.Uninstall`.

## Tests (`go test ./internal/hooks/... ./cmd/ssf/...`)

- Install into a fresh dir → file matches a golden fixture.
- Install into a dir with unrelated `permissions` config → unrelated keys
  preserved verbatim.
- Install into a dir with existing hooks for other slugs → ours appended,
  others kept.
- Install twice → second call is a no-op (file unchanged byte-for-byte).
- Uninstall removes only our entries; other slugs' entries remain.
- `ssf hook write` integration test: build the binary, run it inside a
  temp git repo with a known slug, assert `.ssf/state/<slug>.json` exists
  with the right Kind.

## Manual smoke test (REQUIRED before slice 06)

Documented in this spec, executed by the user (me):

1. `make build`
2. `cd ~/code/fclairamb/some-throwaway-repo`
3. `./bin/ssf .` (registers + installs hooks)
4. Open Claude Code in that repo. Verify `.claude/settings.json` has the
   four hooks.
5. Send a prompt to Claude. Watch `.ssf/state/<slug>.json`:
   - Immediately after sending → `working`
   - After Claude finishes → `ready` (within 2s of the debounce)
   - If Claude asks for permission → `waiting_input`
6. **Document any divergence from the spec in this file.** If `Notification`
   does not actually fire on plain "waiting for user input" (only on
   permission prompts), update CLAUDE.md to reflect what works and adjust
   the four-color model. This is the day-1 reality check.

If the smoke test reveals the heuristic is wrong, **do not paper over it** —
update slice 04 and CLAUDE.md before proceeding.

## Acceptance

- `go test ./internal/hooks/... ./cmd/ssf/...` green.
- Manual smoke test executed and findings documented inline below.

### Smoke test results

(to be filled in after running)
