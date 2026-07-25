// Package search implements an in-memory BM25 index over Prometheus metric
// metadata so MCP clients can discover metrics by keyword or natural-language
// query instead of listing every series name.
package search

import (
	"sync/atomic"
	"time"
)

// Document is the subset of Prometheus metric metadata that we index.
type Document struct {
	Metric string
	Type   string
	Help   string
	Unit   string
}

// Hit is a single ranked search result.
type Hit struct {
	Metric string  `json:"metric"`
	Type   string  `json:"type,omitempty"`
	Help   string  `json:"help,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Score  float64 `json:"score"`
}

// Index is a BM25 inverted index over metric metadata documents.
//
// Concurrency: an Index holds a pointer to one immutable snapshot. Build builds
// a fresh snapshot off to the side and installs it with a single atomic store,
// and every reader loads the pointer once and then works on data that cannot
// change underneath it. There is therefore no lock, no lock ordering to reason
// about, and no way to observe a half-built index. An Index is safe to use from
// any goroutine; the zero value is not usable, call NewIndex.
type Index struct {
	current atomic.Pointer[snapshot]
}

// snapshot is one generation of the index. Every field is written exactly once,
// by newSnapshot, and is read-only from then on.
type snapshot struct {
	docs      []Document
	docLen    []int
	avgDocLen float64
	postings  map[string][]posting
	docFreq   map[string]int
	updatedAt time.Time
}

type posting struct {
	docID int
	tf    int
}

// NewIndex returns an empty Index that is ready to search (returning no hits)
// before the first Build.
func NewIndex() *Index {
	idx := &Index{}
	idx.current.Store(&snapshot{postings: map[string][]posting{}, docFreq: map[string]int{}})
	return idx
}

// Build replaces the current index contents with the provided documents. It
// takes ownership of docs, which must not be mutated afterwards. Searches
// running concurrently keep serving the previous contents until Build returns.
func (idx *Index) Build(docs []Document) {
	idx.current.Store(newSnapshot(docs))
}

// newSnapshot indexes docs into one immutable generation.
func newSnapshot(docs []Document) *snapshot {
	postings := map[string][]posting{}
	docFreq := map[string]int{}
	docLen := make([]int, len(docs))
	totalLen := 0

	for i, d := range docs {
		length := 0
		for term, tf := range documentTerms(d) {
			postings[term] = append(postings[term], posting{docID: i, tf: tf})
			docFreq[term]++
			length += tf
		}
		docLen[i] = length
		totalLen += length
	}

	avg := 0.0
	if len(docs) > 0 {
		avg = float64(totalLen) / float64(len(docs))
	}

	return &snapshot{
		docs:      docs,
		postings:  postings,
		docFreq:   docFreq,
		docLen:    docLen,
		avgDocLen: avg,
		updatedAt: time.Now(),
	}
}

// Search returns up to limit documents ranked by relevance. Scoring combines
// BM25 over the indexed terms with a discounted contribution from prefix
// matches, so partial metric names (e.g. "http_req") still surface the right
// series. An optional typeFilter restricts hits to a metric type. A
// non-positive limit returns every scored document.
func (idx *Index) Search(query string, limit int, typeFilter string) []Hit {
	return idx.current.Load().search(query, limit, typeFilter)
}

func (s *snapshot) search(query string, limit int, typeFilter string) []Hit {
	if len(s.docs) == 0 {
		return nil
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	scores := s.score(tokens, query)
	return s.hits(s.rank(scores, typeFilter), scores, limit)
}

// Size reports the number of documents currently indexed.
func (idx *Index) Size() int {
	return len(idx.current.Load().docs)
}

// UpdatedAt reports when the index was last rebuilt, or the zero time if Build
// has never run.
func (idx *Index) UpdatedAt() time.Time {
	return idx.current.Load().updatedAt
}
