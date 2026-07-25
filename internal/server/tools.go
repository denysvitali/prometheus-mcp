package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) registerTools() {
	register(s.mcp, s.toolSearch)
	register(s.mcp, s.toolQuery)
	register(s.mcp, s.toolQueryRange)
	register(s.mcp, s.toolQueryExemplars)
	register(s.mcp, s.toolLabelNames)
	register(s.mcp, s.toolLabelValues)
	register(s.mcp, s.toolSeries)
	register(s.mcp, s.toolTargets)
	register(s.mcp, s.toolAlerts)
	register(s.mcp, s.toolRules)
	register(s.mcp, s.toolMetadata)
	register(s.mcp, s.toolTSDBStatus)
	register(s.mcp, s.toolAlertManagers)
	register(s.mcp, s.toolWalReplay)
	register(s.mcp, s.toolStatusConfig)
	register(s.mcp, s.toolStatusFlags)
	register(s.mcp, s.toolBuildInfo)
	register(s.mcp, s.toolRuntimeInfo)
}

// register adds a tool built by def to srv. It exists because Go cannot expand
// a two-value call into the three parameters of mcp.AddTool; the input type is
// inferred from def's handler, so each tool keeps its own typed arguments.
func register[In any](srv *mcp.Server, def func() (*mcp.Tool, mcp.ToolHandlerFor[In, any])) {
	tool, handler := def()
	mcp.AddTool(srv, tool, handler)
}

// noArgs is the input type for tools that take no parameters.
type noArgs struct{}

// fetchTool builds a parameterless read-only tool that renders whatever fetch
// returns as JSON. errLabel names the operation in the error message, which
// reads "<errLabel> failed: <cause>"; it is a separate argument because the tool
// name and the wording of its error do not always match (prometheus_status_flags
// reports "flags failed").
func fetchTool[T any](
	name, description, errLabel string,
	fetch func(context.Context) (T, error),
) (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any]) {
	handler := func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		v, err := fetch(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("%s failed: %w", errLabel, err)
		}
		return jsonResult(v)
	}
	return readOnlyTool(name, description), handler
}

// readOnlyTool builds a tool definition annotated as a non-destructive,
// open-world read. Every tool in this server only reads from Prometheus.
func readOnlyTool(name, description string) *mcp.Tool {
	no, yes := false, true
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &no,
			OpenWorldHint:   &yes,
		},
	}
}

// boundedLimit normalises a user-supplied limit. An absent or negative value
// falls back to def; 0 means unlimited and is preserved.
func boundedLimit(v *int, def int) int {
	if v == nil || *v < 0 {
		return def
	}
	return *v
}
