package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func (s *Server) registerTools() {
	register(s.mcp, s.toolSearch)
	register(s.mcp, s.toolQuery)
	register(s.mcp, s.toolQueryRange)
	register(s.mcp, s.toolQueryExemplars)
	register(s.mcp, s.toolLabelNames)
	register(s.mcp, s.toolLabelValues)
	register(s.mcp, s.toolSeries)
	register(s.mcp, s.toolTargets)
	register(s.mcp, s.toolAlerts)
	register(s.mcp, s.toolRules)
	register(s.mcp, s.toolMetadata)
	register(s.mcp, s.toolTSDBStatus)
	register(s.mcp, s.toolAlertManagers)
	register(s.mcp, s.toolWalReplay)
	register(s.mcp, s.toolStatusConfig)
	register(s.mcp, s.toolStatusFlags)
	register(s.mcp, s.toolBuildInfo)
	register(s.mcp, s.toolRuntimeInfo)
}

// register adds a tool built by def to srv. It exists because Go cannot expand
// a two-value call into the three parameters of mcp.AddTool; the input type is
// inferred from def's handler, so each tool keeps its own typed arguments.
func register[In any](srv *mcp.Server, def func() (*mcp.Tool, mcp.ToolHandlerFor[In, any])) {
	tool, handler := def()
	mcp.AddTool(srv, tool, handler)
}

// noArgs is the input type for tools that take no parameters.
type noArgs struct{}

// readOnlyTool builds a tool definition annotated as a non-destructive,
// open-world read. Every tool in this server only reads from Prometheus.
func readOnlyTool(name, description string) *mcp.Tool {
	no, yes := false, true
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &no,
			OpenWorldHint:   &yes,
		},
	}
}

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

type seriesArgs struct {
	Matches []string `json:"matches" jsonschema:"One or more PromQL series selectors (e.g. ['up{job=\"prometheus\"}'])."`
	Start   string   `json:"start,omitempty" jsonschema:"Optional start timestamp (RFC3339 or Unix seconds)."`
	End     string   `json:"end,omitempty" jsonschema:"Optional end timestamp (RFC3339 or Unix seconds)."`
	Limit   *int     `json:"limit,omitempty" jsonschema:"Maximum number of series to return. Defaults to 500; 0 disables the cap."`
}

func (s *Server) toolSeries() (*mcp.Tool, mcp.ToolHandlerFor[seriesArgs, any]) {
	tool := readOnlyTool("prometheus_series", "Find time series matching the provided label selectors.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args seriesArgs) (*mcp.CallToolResult, any, error) {
		if len(args.Matches) == 0 {
			return nil, nil, fmt.Errorf("matches must contain at least one series selector")
		}
		start, end, err := parseOptionalRange(args.Start, args.End)
		if err != nil {
			return nil, nil, err
		}
		limit := boundedLimit(args.Limit, defaultListLimit)

		series, warnings, err := s.prom.API.Series(ctx, args.Matches, start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("series failed: %w", err)
		}
		out, total, truncated := truncateSlice(series, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

type targetsArgs struct {
	State string `json:"state,omitempty" jsonschema:"Which targets to return: 'active', 'dropped' or 'all'. Defaults to 'active'."`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of targets to return per state. Defaults to 200; 0 disables the cap."`
}

func (s *Server) toolTargets() (*mcp.Tool, mcp.ToolHandlerFor[targetsArgs, any]) {
	tool := readOnlyTool("prometheus_targets", "List Prometheus scrape targets.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args targetsArgs) (*mcp.CallToolResult, any, error) {
		limit := boundedLimit(args.Limit, defaultTargetsLimit)

		targets, err := s.prom.API.Targets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("targets failed: %w", err)
		}

		payload := map[string]any{}
		switch args.State {
		case "active", "":
			active, total, truncated := truncateSlice(targets.Active, limit)
			payload["active"] = active
			payload["active_total"] = total
			if truncated {
				payload["active_truncated"] = true
			}
		case "dropped":
			dropped, total, truncated := truncateSlice(targets.Dropped, limit)
			payload["dropped"] = dropped
			payload["dropped_total"] = total
			if truncated {
				payload["dropped_truncated"] = true
			}
		case "all":
			active, aTotal, aTrunc := truncateSlice(targets.Active, limit)
			dropped, dTotal, dTrunc := truncateSlice(targets.Dropped, limit)
			payload["active"] = active
			payload["active_total"] = aTotal
			payload["dropped"] = dropped
			payload["dropped_total"] = dTotal
			if aTrunc {
				payload["active_truncated"] = true
			}
			if dTrunc {
				payload["dropped_truncated"] = true
			}
		default:
			return nil, nil, fmt.Errorf("invalid state %q: want 'active', 'dropped' or 'all'", args.State)
		}
		return jsonResult(payload)
	}

	return tool, handler
}

func (s *Server) toolAlerts() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	tool := readOnlyTool("prometheus_alerts", "List currently firing and pending Prometheus alerts.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		alerts, err := s.prom.API.Alerts(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("alerts failed: %w", err)
		}
		return jsonResult(alerts)
	}

	return tool, handler
}

type rulesArgs struct {
	Type string `json:"type,omitempty" jsonschema:"Optional rule type filter: 'alert' or 'record'. Defaults to both."`
}

func (s *Server) toolRules() (*mcp.Tool, mcp.ToolHandlerFor[rulesArgs, any]) {
	tool := readOnlyTool("prometheus_rules", "List the Prometheus recording and alerting rule groups.")

	handler := func(ctx context.Context, _ *mcp.CallToolRequest, args rulesArgs) (*mcp.CallToolResult, any, error) {
		switch args.Type {
		case "", "all", "alert", "record":
		default:
			return nil, nil, fmt.Errorf("invalid type %q: want 'alert' or 'record'", args.Type)
		}

		rules, err := s.prom.API.Rules(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("rules failed: %w", err)
		}
		if args.Type == "" || args.Type == "all" {
			return jsonResult(rules)
		}
		return jsonResult(filterRules(rules, args.Type))
	}

	return tool, handler
}

// filterRules keeps only alerting or recording rules (per filter) and drops
// any group left empty. The Prometheus client returns Rules as []interface{}
// holding AlertingRule or RecordingRule values.
func filterRules(rules promv1.RulesResult, filter string) promv1.RulesResult {
	out := promv1.RulesResult{Groups: make([]promv1.RuleGroup, 0, len(rules.Groups))}
	for _, g := range rules.Groups {
		kept := make(promv1.Rules, 0, len(g.Rules))
		for _, r := range g.Rules {
			switch r.(type) {
			case promv1.AlertingRule, *promv1.AlertingRule:
				if filter == "alert" {
					kept = append(kept, r)
				}
			case promv1.RecordingRule, *promv1.RecordingRule:
				if filter == "record" {
					kept = append(kept, r)
				}
			default:
				kept = append(kept, r)
			}
		}
		if len(kept) > 0 {
			ng := g
			ng.Rules = kept
			out.Groups = append(out.Groups, ng)
		}
	}
	return out
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

// boundedLimit normalises a user-supplied limit. An absent or negative value
// falls back to def; 0 means unlimited and is preserved.
func boundedLimit(v *int, def int) int {
	if v == nil || *v < 0 {
		return def
	}
	return *v
}
