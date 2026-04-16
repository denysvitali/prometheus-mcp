package prometheus

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/spf13/viper"
)

type Client struct {
	API promv1.API
	URL string
}

func NewFromViper(v *viper.Viper) (*Client, error) {
	url := v.GetString("prometheus.url")
	if url == "" {
		return nil, fmt.Errorf("prometheus.url is required")
	}

	cfg := api.Config{Address: url}

	transport := &http.Transport{}
	if v.GetBool("prometheus.tls.insecure-skip-verify") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	var rt http.RoundTripper = transport

	if token := v.GetString("prometheus.bearer-token"); token != "" {
		rt = &bearerAuthTransport{token: token, next: rt}
	} else if user := v.GetString("prometheus.basic-auth.username"); user != "" {
		rt = &basicAuthTransport{
			username: user,
			password: v.GetString("prometheus.basic-auth.password"),
			next:     rt,
		}
	}

	cfg.RoundTripper = rt

	apiClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	return &Client{API: promv1.NewAPI(apiClient), URL: url}, nil
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
