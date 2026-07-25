package server

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package's tests if any goroutine started by them is still
// running at the end: the index refresher and the MCP sessions must both shut
// down when their context is cancelled.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
