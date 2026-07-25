package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/server"
)

var httpCmd = &cobra.Command{
	Use:   "http",
	Short: "Run the MCP server over streamable HTTP",
	RunE: func(_ *cobra.Command, _ []string) error {
		return run(func(ctx context.Context, srv *server.Server, cfg Config) error {
			logger.Infof("starting prometheus-mcp in http mode on %s%s", cfg.HTTP.ListenAddress, cfg.HTTP.Path)
			return srv.ServeHTTP(ctx, cfg.HTTP)
		})
	},
}

func init() {
	httpCmd.Flags().String("listen-address", ":8080", "Address to bind the HTTP server on")
	httpCmd.Flags().String("path", "/mcp", "HTTP path that serves MCP requests")
	httpCmd.Flags().Bool("stateless", false, "Run the streamable HTTP server in stateless mode")

	_ = viper.BindPFlag("http.listen-address", httpCmd.Flags().Lookup("listen-address"))
	_ = viper.BindPFlag("http.path", httpCmd.Flags().Lookup("path"))
	_ = viper.BindPFlag("http.stateless", httpCmd.Flags().Lookup("stateless"))

	rootCmd.AddCommand(httpCmd)
}
