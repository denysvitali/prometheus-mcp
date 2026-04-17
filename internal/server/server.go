// Package server builds the MCP server and registers its Prometheus tools.
package server

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/search"
)

const serverName = "prometheus-mcp"

// Version is the server version, overridden at build time via -ldflags.
var Version = "dev"

// Server owns the MCP server, a Prometheus client and the metric search index.
type Server struct {
	logger          *logrus.Logger
	prom            *prometheus.Client
	mcp             *server.MCPServer
	index           *search.Index
	refreshInterval time.Duration
}

// Options configures a Server.
type Options struct {
	RefreshInterval time.Duration
}

// New builds a Server with all Prometheus tools registered.
func New(logger *logrus.Logger, prom *prometheus.Client, opts Options) *Server {
	mcpSrv := server.NewMCPServer(
		serverName,
		Version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	s := &Server{
		logger:          logger,
		prom:            prom,
		mcp:             mcpSrv,
		index:           search.NewIndex(),
		refreshInterval: opts.RefreshInterval,
	}
	s.registerTools()
	return s
}

// StartBackground launches the metric-index refresher if enabled. The
// goroutine stops when ctx is cancelled.
func (s *Server) StartBackground(ctx context.Context) {
	if s.refreshInterval <= 0 {
		s.logger.Debug("metric index refresh disabled")
		return
	}
	refresher := &search.Refresher{
		API:      s.prom.API,
		Index:    s.index,
		Interval: s.refreshInterval,
		Logger:   s.logger,
	}
	go refresher.Run(ctx)
}

// ServeStdio serves MCP over standard input/output.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcp)
}

// ServeHTTP serves MCP over the streamable HTTP transport.
func (s *Server) ServeHTTP(addr, path string, stateless bool) error {
	opts := []server.StreamableHTTPOption{
		server.WithEndpointPath(path),
	}
	if stateless {
		opts = append(opts, server.WithStateLess(true))
	}
	httpSrv := server.NewStreamableHTTPServer(s.mcp, opts...)
	return httpSrv.Start(addr)
}
