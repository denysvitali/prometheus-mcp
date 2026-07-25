package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const buildInfoOK = `{"status":"success","data":{"version":"2.53.0","revision":"abc"}}`

func TestBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	client, err := New(Config{URL: srv.URL, BearerToken: "s3cr3t"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "Bearer s3cr3t" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer s3cr3t")
	}
}

func TestBasicAuth(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	client, err := New(Config{
		URL:       srv.URL,
		BasicAuth: BasicAuth{Username: "alice", Password: "hunter2"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !gotOK || gotUser != "alice" || gotPass != "hunter2" {
		t.Errorf("basic auth = (%q,%q,%v), want (alice,hunter2,true)", gotUser, gotPass, gotOK)
	}
}

func TestTLSInsecureSkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Without skip-verify the self-signed cert must be rejected.
	client, err := New(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Ping(ctx); err == nil {
		t.Errorf("expected TLS verification failure without insecure-skip-verify")
	}

	// With skip-verify the request must succeed.
	client, err = New(Config{URL: srv.URL, InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Errorf("Ping with insecure-skip-verify failed: %v", err)
	}
}

func TestURLRequired(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Errorf("expected error when url is missing")
	}
}

func TestBearerTokenWinsOverBasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	client, err := New(Config{
		URL:         srv.URL,
		BearerToken: "s3cr3t",
		BasicAuth:   BasicAuth{Username: "alice", Password: "hunter2"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "Bearer s3cr3t" {
		t.Errorf("Authorization = %q, want the bearer token to win", gotAuth)
	}
}
