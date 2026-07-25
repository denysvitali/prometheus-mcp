package prometheus

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Timeouts for talking to Prometheus. A query against a large TSDB can legitimately
// take a while to produce its first byte, hence the generous response-header
// budget; the others exist so a black-holed connection cannot pin a request
// forever.
const (
	dialTimeout           = 30 * time.Second
	keepAlive             = 30 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 60 * time.Second
	idleConnTimeout       = 90 * time.Second
)

// newRoundTripper returns a transport with the timeouts above that honours proxy
// environment variables (by cloning http.DefaultTransport), optionally skips TLS
// verification, and applies bearer or basic authentication. Bearer wins when both
// are configured.
func newRoundTripper(cfg Config) http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: keepAlive,
	}).DialContext
	transport.TLSHandshakeTimeout = tlsHandshakeTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	transport.IdleConnTimeout = idleConnTimeout

	if cfg.InsecureSkipVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{} //nolint:gosec // opt-in via --tls-insecure-skip-verify
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	var rt http.RoundTripper = transport
	switch {
	case cfg.BearerToken != "":
		rt = &bearerAuthTransport{token: cfg.BearerToken, next: rt}
	case cfg.BasicAuth.Username != "":
		rt = &basicAuthTransport{auth: cfg.BasicAuth, next: rt}
	}
	return rt
}

// bearerAuthTransport and basicAuthTransport look alike but encode two
// independent authentication schemes; they are expected to diverge (token
// refresh, credential sources) and are deliberately not merged.

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
	auth BasicAuth
	next http.RoundTripper
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.auth.Username, t.auth.Password)
	return t.next.RoundTrip(req)
}
