package search

import (
	"context"
	"fmt"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultLookback is the time window used to discover metric names via the
	// __name__ label when falling back from the metadata endpoint.
	DefaultLookback = time.Hour

	// defaultFetchTimeout bounds one refresh. Metadata for a large TSDB can take
	// seconds; anything slower than this is treated as a failed refresh and
	// retried at the next interval rather than allowed to pile up.
	defaultFetchTimeout = 30 * time.Second
)

// RefresherConfig describes a Refresher. API, Index, Interval and Logger are
// required; Timeout and Lookback fall back to defaults when zero.
type RefresherConfig struct {
	API      promv1.API
	Index    *Index
	Interval time.Duration
	Logger   *logrus.Logger

	// Timeout bounds a single refresh. Defaults to defaultFetchTimeout.
	Timeout time.Duration
	// Lookback bounds the time window used for the __name__ fallback query.
	// Defaults to DefaultLookback.
	Lookback time.Duration
}

// Refresher periodically rebuilds an Index from the Prometheus metadata API.
// Construct one with NewRefresher; the zero value is not usable.
type Refresher struct {
	api      promv1.API
	index    *Index
	interval time.Duration
	logger   *logrus.Logger
	timeout  time.Duration
	lookback time.Duration
}

// NewRefresher validates cfg and returns a Refresher ready to Run. It returns an
// error rather than panicking later: a zero Interval used to reach
// time.NewTicker, and a nil Logger used to be a nil dereference on the first
// refresh error.
func NewRefresher(cfg RefresherConfig) (*Refresher, error) {
	if cfg.API == nil {
		return nil, fmt.Errorf("prometheus API is required")
	}
	if cfg.Index == nil {
		return nil, fmt.Errorf("index is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("interval must be positive, got %s", cfg.Interval)
	}

	r := &Refresher{
		api:      cfg.API,
		index:    cfg.Index,
		interval: cfg.Interval,
		logger:   cfg.Logger,
		timeout:  cfg.Timeout,
		lookback: cfg.Lookback,
	}
	if r.timeout <= 0 {
		r.timeout = defaultFetchTimeout
	}
	if r.lookback <= 0 {
		r.lookback = DefaultLookback
	}
	return r, nil
}

// Run builds the index immediately, then rebuilds it every Interval until ctx is
// cancelled, at which point it returns. Fetch errors are logged and the existing
// index is retained.
//
// Concurrency: Run owns nothing but the ticker; it publishes each generation
// through Index.Build, which is safe against concurrent searches. The caller
// starts Run in a goroutine and is responsible for waiting on its return.
func (r *Refresher) Run(ctx context.Context) {
	r.refreshOnce(ctx)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.refreshOnce(ctx)
		}
	}
}

func (r *Refresher) refreshOnce(ctx context.Context) {
	fetchCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	metadata, err := r.api.Metadata(fetchCtx, "", "")
	if err != nil {
		r.logger.WithError(err).Warn("metric index refresh failed")
		return
	}

	docs := r.withFallbackNames(fetchCtx, documentsFrom(metadata))
	r.index.Build(docs)
	r.logger.WithField("metrics", len(docs)).Debug("metric index refreshed")
}

// documentsFrom turns a Prometheus metadata response into documents, keeping the
// first entry when a metric name reports several.
func documentsFrom(metadata map[string][]promv1.Metadata) []Document {
	docs := make([]Document, 0, len(metadata))
	for name, entries := range metadata {
		doc := Document{Metric: name}
		if len(entries) > 0 {
			m := entries[0]
			doc.Type = string(m.Type)
			doc.Help = m.Help
			doc.Unit = m.Unit
		}
		docs = append(docs, doc)
	}
	return docs
}

// withFallbackNames appends name-only documents for metrics that are in the TSDB
// but absent from docs.
//
// Many real deployments expose metrics without HELP/TYPE metadata, so they never
// appear in /api/v1/metadata. Failure here is not fatal: the index is still built
// from whatever metadata was available.
func (r *Refresher) withFallbackNames(ctx context.Context, docs []Document) []Document {
	names, err := r.metricNames(ctx)
	if err != nil {
		r.logger.WithError(err).Debug("metric name fallback unavailable; indexing metadata only")
		return docs
	}

	seen := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		seen[d.Metric] = struct{}{}
	}

	added := 0
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		docs = append(docs, Document{Metric: name})
		seen[name] = struct{}{}
		added++
	}
	if added > 0 {
		r.logger.WithField("metrics", added).Debug("indexed metrics without metadata via __name__ fallback")
	}
	return docs
}

// metricNames returns the distinct metric names present in the TSDB within the
// configured lookback window.
func (r *Refresher) metricNames(ctx context.Context) ([]string, error) {
	end := time.Now()
	start := end.Add(-r.lookback)
	values, _, err := r.api.LabelValues(ctx, "__name__", nil, start, end)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(values))
	for i, v := range values {
		names[i] = string(v)
	}
	return names, nil
}
