package cmd

import (
	"time"

	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
)

// Config is the resolved configuration of one prometheus-mcp run. It is read
// from viper exactly once, by loadConfig, so no package below cmd/ depends on
// global configuration state.
type Config struct {
	Prometheus prometheus.Config
	HTTP       HTTPConfig

	// RefreshInterval is how often the metric search index is rebuilt.
	// Zero or negative disables refreshing.
	RefreshInterval time.Duration
	// CheckConnection makes startup fail fast when Prometheus is unreachable.
	CheckConnection bool
}

// HTTPConfig configures the streamable HTTP transport. It is ignored by the
// stdio command.
type HTTPConfig struct {
	ListenAddress string
	Path          string
	Stateless     bool
}

// loadConfig collects every configuration value the server needs. The keys are
// bound to flags in root.go and http.go, and to PROMETHEUS_MCP_* environment
// variables in initConfig.
func loadConfig(v *viper.Viper) Config {
	return Config{
		Prometheus: prometheus.Config{
			URL:         v.GetString("url"),
			BearerToken: v.GetString("bearer-token"),
			BasicAuth: prometheus.BasicAuth{
				Username: v.GetString("basic-auth.username"),
				Password: v.GetString("basic-auth.password"),
			},
			InsecureSkipVerify: v.GetBool("tls.insecure-skip-verify"),
		},
		HTTP: HTTPConfig{
			ListenAddress: v.GetString("http.listen-address"),
			Path:          v.GetString("http.path"),
			Stateless:     v.GetBool("http.stateless"),
		},
		RefreshInterval: v.GetDuration("search.refresh-interval"),
		CheckConnection: v.GetBool("check-connection"),
	}
}
