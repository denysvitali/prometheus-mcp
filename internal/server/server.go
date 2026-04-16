package server

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/denysvitali/prometheus-mcp/internal/prometheus"
)

const (
	serverName    = "prometheus-mcp"
	serverVersion = "0.1.0"
)

type Server struct {
	logger *logrus.Logger
	prom   *prometheus.Client
	mcp    *server.MCPServer
}

func New(logger *logrus.Logger, prom *prometheus.Client) *Server {
	mcpSrv := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	s := &Server{logger: logger, prom: prom, mcp: mcpSrv}
	s.registerTools()
	return s
}

func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcp)
}

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
