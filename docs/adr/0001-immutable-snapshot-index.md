# ADR-0001: the metric index is an immutable snapshot behind an atomic pointer

- Status: accepted
- Date: 2026-07-24

## Context

`search.Index` serves `prometheus_search`. It is written by exactly one
goroutine — the refresher, every `--search-refresh-interval` — and read by every
tool call. A rebuild replaces *all* of its state at once: documents, postings,
document frequencies, document lengths, the average length and the timestamp are
only meaningful as a set from the same generation.

The original implementation guarded those six fields with a `sync.RWMutex`.
That was correct, but it left two things for every reader to verify by hand:
that no code path reads one field outside the lock, and that no field is mutated
in place after publication. Both are invisible in a diff.

## Decision

`Index` holds a single `atomic.Pointer[snapshot]`. `snapshot` is written exactly
once, by `newSnapshot`, and never mutated. `Build` constructs the new generation
off to the side and installs it with one atomic store; every reader loads the
pointer once and then works on data that cannot change underneath it.

`Build` therefore takes ownership of the `[]Document` it is given, which its doc
comment states.

## Consequences

- The concurrency contract fits in one sentence: readers see one immutable
  generation, chosen at the moment they load the pointer.
- No lock, so no lock ordering, no risk of a blocking call inside a critical
  section, and readers never block a rebuild (or each other).
- A reader that loaded the previous generation keeps using it until it returns.
  That is the same visibility a reader holding `RLock` had, and it is what
  `prometheus_search` wants: consistent results over a stale-by-seconds catalogue.
- Memory: two generations are live while a rebuild's snapshot is being installed
  and any in-flight reader finishes. For metric metadata (tens of thousands of
  short documents) that is a few MB, and it is bounded by one refresh at a time.
- `internal/search/index_concurrency_test.go` exercises the contract under
  `-race`: hits may never mix generations and `Size` may never report a partial
  one.
