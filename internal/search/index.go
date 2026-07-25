// Package search implements an in-memory BM25 index over Prometheus metric
// metadata so MCP clients can discover metrics by keyword or natural-language
// query instead of listing every series name.
package search

import (
	"math"
	"regexp"
	"sort"
	"strings"
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

var (
	tokenSplit = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	camelSplit = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

func tokenize(s string) []string {
	s = camelSplit.ReplaceAllString(s, "$1 $2")
	s = strings.ToLower(s)
	parts := tokenSplit.Split(s, -1)
	// Filter in place: parts is a fresh slice owned by this call, so reusing its
	// backing array avoids a second allocation per tokenized string.
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Build replaces the current index contents with the provided documents. It
// takes ownership of docs, which must not be mutated afterwards. Searches
// running concurrently keep serving the previous contents until Build returns.
func (idx *Index) Build(docs []Document) {
	idx.current.Store(newSnapshot(docs))
}

// newSnapshot indexes docs. The metric name is weighted twice to bias ranking
// toward name matches.
func newSnapshot(docs []Document) *snapshot {
	postings := map[string][]posting{}
	docFreq := map[string]int{}
	docLen := make([]int, len(docs))
	totalLen := 0

	for i, d := range docs {
		terms := map[string]int{}
		for _, t := range tokenize(d.Metric) {
			terms[t] += 2
		}
		for _, t := range tokenize(d.Help) {
			terms[t]++
		}
		for _, t := range tokenize(d.Unit) {
			terms[t]++
		}
		for _, t := range tokenize(d.Type) {
			terms[t]++
		}
		length := 0
		for term, tf := range terms {
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

const (
	bm25K1 = 1.2
	bm25B  = 0.75
	// prefixWeight discounts prefix-only term matches relative to exact ones.
	prefixWeight = 0.5
)

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
	typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))

	scores := make(map[int]float64, 64)
	for _, t := range tokens {
		s.scoreTerm(t, scores)
	}

	// Boost documents whose metric name contains the full query as a
	// substring; this rewards exact phrase matches in the name.
	qLower := strings.ToLower(strings.TrimSpace(query))
	if qLower != "" {
		for id := range scores {
			if strings.Contains(strings.ToLower(s.docs[id].Metric), qLower) {
				scores[id] *= 1.5
			}
		}
	}

	ids := make([]int, 0, len(scores))
	for id, sc := range scores {
		if sc <= 0 {
			continue
		}
		if typeFilter != "" && !strings.EqualFold(s.docs[id].Type, typeFilter) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] != scores[ids[j]] {
			return scores[ids[i]] > scores[ids[j]]
		}
		return s.docs[ids[i]].Metric < s.docs[ids[j]].Metric
	})

	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	hits := make([]Hit, limit)
	for i := 0; i < limit; i++ {
		d := s.docs[ids[i]]
		hits[i] = Hit{
			Metric: d.Metric,
			Type:   d.Type,
			Help:   d.Help,
			Unit:   d.Unit,
			Score:  scores[ids[i]],
		}
	}
	return hits
}

// scoreTerm adds the BM25 contribution of a single query token to scores.
// Exact term matches are weighted fully; terms that merely share the token as
// a prefix (and the token is at least 2 runes) contribute at a discount so
// exact matches outrank prefix-only matches.
func (s *snapshot) scoreTerm(token string, scores map[int]float64) {
	n := float64(len(s.docs))
	for term, df := range s.docFreq {
		weight := 1.0
		if term != token {
			if len(token) < 2 || !strings.HasPrefix(term, token) {
				continue
			}
			weight = prefixWeight
		}
		idf := math.Log(1 + (n-float64(df)+0.5)/(float64(df)+0.5))
		for _, p := range s.postings[term] {
			tf := float64(p.tf)
			docLen := float64(s.docLen[p.docID])
			denom := tf + bm25K1*(1-bm25B+bm25B*docLen/s.avgDocLen)
			scores[p.docID] += weight * idf * (tf * (bm25K1 + 1)) / denom
		}
	}
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
