package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/server"
)

var stdioCmd = &cobra.Command{
	Use:   "stdio",
	Short: "Run the MCP server over stdio",
	RunE: func(_ *cobra.Command, _ []string) error {
		promClient, err := prometheus.NewFromViper(viper.GetViper())
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if err := maybePing(ctx, promClient); err != nil {
			return err
		}

		srv := server.New(logger, promClient, server.Options{
			RefreshInterval: viper.GetDuration("search.refresh-interval"),
		})
		waitRefresher, err := srv.StartBackground(ctx)
		if err != nil {
			return err
		}
		logger.Info("starting prometheus-mcp in stdio mode")
		serveErr := srv.ServeStdio(ctx)
		// Stop the refresher even when the transport returned on its own, then
		// wait for it, so the process never exits with work in flight.
		cancel()
		waitRefresher()
		return serveErr
	},
}

func init() {
	rootCmd.AddCommand(stdioCmd)
}
