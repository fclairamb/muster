# 01 — Config + registry

## Goal
Persist the list of registered directories on disk and expose CRUD operations.

## Deliverables

### `internal/config`

- Config path resolution: `$XDG_CONFIG_HOME/ssf/config.toml`, fall back to
  `~/.config/ssf/config.toml`. Honor `$XDG_CONFIG_HOME` for tests.
- Types:
  ```go
  type Config struct {
      Dirs     []Dir    `toml:"dirs"`
      Settings Settings `toml:"settings"`
  }
  type Dir struct {
      Path       string    `toml:"path"`
      LastOpened time.Time `toml:"last_opened"`
  }
  type Settings struct {
      FileManager  string            `toml:"file_manager"`  // default "open"
      Editor       string            `toml:"editor"`        // default "" → $VISUAL → $EDITOR → "zed"
      OrgOverrides map[string]string `toml:"org_overrides"` // org → preferred prefix
  }
  ```
- `Load() (Config, error)` — missing file returns empty config, no error.
- `Save(Config) error` — atomic write (tempfile + rename), creates parent dir.

Use `github.com/BurntSushi/toml`.

### `internal/registry`

Thin layer over `config`:

- `Add(path string) error` — abs-path normalize, dedupe, set `LastOpened = now`.
- `Remove(path string) error` — no-op if absent.
- `Touch(path string) error` — bump `LastOpened`. Error if absent.
- `List() ([]Dir, error)` — pass-through.

## Tests (`go test ./internal/config/... ./internal/registry/...`)

- `Load` on a missing file returns empty config.
- `Save → Load` round-trip preserves all fields.
- `Save` is atomic: corrupting the temp file path mid-write doesn't damage
  the existing config.
- `Add` is idempotent (calling twice keeps one entry, second call updates
  `LastOpened`).
- `Add` resolves relative paths to absolute.
- `Touch` on missing path returns a sentinel error.
- Settings defaults: empty `FileManager` resolves to `"open"` via a helper.

All tests use `t.TempDir()` and set `XDG_CONFIG_HOME` via `t.Setenv`.

## Acceptance

```
go test ./internal/config/... ./internal/registry/...
```

Green.

## Implementation Plan

1. Add `github.com/BurntSushi/toml` to go.mod via `go get`.
2. `internal/config/config.go` — Path resolution, types, Load (missing → empty, no error), Save (atomic via tempfile + rename), helpers for default `FileManager`/`Editor`.
3. `internal/config/config_test.go` — round-trip, missing file, defaults, atomic-ish behavior.
4. `internal/registry/registry.go` — Add (abs+dedupe+touch), Remove, Touch, List. Wraps config with `Path string` field for per-test isolation.
5. `internal/registry/registry_test.go` — idempotent Add, relative-path normalization, Touch on missing returns ErrNotFound.
6. QA: `make test` and `make vet`.

