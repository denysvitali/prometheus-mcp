package server

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Default caps applied to tool output. An MCP server feeds an LLM, so
// unbounded Prometheus responses (long range queries, high-cardinality label
// values, full series lists) can exhaust a client's context window. These
// defaults keep responses useful while staying bounded; every cap can be
// raised or disabled (0 = unlimited) per call.
const (
	defaultQueryMaxSeries           = 100
	defaultRangeMaxSeries           = 50
	defaultRangeMaxSamplesPerSeries = 100
	defaultListLimit                = 500
	defaultTargetsLimit             = 200
	defaultMetadataLimit            = 100
)

// jsonResult renders v as compact JSON. Compact (non-indented) output is
// deliberate: MCP results are consumed by models, so we minimise token usage.
//
// The three return values match the shape of an mcp.ToolHandlerFor, so
// handlers can `return jsonResult(payload)` directly. The output type is any,
// which tells the SDK not to generate an output schema: Prometheus payloads
// are dynamic maps rather than fixed structs.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// listResult renders a bounded list response with pagination metadata so a
// client can tell when a result was truncated and request more with a higher
// limit.
func listResult(data any, total, returned int, truncated bool, warnings promv1.Warnings) (*mcp.CallToolResult, any, error) {
	payload := map[string]any{
		"data":     data,
		"total":    total,
		"returned": returned,
	}
	if truncated {
		payload["truncated"] = true
	}
	if len(warnings) > 0 {
		payload["warnings"] = []string(warnings)
	}
	return jsonResult(payload)
}

// truncateSlice bounds a slice to limit elements (limit <= 0 means unlimited)
// and reports the original total and whether truncation occurred.
func truncateSlice[T any](in []T, limit int) (out []T, total int, truncated bool) {
	total = len(in)
	if limit > 0 && total > limit {
		return in[:limit], total, true
	}
	return in, total, false
}

// queryStats summarises a (possibly truncated) query result so the caller
// knows how much data was dropped.
type queryStats struct {
	ResultType      string `json:"result_type"`
	SeriesTotal     int    `json:"series_total,omitempty"`
	SeriesReturned  int    `json:"series_returned,omitempty"`
	SamplesTotal    int    `json:"samples_total,omitempty"`
	SamplesReturned int    `json:"samples_returned,omitempty"`
	Truncated       bool   `json:"truncated,omitempty"`
}

// shapeQueryResult bounds a Prometheus query result. maxSeries caps the number
// of returned series; maxSamplesPerSeries caps the number of samples kept per
// series for range (matrix) results. A non-positive value disables the
// corresponding cap. The original value is returned (possibly sliced) together
// with stats describing any truncation.
func shapeQueryResult(value model.Value, maxSeries, maxSamplesPerSeries int) (any, queryStats) {
	stats := queryStats{ResultType: value.Type().String()}

	switch v := value.(type) {
	case model.Vector:
		stats.SeriesTotal = len(v)
		out, total, truncated := truncateSlice(v, maxSeries)
		stats.SeriesReturned = len(out)
		stats.Truncated = truncated
		_ = total
		return out, stats

	case model.Matrix:
		stats.SeriesTotal = len(v)
		series, _, seriesTruncated := truncateSlice(v, maxSeries)
		samplesTotal := 0
		samplesReturned := 0
		samplesTruncated := false
		out := make(model.Matrix, len(series))
		for i, stream := range series {
			samplesTotal += len(stream.Values)
			kept, _, sTrunc := truncateSlice(stream.Values, maxSamplesPerSeries)
			samplesReturned += len(kept)
			if sTrunc {
				samplesTruncated = true
			}
			cp := *stream
			cp.Values = kept
			out[i] = &cp
		}
		// Account for samples belonging to series that were dropped entirely.
		for _, stream := range v[len(series):] {
			samplesTotal += len(stream.Values)
		}
		stats.SeriesReturned = len(out)
		stats.SamplesTotal = samplesTotal
		stats.SamplesReturned = samplesReturned
		stats.Truncated = seriesTruncated || samplesTruncated
		return out, stats

	default:
		// Scalar and String results are already tiny.
		return value, stats
	}
}

// queryResultWithWarnings renders a bounded query result plus stats/warnings.
func queryResultWithWarnings(value model.Value, maxSeries, maxSamplesPerSeries int, warnings promv1.Warnings) (*mcp.CallToolResult, any, error) {
	result, stats := shapeQueryResult(value, maxSeries, maxSamplesPerSeries)
	payload := map[string]any{
		"resultType": stats.ResultType,
		"result":     result,
		"stats":      stats,
	}
	if len(warnings) > 0 {
		payload["warnings"] = []string(warnings)
	}
	return jsonResult(payload)
}

// queryOptions turns an optional timeout into Prometheus client options.
func queryOptions(timeoutSeconds *float64) []promv1.Option {
	if timeoutSeconds == nil || *timeoutSeconds <= 0 {
		return nil
	}
	return []promv1.Option{promv1.WithTimeout(time.Duration(*timeoutSeconds * float64(time.Second)))}
}

// parseTimeArg accepts RFC3339 (with or without nanoseconds) or Unix seconds
// (integer or fractional). An empty string resolves to now.
//
// The numeric fallback requires the whole string to be a number:
// strconv.ParseFloat rejects "2024-01-02", where fmt.Sscanf("%f") would have
// accepted the "2024" prefix and silently returned 1970.
func parseTimeArg(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if seconds, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(seconds)
		nano := int64((seconds - float64(sec)) * 1e9)
		return time.Unix(sec, nano).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q (want RFC3339 such as 2024-01-02T03:04:05Z, or Unix seconds)", s)
}

// parseOptionalRange parses an optional start/end pair, leaving either side as
// the zero time when the corresponding argument is absent.
func parseOptionalRange(startStr, endStr string) (time.Time, time.Time, error) {
	var start, end time.Time
	var err error
	if startStr != "" {
		start, err = parseTimeArg(startStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start: %w", err)
		}
	}
	if endStr != "" {
		end, err = parseTimeArg(endStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end: %w", err)
		}
	}
	return start, end, nil
}
