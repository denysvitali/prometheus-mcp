package server

import (
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestParameterlessToolsNameAndErrorWording pins the name and the error wording
// of every tool built by fetchTool. Those are three adjacent string arguments at
// the call site, so this table is what makes a swapped pair fail loudly.
func TestParameterlessToolsNameAndErrorWording(t *testing.T) {
	errBoom := errors.New("boom")
	s := newTestServer(&fakeAPI{Err: errBoom})

	tests := []struct {
		build    func() (*mcp.Tool, mcp.ToolHandlerFor[noArgs, any])
		wantName string
		wantErr  string
	}{
		{s.toolAlerts, "prometheus_alerts", "alerts failed: boom"},
		{s.toolTSDBStatus, "prometheus_tsdb_status", "tsdb status failed: boom"},
		{s.toolAlertManagers, "prometheus_alertmanagers", "alertmanagers failed: boom"},
		{s.toolWalReplay, "prometheus_wal_replay", "wal replay failed: boom"},
		{s.toolStatusConfig, "prometheus_status_config", "config failed: boom"},
		{s.toolStatusFlags, "prometheus_status_flags", "flags failed: boom"},
		{s.toolBuildInfo, "prometheus_buildinfo", "buildinfo failed: boom"},
		{s.toolRuntimeInfo, "prometheus_runtimeinfo", "runtimeinfo failed: boom"},
	}

	for _, tc := range tests {
		t.Run(tc.wantName, func(t *testing.T) {
			tool, handler := tc.build()
			if tool.Name != tc.wantName {
				t.Errorf("tool name = %q, want %q", tool.Name, tc.wantName)
			}
			if tool.Description == "" {
				t.Error("tool has no description")
			}
			if !tool.Annotations.ReadOnlyHint {
				t.Error("tool is missing the read-only hint")
			}
			_, err := call(t, handler, noArgs{})
			if err == nil {
				t.Fatal("expected an error from the failing API")
			}
			if err.Error() != tc.wantErr {
				t.Errorf("error = %q, want %q", err, tc.wantErr)
			}
			if !errors.Is(err, errBoom) {
				t.Errorf("error %q does not wrap the API error", err)
			}
		})
	}
}
