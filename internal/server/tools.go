package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func (s *Server) registerTools() {
	s.mcp.AddTool(s.toolQuery())
	s.mcp.AddTool(s.toolQueryRange())
	s.mcp.AddTool(s.toolLabelNames())
	s.mcp.AddTool(s.toolLabelValues())
	s.mcp.AddTool(s.toolSeries())
	s.mcp.AddTool(s.toolTargets())
	s.mcp.AddTool(s.toolAlerts())
	s.mcp.AddTool(s.toolRules())
	s.mcp.AddTool(s.toolMetadata())
	s.mcp.AddTool(s.toolBuildInfo())
	s.mcp.AddTool(s.toolRuntimeInfo())
}

func (s *Server) toolQuery() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_query",
		mcp.WithDescription("Evaluate a PromQL instant query against Prometheus."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("PromQL expression to evaluate (e.g. 'up', 'rate(http_requests_total[5m])').")),
		mcp.WithString("time",
			mcp.Description("Evaluation timestamp, RFC3339 or Unix seconds. Defaults to server time.")),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Optional query timeout in seconds.")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ts, err := parseTimeArg(req.GetString("time", ""))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}

		opts := queryOptions(req)
		value, warnings, err := s.prom.API.Query(ctx, query, ts, opts...)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("query failed", err), nil
		}
		return resultWithWarnings(map[string]any{
			"resultType": value.Type().String(),
			"result":     value,
		}, warnings)
	}

	return tool, handler
}

func (s *Server) toolQueryRange() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_query_range",
		mcp.WithDescription("Evaluate a PromQL query over a time range."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
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
	)

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

		opts := queryOptions(req)
		r := promv1.Range{Start: start, End: end, Step: step}
		value, warnings, err := s.prom.API.QueryRange(ctx, query, r, opts...)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("range query failed", err), nil
		}
		return resultWithWarnings(map[string]any{
			"resultType": value.Type().String(),
			"result":     value,
		}, warnings)
	}

	return tool, handler
}

func (s *Server) toolLabelNames() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_label_names",
		mcp.WithDescription("List label names present in the Prometheus TSDB."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithArray("matches",
			mcp.Description("Optional series selectors (e.g. ['up', 'process_cpu_seconds_total']).")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		matches := req.GetStringSlice("matches", nil)
		start, end, err := parseOptionalRange(req)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		names, warnings, err := s.prom.API.LabelNames(ctx, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("label names failed", err), nil
		}
		return resultWithWarnings(names, warnings)
	}

	return tool, handler
}

func (s *Server) toolLabelValues() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_label_values",
		mcp.WithDescription("List the values of a given label."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("label", mcp.Required(),
			mcp.Description("Name of the label (e.g. 'job', '__name__').")),
		mcp.WithArray("matches",
			mcp.Description("Optional series selectors to filter the result.")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
	)

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
		values, warnings, err := s.prom.API.LabelValues(ctx, label, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("label values failed", err), nil
		}
		return resultWithWarnings(values, warnings)
	}

	return tool, handler
}

func (s *Server) toolSeries() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_series",
		mcp.WithDescription("Find time series matching the provided label selectors."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithArray("matches", mcp.Required(),
			mcp.Description("One or more PromQL series selectors (e.g. ['up{job=\"prometheus\"}']).")),
		mcp.WithString("start",
			mcp.Description("Optional start timestamp (RFC3339 or Unix seconds).")),
		mcp.WithString("end",
			mcp.Description("Optional end timestamp (RFC3339 or Unix seconds).")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		matches, err := req.RequireStringSlice("matches")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		start, end, err := parseOptionalRange(req)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid time", err), nil
		}
		series, warnings, err := s.prom.API.Series(ctx, matches, start, end)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("series failed", err), nil
		}
		return resultWithWarnings(series, warnings)
	}

	return tool, handler
}

func (s *Server) toolTargets() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_targets",
		mcp.WithDescription("List Prometheus scrape targets (active and dropped)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		targets, err := s.prom.API.Targets(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("targets failed", err), nil
		}
		return jsonResult(targets)
	}

	return tool, handler
}

func (s *Server) toolAlerts() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_alerts",
		mcp.WithDescription("List currently firing and pending Prometheus alerts."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		alerts, err := s.prom.API.Alerts(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("alerts failed", err), nil
		}
		return jsonResult(alerts)
	}

	return tool, handler
}

func (s *Server) toolRules() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_rules",
		mcp.WithDescription("List the Prometheus recording and alerting rule groups."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rules, err := s.prom.API.Rules(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("rules failed", err), nil
		}
		return jsonResult(rules)
	}

	return tool, handler
}

func (s *Server) toolMetadata() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_metadata",
		mcp.WithDescription("Return metadata (type, help, unit) for ingested metrics."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("metric",
			mcp.Description("Metric name to filter by. Empty returns metadata for all metrics.")),
		mcp.WithString("limit",
			mcp.Description("Optional maximum number of metrics to return.")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		metric := req.GetString("metric", "")
		limit := req.GetString("limit", "")
		metadata, err := s.prom.API.Metadata(ctx, metric, limit)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("metadata failed", err), nil
		}
		return jsonResult(metadata)
	}

	return tool, handler
}

func (s *Server) toolBuildInfo() (mcp.Tool, func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	tool := mcp.NewTool("prometheus_buildinfo",
		mcp.WithDescription("Return Prometheus server build information."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		mcp.WithDescription("Return Prometheus server runtime information (GOMAXPROCS, storage, etc)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, err := s.prom.API.Runtimeinfo(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("runtimeinfo failed", err), nil
		}
		return jsonResult(info)
	}

	return tool, handler
}

func queryOptions(req mcp.CallToolRequest) []promv1.Option {
	var opts []promv1.Option
	if t := req.GetFloat("timeout_seconds", 0); t > 0 {
		opts = append(opts, promv1.WithTimeout(time.Duration(t*float64(time.Second))))
	}
	return opts
}

func parseTimeArg(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	var seconds float64
	if _, err := fmt.Sscanf(s, "%f", &seconds); err == nil {
		sec := int64(seconds)
		nano := int64((seconds - float64(sec)) * 1e9)
		return time.Unix(sec, nano).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

func parseOptionalRange(req mcp.CallToolRequest) (time.Time, time.Time, error) {
	startStr := req.GetString("start", "")
	endStr := req.GetString("end", "")
	var start, end time.Time
	var err error
	if startStr != "" {
		start, err = parseTimeArg(startStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if endStr != "" {
		end, err = parseTimeArg(endStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	return start, end, nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("marshaling result", err), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func resultWithWarnings(v any, warnings promv1.Warnings) (*mcp.CallToolResult, error) {
	payload := map[string]any{"data": v}
	if len(warnings) > 0 {
		payload["warnings"] = []string(warnings)
	}
	return jsonResult(payload)
}
