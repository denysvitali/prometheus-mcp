package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// bindings is the flag → viper key → environment variable mapping documented in
// the README. It is the contract users configure the server through, so it is
// asserted here rather than only described in prose.
var bindings = []struct {
	flag     string
	httpFlag bool // registered on the http subcommand instead of the root
	viperKey string
	envVar   string
}{
	{flag: "url", viperKey: "url", envVar: "PROMETHEUS_MCP_URL"},
	{flag: "bearer-token", viperKey: "bearer-token", envVar: "PROMETHEUS_MCP_BEARER_TOKEN"},
	{flag: "basic-auth-username", viperKey: "basic-auth.username", envVar: "PROMETHEUS_MCP_BASIC_AUTH_USERNAME"},
	{flag: "basic-auth-password", viperKey: "basic-auth.password", envVar: "PROMETHEUS_MCP_BASIC_AUTH_PASSWORD"},
	{flag: "tls-insecure-skip-verify", viperKey: "tls.insecure-skip-verify", envVar: "PROMETHEUS_MCP_TLS_INSECURE_SKIP_VERIFY"},
	{flag: "search-refresh-interval", viperKey: "search.refresh-interval", envVar: "PROMETHEUS_MCP_SEARCH_REFRESH_INTERVAL"},
	{flag: "check-connection", viperKey: "check-connection", envVar: "PROMETHEUS_MCP_CHECK_CONNECTION"},
	{flag: "log-level", viperKey: "log-level", envVar: "PROMETHEUS_MCP_LOG_LEVEL"},
	{flag: "listen-address", httpFlag: true, viperKey: "http.listen-address", envVar: "PROMETHEUS_MCP_HTTP_LISTEN_ADDRESS"},
	{flag: "path", httpFlag: true, viperKey: "http.path", envVar: "PROMETHEUS_MCP_HTTP_PATH"},
	{flag: "stateless", httpFlag: true, viperKey: "http.stateless", envVar: "PROMETHEUS_MCP_HTTP_STATELESS"},
}

func lookupFlag(t *testing.T, name string, onHTTP bool) *pflag.Flag {
	t.Helper()
	if onHTTP {
		return httpCmd.Flags().Lookup(name)
	}
	return rootCmd.PersistentFlags().Lookup(name)
}

func TestDocumentedFlagsExist(t *testing.T) {
	for _, b := range bindings {
		t.Run(b.flag, func(t *testing.T) {
			if lookupFlag(t, b.flag, b.httpFlag) == nil {
				t.Fatalf("flag --%s is documented but not registered", b.flag)
			}
		})
	}
}

func TestFlagDefaultsAreVisibleThroughViper(t *testing.T) {
	want := map[string]string{
		"url":                      "http://localhost:9090",
		"log-level":                "info",
		"search.refresh-interval":  "5m0s",
		"http.listen-address":      ":8080",
		"http.path":                "/mcp",
		"tls.insecure-skip-verify": "false",
		"check-connection":         "false",
	}
	for key, def := range want {
		if got := viper.GetString(key); got != def {
			t.Errorf("viper default for %q = %q, want %q", key, got, def)
		}
	}
}

// TestEnvVarsOverrideDefaults pins the PROMETHEUS_MCP_ prefix plus the dot/dash
// to underscore replacement that maps a viper key onto an environment variable.
func TestEnvVarsOverrideDefaults(t *testing.T) {
	initConfig()

	for _, b := range bindings {
		t.Run(b.envVar, func(t *testing.T) {
			t.Setenv(b.envVar, "from-env")
			if got := viper.GetString(b.viperKey); got != "from-env" {
				t.Errorf("%s did not reach viper key %q (got %q)", b.envVar, b.viperKey, got)
			}
		})
	}
}
