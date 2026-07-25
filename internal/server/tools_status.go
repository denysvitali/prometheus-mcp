package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) toolTSDBStatus() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_tsdb_status",
		"Return TSDB cardinality statistics: head series count and the top series/labels by count and memory. Useful for spotting high-cardinality metrics and gauging TSDB size.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		status, err := s.prom.API.TSDB(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("tsdb status failed: %w", err)
		}
		return jsonResult(status)
	}

	return tool, handler
}

func (s *Server) toolAlertManagers() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_alertmanagers",
		"List the Alertmanager instances Prometheus currently knows about (active and dropped).")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		am, err := s.prom.API.AlertManagers(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("alertmanagers failed: %w", err)
		}
		return jsonResult(am)
	}

	return tool, handler
}

func (s *Server) toolWalReplay() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_wal_replay",
		"Return the current WAL replay status (min, max, current segment).")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		status, err := s.prom.API.WalReplay(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("wal replay failed: %w", err)
		}
		return jsonResult(status)
	}

	return tool, handler
}

func (s *Server) toolStatusConfig() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_status_config",
		"Return the currently loaded Prometheus configuration (YAML).")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		cfg, err := s.prom.API.Config(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("config failed: %w", err)
		}
		return jsonResult(cfg)
	}

	return tool, handler
}

func (s *Server) toolStatusFlags() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_status_flags",
		"Return the command-line flags Prometheus was launched with.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		flags, err := s.prom.API.Flags(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("flags failed: %w", err)
		}
		return jsonResult(flags)
	}

	return tool, handler
}

func (s *Server) toolBuildInfo() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_buildinfo", "Return Prometheus server build information.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		info, err := s.prom.API.Buildinfo(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("buildinfo failed: %w", err)
		}
		return jsonResult(info)
	}

	return tool, handler
}

func (s *Server) toolRuntimeInfo() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_runtimeinfo",
		"Return Prometheus server runtime information (GOMAXPROCS, storage, etc).")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		info, err := s.prom.API.Runtimeinfo(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("runtimeinfo failed: %w", err)
		}
		return jsonResult(info)
	}

	return tool, handler
}
