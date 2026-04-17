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

var httpCmd = &cobra.Command{
	Use:   "http",
	Short: "Run the MCP server over streamable HTTP",
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

		addr := viper.GetString("http.listen-address")
		path := viper.GetString("http.path")
		stateless := viper.GetBool("http.stateless")
		logger.Infof("starting prometheus-mcp in http mode on %s%s", addr, path)
		return srv.ServeHTTP(addr, path, stateless)
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
