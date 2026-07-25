package search

import (
	"fmt"
	"sync"
	"testing"
)

// TestIndexConcurrentBuildAndSearch asserts the concurrency contract in Index's
// doc comment: readers never block writers, never observe a partially built
// generation, and every hit they see belongs to one consistent snapshot. Run
// under -race to be meaningful.
func TestIndexConcurrentBuildAndSearch(t *testing.T) {
	idx := NewIndex()

	// Two generations that share the query token but no metric names, so a hit
	// mixing documents from both would be visible as an unexpected name.
	const docsPerGeneration = 100
	generations := [][]Document{
		makeDocs("alpha", docsPerGeneration),
		makeDocs("beta", docsPerGeneration),
	}
	valid := map[string]bool{}
	for _, docs := range generations {
		for _, d := range docs {
			valid[d.Metric] = true
		}
	}

	// Enough rounds to interleave reliably; scoring is O(vocabulary) per query,
	// so a much larger number only makes the -race run slow.
	const rounds = 20

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			idx.Build(generations[i%len(generations)])
		}
	}()

	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				hits := idx.Search("requests", 50, "")
				for _, h := range hits {
					if !valid[h.Metric] {
						t.Errorf("hit %q belongs to no generation", h.Metric)
						return
					}
				}
				if n := idx.Size(); n != 0 && n != docsPerGeneration {
					t.Errorf("Size() = %d, want 0 or %d (a whole generation)", n, docsPerGeneration)
					return
				}
			}
		}()
	}

	wg.Wait()
}

func makeDocs(prefix string, n int) []Document {
	docs := make([]Document, n)
	for i := range docs {
		docs[i] = Document{
			Metric: fmt.Sprintf("%s_%d_requests_total", prefix, i),
			Type:   "counter",
			Help:   "Total requests.",
		}
	}
	return docs
}
