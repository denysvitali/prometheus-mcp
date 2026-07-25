package search

import (
	"context"
	"io"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/sirupsen/logrus"
)

func discardLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func TestNewRefresherRejectsIncompleteConfig(t *testing.T) {
	valid := RefresherConfig{
		API:      &refreshFake{},
		Index:    NewIndex(),
		Interval: time.Minute,
		Logger:   discardLogger(),
	}

	// Each case removes exactly one required field from a valid config.
	tests := map[string]func(*RefresherConfig){
		"no api":            func(c *RefresherConfig) { c.API = nil },
		"no index":          func(c *RefresherConfig) { c.Index = nil },
		"no logger":         func(c *RefresherConfig) { c.Logger = nil },
		"zero interval":     func(c *RefresherConfig) { c.Interval = 0 },
		"negative interval": func(c *RefresherConfig) { c.Interval = -time.Second },
	}

	for name, invalidate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			invalidate(&cfg)
			if _, err := NewRefresher(cfg); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}

	if _, err := NewRefresher(valid); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestNewRefresherAppliesDefaults(t *testing.T) {
	r, err := NewRefresher(RefresherConfig{
		API:      &refreshFake{},
		Index:    NewIndex(),
		Interval: time.Minute,
		Logger:   discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewRefresher: %v", err)
	}
	if r.timeout != defaultFetchTimeout {
		t.Errorf("timeout = %s, want %s", r.timeout, defaultFetchTimeout)
	}
	if r.lookback != DefaultLookback {
		t.Errorf("lookback = %s, want %s", r.lookback, DefaultLookback)
	}
}

// TestRunReturnsWhenContextIsCancelled pins the exit path that lets a caller
// wait for the refresher instead of leaking it.
func TestRunReturnsWhenContextIsCancelled(t *testing.T) {
	r, err := NewRefresher(RefresherConfig{
		API:      &refreshFake{metadata: map[string][]promv1.Metadata{"up": {{Type: "gauge"}}}},
		Index:    NewIndex(),
		Interval: time.Hour, // never ticks during the test
		Logger:   discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewRefresher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}
