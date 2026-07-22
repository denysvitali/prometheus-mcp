package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func (s *Server) registerTools() {
	s.mcp.AddTool(s.toolSearch())
	s.mcp.AddTool(s.toolQuery())
	s.mcp.AddTool(s.toolQueryRange())
	s.mcp.AddTool(s.toolQueryExemplars())
	s.mcp.AddTool(s.toolLabelNames())
	s.mcp.AddTool(s.toolLabelValues())
	s.mcp.AddTool(s.toolSeries())
	s.mcp.AddTool(s.toolTargets())
	s.mcp.AddTool(s.toolAlerts())
	s.mcp.AddTool(s.toolRules())
	s.mcp.AddTool(s.toolMetadata())
	s.mcp.AddTool(s.toolTSDBStatus())
	s.mcp.AddTool(s.toolAlertManagers())
	s.mcp.AddTool(s.toolWalReplay())
	s.mcp.AddTool(s.toolStatusConfig())
	s.mcp.AddTool(s.toolStatusFlags())
	s.mcp.AddTool(s.toolBuildInfo())
	s.mcp.AddTool(s.toolRuntimeInfo())
}

func readOnly(desc string) []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithDescription(desc),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
	}
}

func (s *Server) toolSearch() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Search the Prometheus metric catalogue (names, help text, units) by keyword or natural-language query. Use this first to discover relevant metrics before running queries."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("Keyword or natural-language query (e.g. 'http request latency', 'node memory free', 'kube pod status'). Partial metric-name prefixes like 'http_req' also match.")),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of hits to return. Defaults to 20.")),
		mcp.WithString("type",
			mcp.Description("Optional metric type filter (counter, gauge, histogram, summary, unknown).")),
	)
	tool := mcp.NewTool("prometheus_search", opts...)

	handler := func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := req.GetInt("limit", 20)
		typeFilter := req.GetString("type", "")

		if s.index.Size() == 0 {
			return mcp.NewToolResultError("metric index is not ready yet; wait a few seconds after startup or check server logs for refresh errors"), nil
		}
		hits := s.index.Search(query, limit, typeFilter)
		return jsonResult(map[string]any{
			"query":           query,
			"results":         hits,
			"result_count":    len(hits),
			"indexed_metrics": s.index.Size(),
			"index_updated":   s.index.UpdatedAt().UTC().Format(time.RFC3339),
		})
	}

	return tool, handler
}

func (s *Server) toolQuery() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Evaluate a PromQL instant query against Prometheus."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("PromQL expression to evaluate (e.g. 'up', 'rate(http_requests_total[5m])').")),
		mcp.WithString("time",
			mcp.Description("Evaluation timestamp, RFC3339 or Unix seconds. Defaults to server time.")),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Optional query timeout in seconds.")),
		mcp.WithNumber("max_series",
			mcp.Description(fmt.Sprintf("Maximum number of series to return. Defaults to %d; 0 disables the cap.", defaultQueryMaxSeries))),
	)
	tool := mcp.NewTool("prometheus_query", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ts, err := parseTimeArg(req.GetString("time", ""))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		maxSeries := boundedLimit(req.GetInt("max_series", defaultQueryMaxSeries), defaultQueryMaxSeries)

		value, warnings, err := s.prom.API.Query(ctx, query, ts, queryOptions(req)...)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("query failed", err), nil
		}
		return queryResultWithWarnings(value, maxSeries, 0, warnings)
	}

	return tool, handler
}

func (s *Server) toolQueryRange() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Evaluate a PromQL query over a time range."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("PromQL expression to evaluate.")),
		mcp.WithString("start", mcp.Required(),
			mcp.Description("Start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end", mcp.Required(),
			mcp.Description("End timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("step", mcp.Required(),
			mcp.Description("Resolution step as a Go duration (e.g. '15s', '1m', '5m').")),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Optional query timeout in seconds.")),
		mcp.WithNumber("max_series",
			mcp.Description(fmt.Sprintf("Maximum number of series to return. Defaults to %d; 0 disables the cap.", defaultRangeMaxSeries))),
		mcp.WithNumber("max_samples_per_series",
			mcp.Description(fmt.Sprintf("Maximum number of samples kept per series. Defaults to %d; 0 disables the cap.", defaultRangeMaxSamplesPerSeries))),
	)
	tool := mcp.NewTool("prometheus_query_range", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		startStr, err := req.RequireString("start")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		endStr, err := req.RequireString("end")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		stepStr, err := req.RequireString("step")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		start, err := parseTimeArg(startStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid start", err), nil
		}
		end, err := parseTimeArg(endStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid end", err), nil
		}
		step, err := time.ParseDuration(stepStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid step", err), nil
		}
		if step <= 0 {
			return mcp.NewToolResultError("step must be a positive duration"), nil
		}
		if !end.After(start) {
			return mcp.NewToolResultError("end must be after start"), nil
		}
		maxSeries := boundedLimit(req.GetInt("max_series", defaultRangeMaxSeries), defaultRangeMaxSeries)
		maxSamples := boundedLimit(req.GetInt("max_samples_per_series", defaultRangeMaxSamplesPerSeries), defaultRangeMaxSamplesPerSeries)

		r := promv1.Range{Start: start, End: end, Step: step}
		value, warnings, err := s.prom.API.QueryRange(ctx, query, r, queryOptions(req)...)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("range query failed", err), nil
		}
		return queryResultWithWarnings(value, maxSeries, maxSamples, warnings)
	}

	return tool, handler
}

func (s *Server) toolQueryExemplars() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Query exemplars (e.g. trace IDs attached to histogram buckets) over a time range."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("PromQL expression selecting the exemplars to return.")),
		mcp.WithString("start", mcp.Required(),
			mcp.Description("Start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end", mcp.Required(),
			mcp.Description("End timestamp (RFC3339 or Unix seconds).")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of series with exemplars to return. Defaults to %d; 0 disables the cap.", defaultListLimit))),
	)
	tool := mcp.NewTool("prometheus_query_exemplars", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		startStr, err := req.RequireString("start")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		endStr, err := req.RequireString("end")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		start, err := parseTimeArg(startStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid start", err), nil
		}
		end, err := parseTimeArg(endStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid end", err), nil
		}
		limit := boundedLimit(req.GetInt("limit", defaultListLimit), defaultListLimit)

		exemplars, err := s.prom.API.QueryExemplars(ctx, query, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("exemplars query failed", err), nil
		}
		out, total, truncated := truncateSlice(exemplars, limit)
		return listResult(out, total, len(out), truncated, nil)
	}

	return tool, handler
}

func (s *Server) toolLabelNames() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("List label names present in the Prometheus TSDB."),
		mcp.WithArray("matches",
			mcp.Description("Optional series selectors (e.g. ['up', 'process_cpu_seconds_total']).")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of label names to return. Defaults to %d; 0 disables the cap.", defaultListLimit))),
	)
	tool := mcp.NewTool("prometheus_label_names", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		matches := req.GetStringSlice("matches", nil)
		start, end, err := parseOptionalRange(req)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		limit := boundedLimit(req.GetInt("limit", defaultListLimit), defaultListLimit)

		names, warnings, err := s.prom.API.LabelNames(ctx, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("label names failed", err), nil
		}
		out, total, truncated := truncateSlice(names, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

func (s *Server) toolLabelValues() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("List the values of a given label."),
		mcp.WithString("label", mcp.Required(),
			mcp.Description("Name of the label (e.g. 'job', '__name__').")),
		mcp.WithArray("matches",
			mcp.Description("Optional series selectors to filter the result.")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of values to return. Defaults to %d; 0 disables the cap.", defaultListLimit))),
	)
	tool := mcp.NewTool("prometheus_label_values", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := req.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		matches := req.GetStringSlice("matches", nil)
		start, end, err := parseOptionalRange(req)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		limit := boundedLimit(req.GetInt("limit", defaultListLimit), defaultListLimit)

		values, warnings, err := s.prom.API.LabelValues(ctx, label, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("label values failed", err), nil
		}
		out, total, truncated := truncateSlice(values, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

func (s *Server) toolSeries() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Find time series matching the provided label selectors."),
		mcp.WithArray("matches", mcp.Required(),
			mcp.Description("One or more PromQL series selectors (e.g. ['up{job=\"prometheus\"}']).")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of series to return. Defaults to %d; 0 disables the cap.", defaultListLimit))),
	)
	tool := mcp.NewTool("prometheus_series", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		matches, err := req.RequireStringSlice("matches")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		start, end, err := parseOptionalRange(req)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		limit := boundedLimit(req.GetInt("limit", defaultListLimit), defaultListLimit)

		series, warnings, err := s.prom.API.Series(ctx, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("series failed", err), nil
		}
		out, total, truncated := truncateSlice(series, limit)
		return listResult(out, total, len(out), truncated, warnings)
	}

	return tool, handler
}

func (s *Server) toolTargets() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("List Prometheus scrape targets."),
		mcp.WithString("state",
			mcp.Description("Which targets to return: 'active', 'dropped' or 'all'. Defaults to 'active'.")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of targets to return per state. Defaults to %d; 0 disables the cap.", defaultTargetsLimit))),
	)
	tool := mcp.NewTool("prometheus_targets", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		state := req.GetString("state", "active")
		limit := boundedLimit(req.GetInt("limit", defaultTargetsLimit), defaultTargetsLimit)

		targets, err := s.prom.API.Targets(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("targets failed", err), nil
		}

		payload := map[string]any{}
		switch state {
		case "active":
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
		case "all", "":
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
			return mcp.NewToolResultError(fmt.Sprintf("invalid state %q: want 'active', 'dropped' or 'all'", state)), nil
		}
		return jsonResult(payload)
	}

	return tool, handler
}

func (s *Server) toolAlerts() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_alerts", readOnly("List currently firing and pending Prometheus alerts.")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		alerts, err := s.prom.API.Alerts(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("alerts failed", err), nil
		}
		return jsonResult(alerts)
	}

	return tool, handler
}

func (s *Server) toolRules() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("List the Prometheus recording and alerting rule groups."),
		mcp.WithString("type",
			mcp.Description("Optional rule type filter: 'alert' or 'record'. Defaults to both.")),
	)
	tool := mcp.NewTool("prometheus_rules", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ruleType := req.GetString("type", "")
		switch ruleType {
		case "", "all", "alert", "record":
		default:
			return mcp.NewToolResultError(fmt.Sprintf("invalid type %q: want 'alert' or 'record'", ruleType)), nil
		}

		rules, err := s.prom.API.Rules(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("rules failed", err), nil
		}
		if ruleType == "" || ruleType == "all" {
			return jsonResult(rules)
		}
		return jsonResult(filterRules(rules, ruleType))
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

func (s *Server) toolMetadata() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	opts := append(readOnly("Return metadata (type, help, unit) for ingested metrics."),
		mcp.WithString("metric",
			mcp.Description("Metric name to filter by. Empty returns metadata for all metrics (bounded by limit).")),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum number of metrics to return when metric is empty. Defaults to %d; 0 disables the cap.", defaultMetadataLimit))),
	)
	tool := mcp.NewTool("prometheus_metadata", opts...)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		metric := req.GetString("metric", "")
		limit := req.GetInt("limit", defaultMetadataLimit)

		limitStr := ""
		if metric == "" && limit > 0 {
			limitStr = strconv.Itoa(limit)
		}
		metadata, err := s.prom.API.Metadata(ctx, metric, limitStr)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("metadata failed", err), nil
		}
		return jsonResult(map[string]any{
			"data":  metadata,
			"count": len(metadata),
		})
	}

	return tool, handler
}

func (s *Server) toolTSDBStatus() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_tsdb_status",
		readOnly("Return TSDB cardinality statistics: head series count and the top series/labels by count and memory. Useful for spotting high-cardinality metrics and gauging TSDB size.")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status, err := s.prom.API.TSDB(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("tsdb status failed", err), nil
		}
		return jsonResult(status)
	}

	return tool, handler
}

func (s *Server) toolAlertManagers() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_alertmanagers",
		readOnly("List the Alertmanager instances Prometheus currently knows about (active and dropped).")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		am, err := s.prom.API.AlertManagers(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("alertmanagers failed", err), nil
		}
		return jsonResult(am)
	}

	return tool, handler
}

func (s *Server) toolWalReplay() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_wal_replay",
		readOnly("Return the current WAL replay status (min, max, current segment).")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status, err := s.prom.API.WalReplay(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("wal replay failed", err), nil
		}
		return jsonResult(status)
	}

	return tool, handler
}

func (s *Server) toolStatusConfig() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_status_config",
		readOnly("Return the currently loaded Prometheus configuration (YAML).")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg, err := s.prom.API.Config(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("config failed", err), nil
		}
		return jsonResult(cfg)
	}

	return tool, handler
}

func (s *Server) toolStatusFlags() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_status_flags",
		readOnly("Return the command-line flags Prometheus was launched with.")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		flags, err := s.prom.API.Flags(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("flags failed", err), nil
		}
		return jsonResult(flags)
	}

	return tool, handler
}

func (s *Server) toolBuildInfo() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_buildinfo",
		readOnly("Return Prometheus server build information.")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, err := s.prom.API.Buildinfo(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("buildinfo failed", err), nil
		}
		return jsonResult(info)
	}

	return tool, handler
}

func (s *Server) toolRuntimeInfo() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_runtimeinfo",
		readOnly("Return Prometheus server runtime information (GOMAXPROCS, storage, etc).")...)

	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, err := s.prom.API.Runtimeinfo(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("runtimeinfo failed", err), nil
		}
		return jsonResult(info)
	}

	return tool, handler
}

// boundedLimit normalises a user-supplied limit. A negative value falls back
// to def; 0 means unlimited and is preserved.
func boundedLimit(v, def int) int {
	if v < 0 {
		return def
	}
	return v
}
