package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type labelNamesArgs struct {
	Matches []string `json:"matches,omitempty" jsonschema:"Optional series selectors (e.g. ['up', 'process_cpu_seconds_total'])."`
	Start   string   `json:"start,omitempty" jsonschema:"Optional start timestamp (RFC3339 or Unix seconds)."`
	End     string   `json:"end,omitempty" jsonschema:"Optional end timestamp (RFC3339 or Unix seconds)."`
	Limit   *int     `json:"limit,omitempty" jsonschema:"Maximum number of label names to return. Defaults to 500; 0 disables the cap."`
}

func (s *Server) toolLabelNames() (*mcp.Tool, mcp.ToolHandlerFor[labelNamesArgs, any]) {
	tool := readOnlyTool("prometheus_label_names", "List label names present in the Prometheus TSDB.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args labelNamesArgs) (*mcp.CallToolResult, any, error) {
		start, end, err := parseOptionalRange(args.Start, args.End)
		if err != nil {
			return nil, nil, err
		}
		limit := boundedLimit(args.Limit, defaultListLimit)

		names, warnings, err := s.prom.API.LabelNames(ctx, args.Matches, start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("label names failed: %w", err)
		}
		out, total, truncated := truncateSlice(names, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

type labelValuesArgs struct {
	Label   string   `json:"label" jsonschema:"Name of the label (e.g. 'job', '__name__')."`
	Matches []string `json:"matches,omitempty" jsonschema:"Optional series selectors to filter the result."`
	Start   string   `json:"start,omitempty" jsonschema:"Optional start timestamp (RFC3339 or Unix seconds)."`
	End     string   `json:"end,omitempty" jsonschema:"Optional end timestamp (RFC3339 or Unix seconds)."`
	Limit   *int     `json:"limit,omitempty" jsonschema:"Maximum number of values to return. Defaults to 500; 0 disables the cap."`
}

func (s *Server) toolLabelValues() (*mcp.Tool, mcp.ToolHandlerFor[labelValuesArgs, any]) {
	tool := readOnlyTool("prometheus_label_values", "List the values of a given label.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args labelValuesArgs) (*mcp.CallToolResult, any, error) {
		start, end, err := parseOptionalRange(args.Start, args.End)
		if err != nil {
			return nil, nil, err
		}
		limit := boundedLimit(args.Limit, defaultListLimit)

		values, warnings, err := s.prom.API.LabelValues(ctx, args.Label, args.Matches, start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("label values failed: %w", err)
		}
		out, total, truncated := truncateSlice(values, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

type metadataArgs struct {
	Metric string `json:"metric,omitempty" jsonschema:"Metric name to filter by. Empty returns metadata for all metrics (bounded by limit)."`
	Limit  *int   `json:"limit,omitempty" jsonschema:"Maximum number of metrics to return when metric is empty. Defaults to 100; 0 disables the cap."`
}

func (s *Server) toolMetadata() (*mcp.Tool, mcp.ToolHandlerFor[metadataArgs, any]) {
	tool := readOnlyTool("prometheus_metadata", "Return metadata (type, help, unit) for ingested metrics.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args metadataArgs) (*mcp.CallToolResult, any, error) {
		limit := boundedLimit(args.Limit, defaultMetadataLimit)

		limitStr := ""
		if args.Metric == "" && limit > 0 {
			limitStr = strconv.Itoa(limit)
		}
		metadata, err := s.prom.API.Metadata(ctx, args.Metric, limitStr)
		if err != nil {
			return nil, nil, fmt.Errorf("metadata failed: %w", err)
		}
		return jsonResult(map[string]any{
			"data":  metadata,
			"count": len(metadata),
		})
	}

	return tool, handler
}
