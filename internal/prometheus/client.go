// Package prometheus builds an authenticated Prometheus API client from
// configuration.
package prometheus

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/spf13/viper"
)

// Client wraps the Prometheus v1 API plus the base URL it talks to.
type Client struct {
	API promv1.API
	URL string
}

// NewFromViper builds a Client from viper configuration keys: url,
// bearer-token, basic-auth.username/password and tls.insecure-skip-verify.
func NewFromViper(v *viper.Viper) (*Client, error) {
	url := v.GetString("url")
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	cfg := api.Config{
		Address:      url,
		RoundTripper: newRoundTripper(v),
	}

	apiClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	return &Client{API: promv1.NewAPI(apiClient), URL: url}, nil
}

// newRoundTripper returns a transport with sane timeouts that honours proxy
// environment variables (by cloning http.DefaultTransport), optional TLS
// verification skip, and bearer/basic authentication.
func newRoundTripper(v *viper.Viper) http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 60 * time.Second
	transport.IdleConnTimeout = 90 * time.Second

	if v.GetBool("tls.insecure-skip-verify") {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	var rt http.RoundTripper = transport
	if token := v.GetString("bearer-token"); token != "" {
		rt = &bearerAuthTransport{token: token, next: rt}
	} else if user := v.GetString("basic-auth.username"); user != "" {
		rt = &basicAuthTransport{
			username: user,
			password: v.GetString("basic-auth.password"),
			next:     rt,
		}
	}
	return rt
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

type bearerAuthTransport struct {
	token string
	next  http.RoundTripper
}

func (t *bearerAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(req)
}

type basicAuthTransport struct {
	username string
	password string
	next     http.RoundTripper
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.username, t.password)
	return t.next.RoundTrip(req)
}
