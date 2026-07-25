package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/denysvitali/prometheus-mcp/internal/server"
)

var stdioCmd = &cobra.Command{
	Use:   "stdio",
	Short: "Run the MCP server over stdio",
	RunE: func(_ *cobra.Command, _ []string) error {
		return run(func(ctx context.Context, srv *server.Server, _ Config) error {
			logger.Info("starting prometheus-mcp in stdio mode")
			return srv.ServeStdio(ctx)
		})
	},
}

func init() {
	rootCmd.AddCommand(stdioCmd)
}
