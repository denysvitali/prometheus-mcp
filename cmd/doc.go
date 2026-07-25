// Package cmd is the command line for prometheus-mcp: it declares the flags,
// resolves them (together with the config file and PROMETHEUS_MCP_* environment
// variables) into a single Config, and runs one of the two transports.
//
// This is the only package that touches viper or os.Exit. Everything below it
// receives explicit configuration.
package cmd
