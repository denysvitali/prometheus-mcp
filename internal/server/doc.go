// Package server builds the MCP server for a Prometheus instance and registers
// the read-only tools that expose it.
//
// What belongs here: the MCP wiring (Server, transports, middleware), one file
// per group of related tools, the argument parsing those tools share (args.go)
// and the bounded rendering of their results (result.go). Every tool is a thin
// adapter: it validates arguments, calls the Prometheus client and renders the
// response. Anything that would be useful without MCP -- talking to Prometheus,
// ranking metrics -- belongs in internal/prometheus or internal/search instead.
//
// Concurrency: a Server is immutable after New and safe for concurrent use; the
// MCP SDK may run tool handlers in parallel, and the handlers hold no state of
// their own. The only goroutine the package starts is the metric-index
// refresher, which StartBackground owns: it stops when its context is cancelled
// and the returned function waits for it. ServeHTTP additionally runs
// http.Server.ListenAndServe in a goroutine whose result it always consumes.
package server
