package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

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
	return fetchTool("prometheus_alerts",
		"List currently firing and pending Prometheus alerts.",
		"alerts", s.prom.API.Alerts)
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
