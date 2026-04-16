package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	logger   = logrus.New()
	logLevel string
)

var rootCmd = &cobra.Command{
	Use:   "prometheus-mcp",
	Short: "An MCP server that exposes a Prometheus API over the Model Context Protocol",
	Long: `prometheus-mcp bridges Prometheus to MCP-compatible clients.

It wraps the official Prometheus HTTP API client and exposes query,
query_range, label, series, targets, alerts, rules and metadata tools.

Two transports are supported:
  - stdio: for local integrations (e.g. Claude Desktop, editors)
  - http:  streamable HTTP transport suitable for remote use`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		lvl, err := logrus.ParseLevel(viper.GetString("log-level"))
		if err != nil {
			return fmt.Errorf("invalid log level %q: %w", viper.GetString("log-level"), err)
		}
		logger.SetLevel(lvl)
		logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		// Logs must never go to stdout in stdio mode.
		logger.SetOutput(os.Stderr)
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.prometheus-mcp.yaml)")
	rootCmd.PersistentFlags().String("url", "http://localhost:9090", "Base URL of the Prometheus server")
	rootCmd.PersistentFlags().String("bearer-token", "", "Bearer token for Prometheus authentication")
	rootCmd.PersistentFlags().String("basic-auth-username", "", "HTTP basic auth username for Prometheus")
	rootCmd.PersistentFlags().String("basic-auth-password", "", "HTTP basic auth password for Prometheus")
	rootCmd.PersistentFlags().Bool("tls-insecure-skip-verify", false, "Skip verification of Prometheus TLS certificates")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (trace, debug, info, warn, error)")

	_ = viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	_ = viper.BindPFlag("bearer-token", rootCmd.PersistentFlags().Lookup("bearer-token"))
	_ = viper.BindPFlag("basic-auth.username", rootCmd.PersistentFlags().Lookup("basic-auth-username"))
	_ = viper.BindPFlag("basic-auth.password", rootCmd.PersistentFlags().Lookup("basic-auth-password"))
	_ = viper.BindPFlag("tls.insecure-skip-verify", rootCmd.PersistentFlags().Lookup("tls-insecure-skip-verify"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}
		viper.AddConfigPath(".")
		viper.SetConfigName(".prometheus-mcp")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("PROMETHEUS_MCP")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		logger.Debugf("using config file: %s", viper.ConfigFileUsed())
	}
}
