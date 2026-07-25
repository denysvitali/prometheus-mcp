package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/denysvitali/prometheus-mcp/internal/search"
)

// connect wires a client to s over an in-memory transport and returns the
// client session. Both sessions are closed when the test ends.
func connect(t *testing.T, ctx context.Context, s *Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := s.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Errorf("closing server session: %v", err)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("closing client session: %v", err)
		}
	})
	return session
}

// TestSessionListsAndCallsTools drives the server through a real MCP session,
// which exercises schema inference for every registered tool (AddTool panics on
// a type it cannot describe) and argument validation on the way in.
func TestSessionListsAndCallsTools(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	s.index.Build([]search.Document{{Metric: "http_requests_total", Type: "counter", Help: "Total requests."}})

	ctx := context.Background()
	session := connect(t, ctx, s)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	if len(byName) != 18 {
		t.Errorf("registered %d tools, want 18", len(byName))
	}
	searchTool := byName["prometheus_search"]
	if searchTool == nil {
		t.Fatalf("prometheus_search not registered")
	}
	if searchTool.Annotations == nil || !searchTool.Annotations.ReadOnlyHint {
		t.Errorf("prometheus_search should carry a read-only hint")
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "prometheus_search",
		Arguments: map[string]any{"query": "requests"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported an error: %v", res.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &payload); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if payload["result_count"].(float64) != 1 {
		t.Errorf("result_count = %v, want 1", payload["result_count"])
	}
}

// TestSessionRejectsMissingRequiredArgument checks that the SDK validates
// arguments against the inferred input schema before the handler runs.
func TestSessionRejectsMissingRequiredArgument(t *testing.T) {
	s := newTestServer(&fakeAPI{})

	ctx := context.Background()
	session := connect(t, ctx, s)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "prometheus_query",
		Arguments: map[string]any{},
	})
	if err == nil && !res.IsError {
		t.Fatalf("expected a failure for a missing required 'query' argument")
	}
}
