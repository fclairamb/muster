# 02 — Slugs, repo info, org-prefix abbreviation

## Goal
Stable internal identifiers (never change) and pretty display names (may
change as new orgs are registered, but always derived deterministically).

## Deliverables

### `internal/slug`

- `Slug(absPath string) string` — first 12 chars of `sha256(absPath)`. Used
  for tmux session names (`ssf-<slug>`) and `.ssf/state/<slug>.json`.
- Stable across runs. Never includes the org abbreviation.

### `internal/repoinfo`

- `Inspect(dir string) (Info, error)`:
  ```go
  type Info struct {
      RepoRoot   string // git rev-parse --show-toplevel
      Branch     string // current branch (or "HEAD detached")
      IsGitHub   bool
      Org        string // "" if not GitHub
      Repo       string // "" if not GitHub
  }
  ```
- Remote selection: prefer `upstream`, fall back to `origin`.
- URL parsing handles both `git@github.com:org/repo.git` and
  `https://github.com/org/repo[.git]`.
- Non-git dirs are not an error: returns `Info{RepoRoot: dir}` with empty
  branch and `IsGitHub=false`.

Tests use `t.TempDir()` + `git init` + `git remote add` to build fixtures —
no network.

### `internal/orgprefix`

- `Derive(orgs []string, overrides map[string]string) map[string]string`
- Algorithm:
  1. Apply manual overrides first; mark those orgs as fixed.
  2. For each non-fixed org, start with prefix length 1.
  3. If two non-fixed orgs share the same prefix at length L, expand both
     to L+1. Repeat until all are unique among themselves AND none collide
     with a fixed override.
  4. Minimum prefix length: 1. Maximum: full org name (degenerate case).
- Pure function. No I/O.

## Tests (`go test ./internal/slug/... ./internal/repoinfo/... ./internal/orgprefix/...`)

Table-driven `orgprefix` cases:

| Input orgs                              | Overrides            | Expected                                          |
|-----------------------------------------|----------------------|---------------------------------------------------|
| `[stonal-tech]`                         | `{}`                 | `{stonal-tech: s}`                                |
| `[stonal-tech, fclairamb]`              | `{}`                 | `{stonal-tech: s, fclairamb: f}`                  |
| `[stonal-tech, some-org]`               | `{}`                 | `{stonal-tech: st, some-org: so}`                 |
| `[microsoft, meta]`                     | `{microsoft: ms}`    | `{microsoft: ms, meta: m}`                        |
| `[microsoft, meta, mozilla]`            | `{}`                 | `{microsoft: mi, meta: me, mozilla: mo}`          |
| `[]`                                    | `{}`                 | `{}`                                              |

`slug` tests:

- Same path → same slug.
- Different paths → different slugs.
- Slug is 12 lowercase hex chars.

`repoinfo` tests:

- SSH URL parsing.
- HTTPS URL parsing (with and without `.git`).
- `upstream` wins over `origin`.
- Non-git dir → `IsGitHub=false`, no error.
- Detached HEAD → branch string is `"HEAD detached"`.

## Acceptance

```
go test ./internal/slug/... ./internal/repoinfo/... ./internal/orgprefix/...
```

Green.
