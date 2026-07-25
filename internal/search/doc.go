// Package search implements an in-memory BM25 index over Prometheus metric
// metadata so MCP clients can discover metrics by keyword or natural-language
// query instead of listing every series name.
//
// What belongs here: the index and its snapshot lifecycle (index.go), the
// ranking rules (score.go), and the refresher that rebuilds the index from the
// Prometheus API (refresh.go). The package knows nothing about MCP.
//
// Concurrency: an Index publishes one immutable snapshot at a time through an
// atomic pointer, so searches never block a rebuild and never observe a partial
// one; there is no lock. A Refresher performs one rebuild at a time from the
// goroutine that calls Run, which returns when its context is cancelled. See
// docs/adr/0001-immutable-snapshot-index.md.
package search
