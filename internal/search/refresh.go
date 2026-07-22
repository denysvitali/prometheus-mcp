package search

import (
	"context"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/sirupsen/logrus"
)

// DefaultLookback is the time window used to discover metric names via the
// __name__ label when falling back from the metadata endpoint.
const DefaultLookback = time.Hour

// Refresher periodically rebuilds an Index from the Prometheus metadata API.
type Refresher struct {
	API      promv1.API
	Index    *Index
	Interval time.Duration
	Logger   *logrus.Logger
	Timeout  time.Duration
	// Lookback bounds the time window used for the __name__ fallback query.
	// Defaults to DefaultLookback when zero.
	Lookback time.Duration
}

// Run builds the index immediately, then rebuilds it every Interval until
// ctx is cancelled. Fetch errors are logged; the existing index is retained.
func (r *Refresher) Run(ctx context.Context) {
	r.refreshOnce(ctx)
	t := time.NewTicker(r.Interval)
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
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	metadata, err := r.API.Metadata(fetchCtx, "", "")
	if err != nil {
		r.Logger.WithError(err).Warn("metric index refresh failed")
		return
	}

	docs := make([]Document, 0, len(metadata))
	seen := make(map[string]struct{}, len(metadata))
	for name, entries := range metadata {
		doc := Document{Metric: name}
		if len(entries) > 0 {
			m := entries[0]
			doc.Type = string(m.Type)
			doc.Help = m.Help
			doc.Unit = m.Unit
		}
		docs = append(docs, doc)
		seen[name] = struct{}{}
	}

	// Many real deployments expose metrics without HELP/TYPE metadata, so they
	// never appear in /api/v1/metadata. Fall back to the __name__ label values
	// to index those names too (name-only documents). Failure here is not
	// fatal: we still build from whatever metadata we have.
	if names, err := r.metricNames(fetchCtx); err != nil {
		r.Logger.WithError(err).Debug("metric name fallback unavailable; indexing metadata only")
	} else {
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
			r.Logger.WithField("metrics", added).Debug("indexed metrics without metadata via __name__ fallback")
		}
	}

	r.Index.Build(docs)
	r.Logger.WithField("metrics", len(docs)).Debug("metric index refreshed")
}

// metricNames returns the distinct metric names present in the TSDB within the
// configured lookback window.
func (r *Refresher) metricNames(ctx context.Context) ([]string, error) {
	lookback := r.Lookback
	if lookback <= 0 {
		lookback = DefaultLookback
	}
	end := time.Now()
	start := end.Add(-lookback)
	values, _, err := r.API.LabelValues(ctx, "__name__", nil, start, end)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(values))
	for i, v := range values {
		names[i] = string(v)
	}
	return names, nil
}
