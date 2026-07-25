# Refactor report — prometheus-mcp

Baseline: the working tree audited in `docs/refactor-audit.md`, committed as
`chore: snapshot in-progress working tree as refactor baseline`.
Final: `ci: enforce the complexity, context and test-hygiene budgets`.

## Before / after

| Metric | Before | After |
| --- | --- | --- |
| Non-test LOC | 1558 | 1856 |
| Test LOC | 905 | 1711 |
| Longest file (any) | 546 (`internal/server/tools.go`) | 273 (a test file) |
| Longest non-test file | 546 | 184 (`internal/search/refresh.go`) |
| Files over 300 / 500 lines (non-test) | 1 / 1 | 0 / 0 |
| Non-test functions over 40 / 60 lines | 5 / 1 | 0 / 0 |
| Max cognitive complexity (non-test) | 20 (`Index.Search`) | 9 (`scoreTerm`) |
| Non-test functions over cognitive 10 | 6 | 0 |
| Max cyclomatic complexity (non-test) | 15 | < 9 |
| `sync.Mutex` / `RWMutex` / `WaitGroup` | 1 / 0 | 0 / 0 |
| `go` statements (non-test) | 2, neither with a wait path | 2, both owned and waited for |
| `dupl -threshold 60` groups in non-test code | 6 | 0 |
| `golangci-lint` linters enabled beyond standard | 3 | 20 |
| `golangci-lint run` | 0 issues | 0 issues |
| `goleak` | not used | enforced in `internal/search` and `internal/server` |
| Coverage: `cmd` | 0.0% | 48.1% |
| Coverage: `internal/search` | 89.8% | 96.8% |
| Coverage: `internal/server` | 64.5% | 76.9% |
| Coverage: `internal/prometheus` | 91.7% | 90.9% |

Non-test LOC grew by ~300 lines. Roughly 120 of those are doc comments and three
`doc.go` files; the rest is the configuration struct, the refresher constructor
and the shutdown plumbing that replaced implicit behaviour. The tool definitions
themselves shrank (`tools_status.go`: 112 → 55).

`internal/prometheus` coverage dips 0.8pp because `New` has one more early-return
branch than `NewFromViper` did; the package gained a test (bearer beats basic),
and no test was deleted anywhere in this work.

## What changed, by objective

**Correctness preserved.** Every commit is green on
`go build`, `go vet`, `go test`, `go test -race` and `golangci-lint run`. Tool
names, descriptions, JSON payload keys and error strings are byte-identical
except where a commit says otherwise. Verified end to end against a fake
Prometheus: an MCP session over stdio lists all 18 tools, `prometheus_buildinfo`
round-trips, and the HTTP transport answers `POST /mcp` with 200 and drains on
SIGTERM.

**One bug found and fixed separately.** `parseTimeArg` accepted a numeric
*prefix*, so `"2024-01-02"` silently became 1970. Behaviour
was pinned in the `test:` commit, then flipped in a standalone `fix:` commit.

**Footguns removed.**
- `search.Refresher`'s zero value used to panic in `time.NewTicker` and
  nil-deref its logger; `NewRefresher` now validates and defaults.
- `StartBackground`'s goroutine had no owner; it returns a wait function, and
  both commands cancel and wait before exiting.
- `ServeHTTP(ctx, addr, path, stateless)` became `ServeHTTP(ctx, HTTPOptions{…})`
  — no more swappable strings or bare boolean at the call site.
- Configuration is no longer read from the global viper singleton three levels
  deep; `loadConfig` resolves it once and `cmd/doc.go` records that cmd is the
  only package allowed to touch viper.

**Complexity reduced.** `Index.Search` (62 lines, cognitive 20) became four
named steps in `score.go`. `toolTargets`, `filterRules`, `toolQueryRange`,
`shapeQueryResult` and `refreshOnce` all dropped below the threshold.
`tools.go` (546 lines) split into five files by tool domain, `result.go` split
from `args.go`, and `client.go` from `transport.go`.

**Truthful docs.** Four README statements were corrected (index sources, GHCR
tag trigger, the commands CI runs, the version of a `go install` binary), a
Layout section documents the tree and the one-way dependency direction, and
three `doc.go` files state what belongs in each package and its concurrency
model. Every README command was executed except the Docker ones — no container
runtime is available here, so those two blocks are unverified.

**Duplication removed.** The eight parameterless tools now share
`fetchTool[T]`, which eliminated the whole non-test `dupl` report.
`cmd/http.go` and `cmd/stdio.go` shared startup path moved into `cmd/runtime.go`
(they had already drifted). Coincidental similarity was left alone and
annotated: the two auth round trippers, the per-tool argument structs, the
output-cap constants.

**Concurrency.** The only lock in the repository is gone: `search.Index`
publishes an immutable snapshot through `atomic.Pointer`
(`docs/adr/0001-immutable-snapshot-index.md`), exercised by a `-race` test that
checks a hit can never mix generations. Both remaining `go` statements have a
documented owner and a guaranteed exit, and `goleak` fails the tests if either
leaks.

## `BREAKING:`

All three are `internal/` packages, so every caller is in this repository.

- `search.Refresher`'s exported fields → `search.NewRefresher(search.RefresherConfig)`.
- `(*server.Server).StartBackground(ctx)` → returns `(wait func(), err error)`.
- `prometheus.NewFromViper(*viper.Viper)` → `prometheus.New(prometheus.Config)`.
- `(*server.Server).ServeHTTP(ctx, addr, path, stateless)` → `ServeHTTP(ctx, server.HTTPOptions)`.

## Bugs found but not fixed

None outstanding. The `parseTimeArg` prefix-parsing bug was fixed in its own
`fix:` commit. The other finding -- `search.Refresher`'s zero value panicking in
`time.NewTicker` -- was unreachable in practice (its only caller guarded the
interval), so it was addressed by the validating constructor rather than a
`fix:`.

Two observations that are not bugs and were deliberately left alone:

- A signal-initiated shutdown exits with status 1, because the transport returns
  the cancelled context's error. Changing that is a behaviour change; worth a
  decision, not a silent edit.
- `prometheus_search` reports "index is not ready yet" when
  `--search-refresh-interval 0` disables refreshing, which is accurate but reads
  like a transient condition.

## Deferred

- **`scoreTerm` scans the whole vocabulary per query token**
  (`internal/search/score.go`). That is the real scaling limit of the search
  tool — O(vocabulary × postings) per query. Fixing it means a different data
  structure (prefix trie or a term-prefix map) and possibly different ranking,
  so it needs a benchmark and its own change, not a refactor.
- **`internal/server/tools_handlers_test.go` (273 lines)** is the longest file
  left. It holds the shared test harness plus handler tests spanning three tool
  groups; splitting it is easy but would move code the same week it was moved
  once already, so it is left for whoever next touches those tests.
- **A coverage floor in CI** was not added: at 48% for `cmd` a floor would
  either be meaningless or immediately block unrelated work. The `cmd` tests
  that matter (the documented flag/env mapping) now exist regardless.

## Dependencies

One added: `go.uber.org/goleak` (test-only). It replaces nothing; the standard
library has no goroutine-leak detector, and the alternative was to trust that
the new shutdown paths work.
