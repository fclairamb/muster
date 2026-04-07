# 00 — Project skeleton

## Goal
Buildable, testable Go project with the layout the rest of the slices assume.

## Deliverables

- `go.mod` — `module github.com/fclairamb/ssf`, Go 1.22+.
- `cmd/ssf/main.go` — entry point. Prints `ssf` for now.
- Empty packages (with a `doc.go` each so `go vet` is happy):
  - `internal/config`
  - `internal/registry`
  - `internal/slug`
  - `internal/orgprefix`
  - `internal/repoinfo`
  - `internal/state`
  - `internal/state/watcher`
  - `internal/hooks`
  - `internal/session`
  - `internal/notify`
  - `internal/tui`
  - `test/e2e`
- `Makefile`:
  - `make test` → `go test ./...`
  - `make test-tmux` → `go test -tags=tmux ./...`
  - `make test-e2e` → `go test -tags=e2e,tmux ./test/e2e/...`
  - `make smoke` → `go test -tags=manual ./...`
  - `make build` → `go build -o bin/ssf ./cmd/ssf`
- `.gitignore`: `bin/`, `.ssf/`, `*.test`, `coverage.out`
- `.github/workflows/ci.yml` — runs `make test` on push.

## Acceptance

```
make build && make test
```

Both green. `./bin/ssf` prints `ssf` and exits 0.

## Notes

No business logic in this slice. Pure scaffolding so subsequent slices have
predictable file paths.
