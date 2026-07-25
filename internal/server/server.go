// Package server builds the MCP server and registers its Prometheus tools.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	mcp             *mcp.Server
	index           *search.Index
	refreshInterval time.Duration
}

// Options configures a Server.
type Options struct {
	RefreshInterval time.Duration
}

// New builds a Server with all Prometheus tools registered.
func New(logger *logrus.Logger, prom *prometheus.Client, opts Options) *Server {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: Version,
	}, nil)

	s := &Server{
		logger:          logger,
		prom:            prom,
		mcp:             mcpSrv,
		index:           search.NewIndex(),
		refreshInterval: opts.RefreshInterval,
	}
	mcpSrv.AddReceivingMiddleware(s.recoverMiddleware)
	s.registerTools()
	return s
}

// recoverMiddleware turns a panic in a tool handler into an error response
// instead of tearing down the process.
func (s *Server) recoverMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Errorf("panic handling %s: %v", method, r)
				result, err = nil, fmt.Errorf("internal error handling %s: %v", method, r)
			}
		}()
		return next(ctx, method, req)
	}
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

// ServeStdio serves MCP over standard input/output. It returns when ctx is
// cancelled or the peer disconnects.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// ServeHTTP serves MCP over the streamable HTTP transport. It blocks until the
// server exits; when ctx is cancelled it gracefully shuts down, draining
// in-flight requests for up to shutdownTimeout.
func (s *Server) ServeHTTP(ctx context.Context, addr, path string, stateless bool) error {
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcp },
		&mcp.StreamableHTTPOptions{Stateless: stateless},
	)

	mux := http.NewServeMux()
	mux.Handle(path, handler)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		// ListenAndServe returns http.ErrServerClosed after a clean Shutdown.
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
