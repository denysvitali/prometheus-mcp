package server

import (
	"fmt"
	"strconv"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// This file turns the string arguments an MCP client sends into the types the
// Prometheus client expects. Every function here is pure and returns an error
// message the calling model can act on.

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

// parseQueryRange validates the start/end/step trio of a range query: both
// timestamps must parse, step must be a positive duration, and end must be
// after start.
func parseQueryRange(startStr, endStr, stepStr string) (promv1.Range, error) {
	start, err := parseTimeArg(startStr)
	if err != nil {
		return promv1.Range{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err := parseTimeArg(endStr)
	if err != nil {
		return promv1.Range{}, fmt.Errorf("invalid end: %w", err)
	}
	step, err := time.ParseDuration(stepStr)
	if err != nil {
		return promv1.Range{}, fmt.Errorf("invalid step: %w", err)
	}
	if step <= 0 {
		return promv1.Range{}, fmt.Errorf("step must be a positive duration")
	}
	if !end.After(start) {
		return promv1.Range{}, fmt.Errorf("end must be after start")
	}
	return promv1.Range{Start: start, End: end, Step: step}, nil
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
