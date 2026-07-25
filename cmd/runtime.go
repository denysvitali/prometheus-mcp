package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/server"
)

// pingTimeout bounds the optional startup connectivity check.
const pingTimeout = 10 * time.Second

// serve runs one MCP transport until ctx is cancelled or it fails.
type serve func(ctx context.Context, srv *server.Server, cfg Config) error

// run owns everything the transports share: configuration, the Prometheus
// client, signal handling, the optional startup ping, and the lifecycle of the
// metric-index refresher. Each command supplies only its transport.
//
// The refresher is stopped and waited for before run returns, including when the
// transport exits on its own rather than on a signal.
func run(transport serve) error {
	cfg := loadConfig(viper.GetViper())

	promClient, err := prometheus.New(cfg.Prometheus)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := checkConnection(ctx, cfg, promClient); err != nil {
		return err
	}

	srv := server.New(logger, promClient, server.Options{
		RefreshInterval: cfg.RefreshInterval,
	})
	waitForRefresher, err := srv.StartBackground(ctx)
	if err != nil {
		return err
	}

	transportErr := transport(ctx, srv, cfg)
	cancel()
	waitForRefresher()
	return transportErr
}

// checkConnection verifies connectivity to Prometheus when --check-connection is
// set, and is a no-op otherwise.
func checkConnection(ctx context.Context, cfg Config, client *prometheus.Client) error {
	if !cfg.CheckConnection {
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
