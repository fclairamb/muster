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

## Implementation Plan

1. `go mod init github.com/fclairamb/ssf` (Go 1.22 minimum, current toolchain 1.26).
2. Create `cmd/ssf/main.go` printing `ssf`.
3. Create `doc.go` in every internal/* package + `test/e2e/`.
4. Write `Makefile` with `test`, `test-tmux`, `test-e2e`, `smoke`, `build`, `fmt` targets.
5. Write `.gitignore` (`bin/`, `.ssf/`, `*.test`, `coverage.out`, `.claude/`).
6. Write `.github/workflows/ci.yml` running `make test`.
7. QA: `go build ./... && go vet ./... && go test ./...`.

