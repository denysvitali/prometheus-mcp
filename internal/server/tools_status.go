package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// The tools in this file all report Prometheus server state, take no arguments
// and render the API response verbatim, so each one is a single fetchTool call.

func (s *Server) toolTSDBStatus() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_tsdb_status",
		"Return TSDB cardinality statistics: head series count and the top series/labels by count and memory. Useful for spotting high-cardinality metrics and gauging TSDB size.",
		"tsdb status",
		// TSDB takes variadic options, so it needs a wrapper to match fetchTool.
		func(ctx context.Context) (promv1.TSDBResult, error) { return s.prom.API.TSDB(ctx) })
}

func (s *Server) toolAlertManagers() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_alertmanagers",
		"List the Alertmanager instances Prometheus currently knows about (active and dropped).",
		"alertmanagers", s.prom.API.AlertManagers)
}

func (s *Server) toolWalReplay() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_wal_replay",
		"Return the current WAL replay status (min, max, current segment).",
		"wal replay", s.prom.API.WalReplay)
}

func (s *Server) toolStatusConfig() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_status_config",
		"Return the currently loaded Prometheus configuration (YAML).",
		"config", s.prom.API.Config)
}

func (s *Server) toolStatusFlags() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_status_flags",
		"Return the command-line flags Prometheus was launched with.",
		"flags", s.prom.API.Flags)
}

func (s *Server) toolBuildInfo() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_buildinfo",
		"Return Prometheus server build information.",
		"buildinfo", s.prom.API.Buildinfo)
}

func (s *Server) toolRuntimeInfo() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	return fetchTool("prometheus_runtimeinfo",
		"Return Prometheus server runtime information (GOMAXPROCS, storage, etc).",
		"runtimeinfo", s.prom.API.Runtimeinfo)
}
