// Package server builds the MCP server and registers its Prometheus tools.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
	"github.com/denysvitali/prometheus-mcp/internal/search"
)

// shutdownTimeout bounds how long a graceful HTTP shutdown waits for in-flight
// requests to drain.
const shutdownTimeout = 15 * time.Second

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

// ServeHTTP serves MCP over the streamable HTTP transport. It blocks until the
// server exits; when ctx is cancelled it gracefully shuts down, draining
// in-flight requests for up to shutdownTimeout.
func (s *Server) ServeHTTP(ctx context.Context, addr, path string, stateless bool) error {
	opts := []server.StreamableHTTPOption{
		server.WithEndpointPath(path),
	}
	if stateless {
		opts = append(opts, server.WithStateLess(true))
	}
	httpSrv := server.NewStreamableHTTPServer(s.mcp, opts...)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.Start(addr)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		// Start returns http.ErrServerClosed after a clean Shutdown.
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
