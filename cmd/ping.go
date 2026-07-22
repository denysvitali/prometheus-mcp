package cmd

import (
	"context"
	"time"

	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
)

// pingTimeout bounds the optional startup connectivity check.
const pingTimeout = 10 * time.Second

// maybePing verifies connectivity to Prometheus when --check-connection is set.
// It is a no-op otherwise.
func maybePing(ctx context.Context, client *prometheus.Client) error {
	if !viper.GetBool("check-connection") {
		return nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx); err != nil {
		return err
	}
	logger.Infof("connected to prometheus at %s", client.URL)
	return nil
}
