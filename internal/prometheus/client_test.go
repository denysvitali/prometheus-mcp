package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/viper"
)

const buildInfoOK = `{"status":"success","data":{"version":"2.53.0","revision":"abc"}}`

func newViper(url string) *viper.Viper {
	v := viper.New()
	v.Set("url", url)
	return v
}

func TestBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	v := newViper(srv.URL)
	v.Set("bearer-token", "s3cr3t")
	client, err := NewFromViper(v)
	if err != nil {
		t.Fatalf("NewFromViper: %v", err)
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

	v := newViper(srv.URL)
	v.Set("basic-auth.username", "alice")
	v.Set("basic-auth.password", "hunter2")
	client, err := NewFromViper(v)
	if err != nil {
		t.Fatalf("NewFromViper: %v", err)
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buildInfoOK))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Without skip-verify the self-signed cert must be rejected.
	v := newViper(srv.URL)
	client, err := NewFromViper(v)
	if err != nil {
		t.Fatalf("NewFromViper: %v", err)
	}
	if err := client.Ping(ctx); err == nil {
		t.Errorf("expected TLS verification failure without insecure-skip-verify")
	}

	// With skip-verify the request must succeed.
	v.Set("tls.insecure-skip-verify", true)
	client, err = NewFromViper(v)
	if err != nil {
		t.Fatalf("NewFromViper: %v", err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Errorf("Ping with insecure-skip-verify failed: %v", err)
	}
}

func TestURLRequired(t *testing.T) {
	if _, err := NewFromViper(viper.New()); err == nil {
		t.Errorf("expected error when url is missing")
	}
}
