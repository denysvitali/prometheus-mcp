package search

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

// refreshFake overrides only the methods Refresher uses; the embedded nil
// interface panics if anything else is called, which would fail the test.
type refreshFake struct {
	promv1.API
	metadata map[string][]promv1.Metadata
	names    []string
	namesErr error
}

func (f *refreshFake) Metadata(context.Context, string, string) (map[string][]promv1.Metadata, error) {
	return f.metadata, nil
}

func (f *refreshFake) LabelValues(_ context.Context, label string, _ []string, _, _ time.Time, _ ...promv1.Option) (model.LabelValues, promv1.Warnings, error) {
	if label != "__name__" {
		return nil, nil, nil
	}
	if f.namesErr != nil {
		return nil, nil, f.namesErr
	}
	out := make(model.LabelValues, len(f.names))
	for i, n := range f.names {
		out[i] = model.LabelValue(n)
	}
	return out, nil, nil
}

func newTestRefresher(t *testing.T, api promv1.API, idx *Index) *Refresher {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	r, err := NewRefresher(RefresherConfig{API: api, Index: idx, Interval: time.Minute, Logger: logger})
	if err != nil {
		t.Fatalf("NewRefresher: %v", err)
	}
	return r
}

func TestRefreshIndexesMetadataAndFallbackNames(t *testing.T) {
	api := &refreshFake{
		metadata: map[string][]promv1.Metadata{
			"http_requests_total": {{Type: "counter", Help: "Total HTTP requests."}},
		},
		// __name__ includes the metadata metric plus one without metadata.
		names: []string{"http_requests_total", "custom_metric_without_help"},
	}
	idx := NewIndex()
	newTestRefresher(t, api, idx).refreshOnce(context.Background())

	if idx.Size() != 2 {
		t.Fatalf("index size = %d, want 2", idx.Size())
	}
	hits := idx.Search("custom_metric_without_help", 5, "")
	if len(hits) == 0 || hits[0].Metric != "custom_metric_without_help" {
		t.Fatalf("fallback metric not searchable, got %+v", hits)
	}
}

func TestRefreshToleratesFallbackFailure(t *testing.T) {
	api := &refreshFake{
		metadata: map[string][]promv1.Metadata{
			"http_requests_total": {{Type: "counter"}},
		},
		namesErr: errors.New("label values unavailable"),
	}
	idx := NewIndex()
	newTestRefresher(t, api, idx).refreshOnce(context.Background())

	if idx.Size() != 1 {
		t.Fatalf("index size = %d, want 1 (metadata only)", idx.Size())
	}
}
