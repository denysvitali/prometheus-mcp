// Package prometheus builds an authenticated Prometheus API client from
// configuration.
package prometheus

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// Config describes how to reach a Prometheus server. URL is required; the
// authentication fields are optional and BearerToken wins when both are set.
type Config struct {
	URL         string
	BearerToken string
	BasicAuth   BasicAuth

	// InsecureSkipVerify disables verification of the server's TLS certificate.
	InsecureSkipVerify bool
}

// BasicAuth is an HTTP basic authentication credential. It is used only when
// Username is non-empty.
type BasicAuth struct {
	Username string
	Password string
}

// Client wraps the Prometheus v1 API plus the base URL it talks to. URL is kept
// so errors and logs can name the server. Construct one with New.
type Client struct {
	API promv1.API
	URL string
}

// New builds a Client from cfg. It returns an error if cfg.URL is empty or not a
// usable Prometheus address; it performs no I/O, so a returned Client is not
// proof that Prometheus is reachable (see Ping).
func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	apiClient, err := api.NewClient(api.Config{
		Address:      cfg.URL,
		RoundTripper: newRoundTripper(cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	return &Client{API: promv1.NewAPI(apiClient), URL: cfg.URL}, nil
}

// Ping verifies connectivity by fetching Prometheus build info. It is intended
// as an optional startup health check.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.API.Buildinfo(ctx)
	if err != nil {
		return fmt.Errorf("contacting prometheus at %s: %w", c.URL, err)
	}
	return nil
}
