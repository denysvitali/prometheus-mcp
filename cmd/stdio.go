package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/server"
)

var stdioCmd = &cobra.Command{
	Use:   "stdio",
	Short: "Run the MCP server over stdio",
	RunE: func(cmd *cobra.Command, args []string) error {
		promClient, err := prometheus.NewFromViper(viper.GetViper())
		if err != nil {
			return err
		}
		srv := server.New(logger, promClient)
		logger.Info("starting prometheus-mcp in stdio mode")
		return srv.ServeStdio()
	},
}

func init() {
	rootCmd.AddCommand(stdioCmd)
}
