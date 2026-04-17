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

		srv := server.New(logger, promClient, server.Options{
			RefreshInterval: viper.GetDuration("search.refresh-interval"),
		})
		srv.StartBackground(ctx)
		logger.Info("starting prometheus-mcp in stdio mode")
		return srv.ServeStdio()
	},
}

func init() {
	rootCmd.AddCommand(stdioCmd)
}
