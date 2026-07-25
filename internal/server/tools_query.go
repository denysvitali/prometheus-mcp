package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type searchArgs struct {
	Query string `json:"query" jsonschema:"Keyword or natural-language query (e.g. 'http request latency', 'node memory free', 'kube pod status'). Partial metric-name prefixes like 'http_req' also match."`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of hits to return. Defaults to 20."`
	Type  string `json:"type,omitempty" jsonschema:"Optional metric type filter (counter, gauge, histogram, summary, unknown)."`
}

func (s *Server) toolSearch() (*mcp.Tool, mcp.ToolHandlerFor[searchArgs, any]) {
	tool := readOnlyTool("prometheus_search",
		"Search the Prometheus metric catalogue (names, help text, units) by keyword or natural-language query. Use this first to discover relevant metrics before running queries.")

	handler := func(_ context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
		limit := boundedLimit(args.Limit, 20)
		if s.index.Size() == 0 {
			return nil, nil, fmt.Errorf("metric index is not ready yet; wait a few seconds after startup or check server logs for refresh errors")
		}
		hits := s.index.Search(args.Query, limit, args.Type)
		return jsonResult(map[string]any{
			"query":           args.Query,
			"results":         hits,
			"result_count":    len(hits),
			"indexed_metrics": s.index.Size(),
			"index_updated":   s.index.UpdatedAt().UTC().Format(time.RFC3339),
		})
	}

	return tool, handler
}

type queryArgs struct {
	Query          string   `json:"query" jsonschema:"PromQL expression to evaluate (e.g. 'up', 'rate(http_requests_total[5m])')."`
	Time           string   `json:"time,omitempty" jsonschema:"Evaluation timestamp, RFC3339 or Unix seconds. Defaults to server time."`
	TimeoutSeconds *float64 `json:"timeout_seconds,omitempty" jsonschema:"Optional query timeout in seconds."`
	MaxSeries      *int     `json:"max_series,omitempty" jsonschema:"Maximum number of series to return. Defaults to 100; 0 disables the cap."`
}

func (s *Server) toolQuery() (*mcp.Tool, mcp.ToolHandlerFor[queryArgs, any]) {
	tool := readOnlyTool("prometheus_query", "Evaluate a PromQL instant query against Prometheus.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args queryArgs) (*mcp.CallToolResult, any, error) {
		ts, err := parseTimeArg(args.Time)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid time: %w", err)
		}
		maxSeries := boundedLimit(args.MaxSeries, defaultQueryMaxSeries)

		value, warnings, err := s.prom.API.Query(ctx, args.Query, ts, queryOptions(args.TimeoutSeconds)...)
		if err != nil {
			return nil, nil, fmt.Errorf("query failed: %w", err)
		}
		return queryResultWithWarnings(value, maxSeries, 0, warnings)
	}

	return tool, handler
}

type queryRangeArgs struct {
	Query               string   `json:"query" jsonschema:"PromQL expression to evaluate."`
	Start               string   `json:"start" jsonschema:"Start timestamp (RFC3339 or Unix seconds)."`
	End                 string   `json:"end" jsonschema:"End timestamp (RFC3339 or Unix seconds)."`
	Step                string   `json:"step" jsonschema:"Resolution step as a Go duration (e.g. '15s', '1m', '5m')."`
	TimeoutSeconds      *float64 `json:"timeout_seconds,omitempty" jsonschema:"Optional query timeout in seconds."`
	MaxSeries           *int     `json:"max_series,omitempty" jsonschema:"Maximum number of series to return. Defaults to 50; 0 disables the cap."`
	MaxSamplesPerSeries *int     `json:"max_samples_per_series,omitempty" jsonschema:"Maximum number of samples kept per series. Defaults to 100; 0 disables the cap."`
}

func (s *Server) toolQueryRange() (*mcp.Tool, mcp.ToolHandlerFor[queryRangeArgs, any]) {
	tool := readOnlyTool("prometheus_query_range", "Evaluate a PromQL query over a time range.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args queryRangeArgs) (*mcp.CallToolResult, any, error) {
		start, err := parseTimeArg(args.Start)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start: %w", err)
		}
		end, err := parseTimeArg(args.End)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end: %w", err)
		}
		step, err := time.ParseDuration(args.Step)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid step: %w", err)
		}
		if step <= 0 {
			return nil, nil, fmt.Errorf("step must be a positive duration")
		}
		if !end.After(start) {
			return nil, nil, fmt.Errorf("end must be after start")
		}
		maxSeries := boundedLimit(args.MaxSeries, defaultRangeMaxSeries)
		maxSamples := boundedLimit(args.MaxSamplesPerSeries, defaultRangeMaxSamplesPerSeries)

		r := promv1.Range{Start: start, End: end, Step: step}
		value, warnings, err := s.prom.API.QueryRange(ctx, args.Query, r, queryOptions(args.TimeoutSeconds)...)
		if err != nil {
			return nil, nil, fmt.Errorf("range query failed: %w", err)
		}
		return queryResultWithWarnings(value, maxSeries, maxSamples, warnings)
	}

	return tool, handler
}

type queryExemplarsArgs struct {
	Query string `json:"query" jsonschema:"PromQL expression selecting the exemplars to return."`
	Start string `json:"start" jsonschema:"Start timestamp (RFC3339 or Unix seconds)."`
	End   string `json:"end" jsonschema:"End timestamp (RFC3339 or Unix seconds)."`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of series with exemplars to return. Defaults to 500; 0 disables the cap."`
}

func (s *Server) toolQueryExemplars() (*mcp.Tool, mcp.ToolHandlerFor[queryExemplarsArgs, any]) {
	tool := readOnlyTool("prometheus_query_exemplars",
		"Query exemplars (e.g. trace IDs attached to histogram buckets) over a time range.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args queryExemplarsArgs) (*mcp.CallToolResult, any, error) {
		start, err := parseTimeArg(args.Start)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start: %w", err)
		}
		end, err := parseTimeArg(args.End)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end: %w", err)
		}
		limit := boundedLimit(args.Limit, defaultListLimit)

		exemplars, err := s.prom.API.QueryExemplars(ctx, args.Query, start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("exemplars query failed: %w", err)
		}
		out, total, truncated := truncateSlice(exemplars, limit)
		return listResult(out, total, len(out), truncated, nil)
	}

	return tool, handler
}
