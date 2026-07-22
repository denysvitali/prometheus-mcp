package server

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/search"
)

func newTestServer(api promv1.API) *Server {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return New(logger, &prometheus.Client{API: api, URL: "http://test"}, Options{})
}

// call invokes a tool handler and returns the decoded JSON payload plus the
// raw result (for IsError inspection).
func call(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) (map[string]any, *mcp.CallToolResult) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("result has no content")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if res.IsError {
		// Error results carry a plain-text message, not JSON.
		return map[string]any{"error": text}, res
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, text)
	}
	return payload, res
}

func makeVector(n int) model.Vector {
	v := make(model.Vector, n)
	for i := 0; i < n; i++ {
		v[i] = &model.Sample{Metric: model.Metric{"__name__": "up", "i": model.LabelValue(string(rune('a' + i)))}, Value: 1}
	}
	return v
}

func makeMatrix(series, samples int) model.Matrix {
	m := make(model.Matrix, series)
	for i := 0; i < series; i++ {
		vals := make([]model.SamplePair, samples)
		for j := 0; j < samples; j++ {
			vals[j] = model.SamplePair{Timestamp: model.Time(j), Value: model.SampleValue(j)}
		}
		m[i] = &model.SampleStream{Metric: model.Metric{"__name__": "up"}, Values: vals}
	}
	return m
}

func TestQueryTruncatesVector(t *testing.T) {
	api := &fakeAPI{QueryFn: func(context.Context, string, time.Time) (model.Value, promv1.Warnings, error) {
		return makeVector(5), nil, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolQuery()

	payload, _ := call(t, handler, map[string]any{"query": "up", "max_series": 2})
	stats := payload["stats"].(map[string]any)
	if stats["series_total"].(float64) != 5 {
		t.Errorf("series_total = %v, want 5", stats["series_total"])
	}
	if stats["series_returned"].(float64) != 2 {
		t.Errorf("series_returned = %v, want 2", stats["series_returned"])
	}
	if stats["truncated"] != true {
		t.Errorf("truncated = %v, want true", stats["truncated"])
	}
	result := payload["result"].([]any)
	if len(result) != 2 {
		t.Errorf("result len = %d, want 2", len(result))
	}
}

func TestQueryRangeTruncatesSamples(t *testing.T) {
	api := &fakeAPI{QueryRangeFn: func(context.Context, string, promv1.Range) (model.Value, promv1.Warnings, error) {
		return makeMatrix(1, 10), nil, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolQueryRange()

	payload, _ := call(t, handler, map[string]any{
		"query": "up", "start": "2024-01-01T00:00:00Z", "end": "2024-01-01T01:00:00Z",
		"step": "1m", "max_samples_per_series": 3,
	})
	stats := payload["stats"].(map[string]any)
	if stats["samples_total"].(float64) != 10 {
		t.Errorf("samples_total = %v, want 10", stats["samples_total"])
	}
	if stats["samples_returned"].(float64) != 3 {
		t.Errorf("samples_returned = %v, want 3", stats["samples_returned"])
	}
	if stats["truncated"] != true {
		t.Errorf("truncated = %v, want true", stats["truncated"])
	}
}

func TestQueryRangeRejectsBadRange(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	_, handler := s.toolQueryRange()

	_, res := call(t, handler, map[string]any{
		"query": "up", "start": "2024-01-01T01:00:00Z", "end": "2024-01-01T00:00:00Z", "step": "1m",
	})
	if !res.IsError {
		t.Errorf("expected error result for end before start")
	}
}

func TestSeriesLimit(t *testing.T) {
	api := &fakeAPI{SeriesFn: func(context.Context, []string) ([]model.LabelSet, promv1.Warnings, error) {
		out := make([]model.LabelSet, 5)
		for i := range out {
			out[i] = model.LabelSet{"__name__": "up"}
		}
		return out, nil, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolSeries()

	payload, _ := call(t, handler, map[string]any{"matches": []any{"up"}, "limit": 2})
	if payload["total"].(float64) != 5 {
		t.Errorf("total = %v, want 5", payload["total"])
	}
	if payload["returned"].(float64) != 2 {
		t.Errorf("returned = %v, want 2", payload["returned"])
	}
	if payload["truncated"] != true {
		t.Errorf("truncated = %v, want true", payload["truncated"])
	}
}

func TestLabelValuesLimit(t *testing.T) {
	api := &fakeAPI{LabelValuesFn: func(context.Context, string) (model.LabelValues, promv1.Warnings, error) {
		return model.LabelValues{"a", "b", "c", "d"}, nil, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolLabelValues()

	payload, _ := call(t, handler, map[string]any{"label": "job", "limit": 2})
	if payload["total"].(float64) != 4 {
		t.Errorf("total = %v, want 4", payload["total"])
	}
	if payload["returned"].(float64) != 2 {
		t.Errorf("returned = %v, want 2", payload["returned"])
	}
	data := payload["data"].([]any)
	if len(data) != 2 {
		t.Errorf("data len = %d, want 2", len(data))
	}
}

func TestSearchNotReady(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	_, handler := s.toolSearch()

	_, res := call(t, handler, map[string]any{"query": "http"})
	if !res.IsError {
		t.Errorf("expected error result when index is empty")
	}
}

func TestSearchUsesIndex(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	s.index.Build([]search.Document{{Metric: "http_request_duration_seconds", Type: "histogram", Help: "HTTP request duration."}})
	_, handler := s.toolSearch()

	payload, _ := call(t, handler, map[string]any{"query": "http request"})
	if payload["result_count"].(float64) != 1 {
		t.Errorf("result_count = %v, want 1", payload["result_count"])
	}
}

func TestTSDBStatus(t *testing.T) {
	api := &fakeAPI{TSDBFn: func(context.Context) (promv1.TSDBResult, error) {
		return promv1.TSDBResult{HeadStats: promv1.TSDBHeadStats{NumSeries: 12345}}, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolTSDBStatus()

	payload, _ := call(t, handler, map[string]any{})
	head := payload["headStats"].(map[string]any)
	if head["numSeries"].(float64) != 12345 {
		t.Errorf("numSeries = %v, want 12345", head["numSeries"])
	}
}

func TestRulesTypeFilter(t *testing.T) {
	api := &fakeAPI{RulesFn: func(context.Context) (promv1.RulesResult, error) {
		return promv1.RulesResult{Groups: []promv1.RuleGroup{{
			Name: "g",
			Rules: promv1.Rules{
				promv1.AlertingRule{Name: "alert1"},
				promv1.RecordingRule{Name: "record1"},
			},
		}}}, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolRules()

	payload, _ := call(t, handler, map[string]any{"type": "alert"})
	groups := payload["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	rules := groups[0].(map[string]any)["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules len = %d, want 1 (alerting only)", len(rules))
	}
	if rules[0].(map[string]any)["name"] != "alert1" {
		t.Errorf("kept rule = %v, want alert1", rules[0])
	}
}

func TestTargetsStateFilter(t *testing.T) {
	api := &fakeAPI{TargetsFn: func(context.Context) (promv1.TargetsResult, error) {
		return promv1.TargetsResult{
			Active:  []promv1.ActiveTarget{{ScrapeURL: "http://a"}},
			Dropped: []promv1.DroppedTarget{{DiscoveredLabels: map[string]string{"job": "d"}}},
		}, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolTargets()

	payload, _ := call(t, handler, map[string]any{"state": "dropped"})
	if _, ok := payload["active"]; ok {
		t.Errorf("state=dropped should not include active targets")
	}
	if payload["dropped_total"].(float64) != 1 {
		t.Errorf("dropped_total = %v, want 1", payload["dropped_total"])
	}
}

func TestMetadataDefaultLimit(t *testing.T) {
	var gotLimit string
	api := &fakeAPI{MetadataFn: func(_ context.Context, _, limit string) (map[string][]promv1.Metadata, error) {
		gotLimit = limit
		return map[string][]promv1.Metadata{"up": {{Type: "gauge"}}}, nil
	}}
	s := newTestServer(api)
	_, handler := s.toolMetadata()

	call(t, handler, map[string]any{})
	if gotLimit != "100" {
		t.Errorf("metadata limit passed to API = %q, want \"100\"", gotLimit)
	}
}
