package server

import (
	"context"
	"testing"
	"time"
)

func TestStartBackgroundWaitReturnsAfterCancel(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	s.refreshInterval = time.Hour // one immediate refresh, then idle

	ctx, cancel := context.WithCancel(context.Background())
	wait, err := s.StartBackground(ctx)
	if err != nil {
		t.Fatalf("StartBackground: %v", err)
	}

	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not return after the context was cancelled")
	}
}

func TestStartBackgroundDisabledIsANoop(t *testing.T) {
	s := newTestServer(&fakeAPI{}) // Options{} leaves RefreshInterval at 0

	wait, err := s.StartBackground(context.Background())
	if err != nil {
		t.Fatalf("StartBackground: %v", err)
	}
	wait() // must not block: there is no goroutine to wait for
}
