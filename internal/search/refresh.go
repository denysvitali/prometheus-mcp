package search

import (
	"context"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/sirupsen/logrus"
)

// Refresher periodically rebuilds an Index from the Prometheus metadata API.
type Refresher struct {
	API      promv1.API
	Index    *Index
	Interval time.Duration
	Logger   *logrus.Logger
	Timeout  time.Duration
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
	r.Index.Build(docs)
	r.Logger.WithField("metrics", len(docs)).Debug("metric index refreshed")
}
