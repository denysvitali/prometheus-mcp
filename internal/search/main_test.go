package search

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package's tests if any goroutine started by them is still
// running at the end, which is how the refresher's exit path stays honest.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
