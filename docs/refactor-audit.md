# Refactor audit — prometheus-mcp

Audited state: **working tree** on branch `improvements` (dirty: `README.md`,
`cmd/stdio.go`, `go.mod`, `go.sum`, `internal/server/{result,server,tools}.go`,
`internal/server/tools_handlers_test.go` modified; `internal/server/server_test.go`
untracked). All numbers below describe the working tree, not `HEAD`.

Date: 2026-07-24

## 1. Metrics baseline

| Metric | Value |
| --- | --- |
| Go LOC (total) | 2463 |
| Go LOC (non-test) | 1558 |
| Go LOC (test) | 905 |
| Go files (non-test) | 11 |
| Go files (test) | 6 |
| Packages | 5 (`main`, `cmd`, `internal/prometheus`, `internal/search`, `internal/server`) |
| Longest file | `internal/server/tools.go` — 546 |
| Files > 300 lines | 1 (`internal/server/tools.go`) |
| Files > 500 lines | 1 (`internal/server/tools.go`) |
| Functions > 40 lines | 5 (`Index.Search` 62, `Refresher.refreshOnce` 52, `toolTargets` 48, `shapeQueryResult` 45, `Index.Build` 45) |
| Functions > 60 lines | 1 (`Index.Search`) |
| Max cyclomatic (`gocyclo`) | 15 — `Index.Search` |
| Max cognitive (`gocognit`) | 20 — `Index.Search` |
| Functions with cognitive > 10 (non-test) | 6 (`Index.Search` 20, `toolTargets` 16, `filterRules` 16, `Index.Build` 12, `toolQueryRange` 12, `refreshOnce` 11) |
| `sync.Mutex` / `RWMutex` | 1 (`search.Index.mu`, RWMutex) |
| `sync.WaitGroup` | 0 |
| bare `go ` statements | 2 (`server.go:87` refresher, `server.go:114` `ListenAndServe`) |
| `dupl -threshold 60` clone groups | 7 (6 of them the eight no-arg tool constructors in `tools.go`; 1 in `fake_api_test.go`) |
| `golangci-lint run` (current config) | 0 issues |
| `go vet ./...` | clean |
| `go build ./...` | clean |
| `go test ./...` | pass |
| `go test -race ./...` | pass |
| Coverage | `internal/prometheus` 91.7%, `internal/search` 89.8%, `internal/server` 64.5%, `cmd` 0%, `main` 0% |
| `goleak` | not used |

`gocognit`/`gocyclo` output over threshold 8 is included above; `dupl` was run
over the explicit file list (`dupl ./...` does not resolve the Go pattern).

## 2. Findings

Severity: **H** = blocks a future change or hides a real defect, **M** = costs the
reader real effort, **L** = polish.

| path:line | issue | category | sev | proposed action | risk | est. size |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/server/result.go:158` | `parseTimeArg` falls back to `fmt.Sscanf(s, "%f", …)`, which accepts a *prefix*. `"2024-01-02"` and `"2024-01-02T03:04:05"` (no zone) silently become `1970-01-01T00:33:44Z`; `"5abc"` becomes 5s past the epoch. A user asking for a query at a plausible-looking timestamp gets 1970 data with no error. | footgun (bug) | **H** | Fix separately with `strconv.ParseFloat` (whole-string) in a `fix:` commit + table test | low, but it is a **behaviour change** so it must not ride along with a refactor | S |
| `internal/search/refresh.go:16` | `Refresher` is an exported struct whose zero value is invalid: `Run` calls `time.NewTicker(r.Interval)` which **panics** on `Interval <= 0`, and `refreshOnce` nil-derefs `Logger`. Today only `server.StartBackground` constructs it and happens to guard the interval. | footgun | **H** | Unexport the fields behind `NewRefresher(cfg RefresherConfig) (*Refresher, error)` that validates interval/logger/API/index; keep defaults for `Timeout`/`Lookback` | low — one caller | S |
| `internal/server/server.go:76,87` | `StartBackground` starts a goroutine with no way to wait for its exit and no error path — `go` statement without a shutdown handle (§7.6). Process exit currently masks it; a test cannot observe a clean stop and `goleak` would flag it. | concurrency | **H** | Return a `func()`/`Wait` handle (or an `errgroup`) from `StartBackground`; have `Refresher.Run` close a `done` channel on exit. **BREAKING** (exported signature) | low; both callers are in `cmd/` | S |
| `internal/server/tools.go:336–537` | Eight no-arg tools (`alerts`, `tsdb_status`, `alertmanagers`, `wal_replay`, `status_config`, `status_flags`, `buildinfo`, `runtimeinfo`) are byte-for-byte identical mechanics varying only name, description and the API method — this is the entire `dupl` report (6 clone groups). ~110 lines encoding one rule eight times. | duplication (structural boilerplate) | **H** | One generic helper `simpleTool[T](name, desc string, fetch func(context.Context) (T, error))`; each tool becomes a 3-line registration | low — same output shape, covered by the session test | S |
| `internal/server/tools.go` (546 lines) | Single file holds the registry, 18 tool definitions, arg types, and `filterRules`. Over the 500-line must-fix line; a change to one tool forces reading all of them. | structure | **H** | Split by domain: `tools.go` (registry + `readOnlyTool` + `boundedLimit`), `tools_query.go` (search/query/query_range/exemplars), `tools_series.go` (label names/values/series/metadata), `tools_admin.go` (targets/alerts/rules + `filterRules`), `tools_status.go` (the generic no-arg group). Pure `git mv`-style moves, no content edits | very low | M |
| `internal/search/index.go:132` | `Index.Search` is 62 lines, cognitive 20, cyclomatic 15: locking, tokenizing, scoring, phrase boost, filtering, sorting and hit building in one function. | complexity | **H** | Split into `snapshot` read + pure `score(tokens) map[int]float64`, `rank(scores, typeFilter) []int`, `hits(ids, limit)`. Each becomes table-testable without a lock | low — behaviour-preserving, 89.8% covered; add characterization tests for ties/limit<=0/type filter first | M |
| `internal/search/index.go:33` | `RWMutex` guarding six fields that are always replaced *together* by `Build` — a textbook read-mostly snapshot (§7.4). Readers contend on a lock for data that never mutates in place. | concurrency | **M** | Immutable `snapshot` struct + `atomic.Pointer[snapshot]`; `Build` constructs and swaps. Removes the only lock in the repo, and makes "which fields are consistent with which" unrepresentable-if-wrong | low; `-race` + a concurrent Build/Search test | M |
| `internal/server/tools.go:287` | `toolTargets` is 48 lines, cognitive 16: three `switch` arms each hand-rolling the same truncate-and-name-keys logic with different key prefixes. | complexity + duplication | **M** | Extract `putTargets(payload map[string]any, prefix string, list []T, limit int)`; the switch collapses to three calls + a default error | low — behaviour identical, keys unchanged | S |
| `internal/server/tools.go:380` | `filterRules` — cognitive 16, nested switch inside two loops, and the `default:` arm silently keeps unknown rule kinds (intentional but undocumented as a decision). | complexity | **M** | Extract `keepRule(r any, filter string) bool` (pure, table-testable); document the `default: keep` policy in one line | low | S |
| `cmd/http.go:19–41`, `cmd/stdio.go:19–37` | True duplication: build client → signal context → `maybePing` → `server.New` → `StartBackground` is the same startup rule in both commands and must change together (it already drifted once — `stdio.go` is modified in the working tree while `http.go` is not). | duplication | **M** | Extract `func newRuntime(ctx) (*server.Server, error)` (or `runWith(func(*server.Server) error)`) in `cmd/`; each command keeps only its transport line | low | S |
| `cmd/root.go:14–18`, `cmd/ping.go:18`, `internal/prometheus/client.go:26` | Config is read from the global `viper` singleton at three depths, including inside `maybePing`. Hidden global input: no call site shows what it depends on, and nothing fails fast on a bad value except `log-level`. | footgun | **M** | Bind flags once, decode into an explicit `Config` struct in `root.go`, validate there, pass it down. `prometheus.NewFromViper` becomes `prometheus.New(Config)`; viper stays confined to `cmd/`. **BREAKING** (exported `NewFromViper`) | medium — touches every command; add a config-decoding test | M |
| `cmd/ping.go` | File named after a command that does not exist; it holds one helper (`maybePing`) that reads a global flag. Borderline dumping ground (§4.1). | structure | **L** | Fold into the startup helper from the `cmd/http.go`/`stdio.go` de-duplication; delete the file | low | S |
| `internal/server/result.go:94–98` | `_ = total` — dead assignment left from the vector branch; `stats.SeriesTotal` is set from `len(v)` just above. | complexity (dead code) | **L** | Drop the unused return with `_` | none | S |
| `internal/server/result.go:158`, `:167` | `time.Now()` called deep in the parse path, so "empty means now" is only testable with a 1-second tolerance (see `TestParseTimeArgEmptyUsesNow`). | footgun (testability) | **L** | Inject a clock (`now func() time.Time` field on `Server`, defaulting to `time.Now`) when the `parseTimeArg` fix lands | low | S |
| `internal/search/index.go:64` | `out := parts[:0]` filters in place, aliasing the slice returned by `regexp.Split`. Correct, but non-obvious enough to need the one-line "why" (allocation avoidance). | docs | **L** | One comment | none | XS |
| `cmd/root.go:15,62` | `logLevel` is bound with `StringVar` **and** via `viper.BindPFlag`; only the viper path is ever read, so the variable is a second, dead source of truth (same shape as `cfgFile`, which *is* read). | footgun | **L** | Drop the `StringVar` binding, use `String(...)` | none | XS |
| `internal/server/` (5 files, no `doc.go`) | Package doc lives in `server.go`; the package is over 3 files (§5.2) and has no statement of what belongs in it or its concurrency model. | docs | **L** | Add `internal/server/doc.go`; same for `internal/search` (4 files) | none | S |
| `.golangci.yml` | Enables `standard` + 3 linters. None of the invariants this audit establishes (function length, cognitive complexity, nesting, duplication, `noctx`, `errorlint`, `paralleltest`, lock policy) are enforced, so every fix below can silently regress. | structure | **M** | Tighten per §Phase C, then fix the fallout | medium — will surface new warnings in test files (`paralleltest`, `thelper`) | M |
| CI (`.github/workflows/ci.yml`) | Runs `vet`, `test -race`, `build`, lint. No coverage floor, no `goleak`, no `gofumpt` check. | structure | **L** | Add `goleak` in `TestMain` for `internal/search` + `internal/server`; leave coverage gating out unless wanted | low | S |
| `cmd/` (0% coverage) | Flag/env/config wiring — the part users hit first — is untested; the `PROMETHEUS_MCP_*` mapping in the README is verified by nothing. | test-coverage | **M** | Table test asserting flag → viper key → env var for every row of the README table (this also pins the doc) | low | S |

### Not changing (coincidental similarity)

- `bearerAuthTransport` and `basicAuthTransport` (`internal/prometheus/client.go:92,103`)
  share a shape but encode two independent auth schemes with different headers.
  Leave; annotate as independent.
- The per-tool `args` structs look repetitive but each is a distinct wire schema
  with its own JSON-schema docs. Merging them would couple unrelated tools.
- `defaultQueryMaxSeries` … `defaultMetadataLimit` are flat, independent policy
  constants that happen to be integers. Leave as a block.

## 3. Proposed target tree (path diff)

```
  cmd/
    root.go              # flags + Config decode/validate (was: + globals)
+   config.go            # Config struct, decode, validation, table-tested
    http.go              # transport line only
    stdio.go             # transport line only
-   ping.go              # folded into cmd/runtime.go
+   runtime.go           # shared startup: client -> ping -> server -> background
  internal/prometheus/
    client.go
+   transport.go         # newRoundTripper + the two auth transports
    client_test.go
  internal/search/
+   doc.go
    index.go             # Index + snapshot swap (was: + scoring internals)
+   score.go             # pure BM25 scoring/ranking, no lock, table-tested
    refresh.go           # NewRefresher + Run/refreshOnce
    index_test.go
+   score_test.go
    refresh_test.go
  internal/server/
+   doc.go
    server.go            # Server, New, transports, lifecycle
    result.go            # rendering + bounds (unchanged)
    tools.go             # registry + readOnlyTool + boundedLimit only
+   tools_query.go       # search, query, query_range, exemplars
+   tools_series.go      # label_names, label_values, series, metadata
+   tools_admin.go       # targets, alerts, rules, filterRules
+   tools_status.go      # the eight no-arg tools via one generic helper
    …_test.go            # split to match
+ docs/refactor-audit.md
+ docs/refactor-report.md
+ docs/adr/0001-snapshot-index-instead-of-rwmutex.md
```

Dependency direction is already one-way (`cmd` → `server` → {`prometheus`,
`search`}) with no cycles; no package moves are needed, only file splits.

## 4. Concurrency inventory

| site | what | disposition |
| --- | --- | --- |
| `internal/search/index.go:34` `Index.mu` (RWMutex, 6 fields) | Read-mostly index replaced wholesale by `Build` | **Convert** to immutable snapshot + `atomic.Pointer` (§7.4). Removes the lock; ADR-0001 |
| `internal/server/server.go:87` `go refresher.Run(ctx)` | Index refresher, exits on ctx cancel | **Keep the goroutine, add a handle**: `Run` closes `done`; `StartBackground` returns a wait/stop func. Currently no owner can observe exit |
| `internal/server/server.go:114` `go func(){ errCh <- httpSrv.ListenAndServe() }()` | Buffered `errCh` (cap 1), drained on both paths | **Keep as-is.** Correct and cannot leak; already documented |
| `internal/search/refresh.go:31` ticker | `time.NewTicker` + `defer Stop()` | Correct; panics on `Interval <= 0` — covered by the `NewRefresher` finding |

No `WaitGroup`, no channel-as-mutex, no unbounded queues. `-race` is green.

## 5. Doc drift

Verified against the working tree.

**True today (leave alone):** the 18-row tool table matches `registerTools`
exactly; every bound in the "Bounded output" table matches the constants in
`result.go`; every flag in the env-var table exists in `cmd/`; the
`__name__`-fallback paragraph matches `refresh.go`; `go install …@latest` works
against `main.go` at the module root; the goreleaser `-X …/internal/server.Version`
path matches `server.Version`; `go test/vet/build ./...` all pass as documented.

**Corrections needed:**

1. `README.md:41` — "a BM25 index built in-process from the `/api/v1/metadata`
   endpoint" is only half the source; the very next paragraph explains the
   `__name__` fallback. Reword to "from `/api/v1/metadata` **and** the
   `__name__` label values" so the first sentence is not contradicted by the
   second.
2. `README.md:91` — "for every tag" is false: `docker.yml` triggers on
   `tags: ["v*.*.*"]` only, so a non-semver tag publishes nothing. (`:latest`
   on the default branch and the two platforms are correct.) Reword to
   "every `v*.*.*` tag".
3. `README.md:177` (Development) omits `go test -race ./...` and
   `golangci-lint run`, both of which CI gates on. A contributor following the
   README can pass locally and fail CI.
4. `README.md:78` — `go install` does not set `-ldflags`, so an installed
   binary reports `Version = "dev"`. Not wrong, but worth one line since the
   README documents no `version` command at all (there isn't one — `Version` is
   only sent in the MCP handshake).
5. `internal/prometheus/client.go:24` — `NewFromViper`'s doc lists the viper
   keys it reads; it omits nothing today, but it will go stale the moment a key
   is added. The `Config` struct from finding §2 makes the list compile-checked
   instead.
6. `internal/server/tools.go:34` — the `register` doc explains a Go limitation
   accurately; keep, it is exactly the kind of "why" comment §5.4 wants.

No false statements found beyond (1) and the unverified (2).

## 6. Ordered plan

Each numbered item is one commit, independently green
(`go build ./... && go vet ./... && go test ./... && go test -race ./... && golangci-lint run`).

**Prerequisite:** commit or stash the current working-tree changes first, so
every refactor diff below is reviewable in isolation.

1. `test:` characterization tests before anything moves — `Index.Search`
   (tie-break order, `limit <= 0`, type filter, empty query), `filterRules`
   (unknown rule kinds kept), `toolTargets` (all three states + truncation
   keys), `parseTimeArg` (pin **today's** prefix-parsing behaviour, so the
   later `fix:` commit shows the change explicitly), `cmd` flag→viper→env table.
2. `chore:` deletions — `_ = total` (`result.go:97`), the dead `logLevel`
   `StringVar`.
3. `refactor:` split `internal/server/tools.go` into the five files above.
   Moves only, no content edits.
4. `refactor:` collapse the eight no-arg tools onto one generic `simpleTool`
   helper (kills the whole `dupl` report).
5. `refactor:` complexity — `toolTargets` (`putTargets` helper) and
   `filterRules` (`keepRule` predicate).
6. `refactor:` complexity — split `Index.Search` into score/rank/hits.
7. `refactor:` concurrency — `search.Index` snapshot + `atomic.Pointer`,
   with a concurrent Build/Search `-race` test and ADR-0001.
8. `refactor:` **BREAKING** — `search.NewRefresher` constructor (validated,
   unexported fields) + `StartBackground` returns a wait handle;
   `goleak` in `TestMain` for `internal/search` and `internal/server`.
9. `refactor:` **BREAKING** — `cmd` config struct: bind once, validate at
   startup, `prometheus.New(Config)` replaces `NewFromViper`; fold `ping.go`
   into `runtime.go` and de-duplicate `http.go`/`stdio.go`.
10. `refactor:` split `internal/prometheus/transport.go` out of `client.go`;
    annotate the two auth transports as independently-evolving.
11. `docs:` `doc.go` for `internal/server` and `internal/search`; concurrency
    contract comments; README corrections 1–4; the `tokenize` aliasing comment.
12. `ci:` tighten `.golangci.yml` (§Phase C) and fix fallout; add
    `gofumpt -l` and the `goleak` job wiring.
13. `docs:` `docs/refactor-report.md` with before/after metrics.

**Separately, not part of the refactor** (§2.1 — behaviour change):

- `fix:` `parseTimeArg` whole-string numeric parsing. Recommend landing this
  **after** commit 1 (which pins the current behaviour) and **before** the rest,
  so the flip is a two-file diff.

### `BREAKING:`

- `search.Refresher`'s exported fields → `search.NewRefresher(RefresherConfig)`.
- `(*server.Server).StartBackground` gains a return value (wait/stop handle).
- `prometheus.NewFromViper(*viper.Viper)` → `prometheus.New(prometheus.Config)`.

All three are `internal/` packages with in-repo callers only, so no external
consumer can break; they are listed because they change exported signatures.

## 7. Deliberately out of scope

- `Index.scoreTerm` walks the **entire vocabulary** per query token to support
  prefix matching (`index.go:201`). That is O(vocab × postings) per query and
  the real scaling limit of the search tool, but fixing it means changing the
  data structure (e.g. a trie or a prefix-term map) and possibly the ranking.
  Performance work with a benchmark, not a complexity refactor — flagged, not
  touched.
- No new dependencies are proposed except `go.uber.org/goleak` (test-only,
  commit 8): it replaces nothing, and stdlib has no goroutine-leak detector.
  `golang.org/x/sync/errgroup` is **not** needed — there is one background
  goroutine.
