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
// Construct one with New; the zero value is not usable. A Server is immutable
// after New and safe for concurrent use.
type Server struct {
	logger          *logrus.Logger
	prom            *prometheus.Client
	mcp             *mcp.Server
	index           *search.Index
	refreshInterval time.Duration
}

// Options configures a Server.
type Options struct {
	// RefreshInterval is how often StartBackground rebuilds the metric search
	// index. Zero or negative disables refreshing, which leaves
	// prometheus_search reporting that the index is not ready.
	RefreshInterval time.Duration
}

// New builds a Server with every Prometheus tool registered and a middleware
// that converts a panic in a handler into an error response. It does no I/O and
// starts no goroutine: call StartBackground for the metric index and one of the
// Serve methods to accept traffic.
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

// StartBackground launches the metric-index refresher if enabled and returns a
// function that blocks until the refresher goroutine has exited. Cancel ctx to
// stop it, then call the returned function; it is safe to call more than once,
// and returns immediately when refreshing is disabled.
//
// It returns an error only if the refresher is misconfigured, which is a
// startup-time bug rather than a runtime condition.
func (s *Server) StartBackground(ctx context.Context) (wait func(), err error) {
	if s.refreshInterval <= 0 {
		s.logger.Debug("metric index refresh disabled")
		return func() {}, nil
	}

	refresher, err := search.NewRefresher(search.RefresherConfig{
		API:      s.prom.API,
		Index:    s.index,
		Interval: s.refreshInterval,
		Logger:   s.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("starting metric index refresher: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		refresher.Run(ctx)
	}()
	return func() { <-done }, nil
}

// ServeStdio serves MCP over standard input/output. It returns when ctx is
// cancelled or the peer disconnects.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// HTTPOptions configures the streamable HTTP transport.
type HTTPOptions struct {
	// ListenAddress is the host:port to bind, e.g. ":8080".
	ListenAddress string
	// Path is the HTTP path that serves MCP requests, e.g. "/mcp".
	Path string
	// Stateless serves every request without server-side session state, for
	// load-balanced deployments that cannot maintain sticky sessions.
	Stateless bool
}

// ServeHTTP serves MCP over the streamable HTTP transport. It blocks until the
// server exits; when ctx is cancelled it gracefully shuts down, draining
// in-flight requests for up to shutdownTimeout.
func (s *Server) ServeHTTP(ctx context.Context, opts HTTPOptions) error {
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcp },
		&mcp.StreamableHTTPOptions{Stateless: opts.Stateless},
	)

	mux := http.NewServeMux()
	mux.Handle(opts.Path, handler)
	httpSrv := &http.Server{
		Addr:              opts.ListenAddress,
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
