// Package search implements an in-memory BM25 index over Prometheus metric
// metadata so MCP clients can discover metrics by keyword or natural-language
// query instead of listing every series name.
package search

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
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

// Index is a concurrent BM25 inverted index over metric metadata documents.
type Index struct {
	mu        sync.RWMutex
	docs      []Document
	docLen    []int
	avgDocLen float64
	postings  map[string][]posting
	docFreq   map[string]int
	total     int
	updatedAt time.Time
}

type posting struct {
	docID int
	tf    int
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{postings: map[string][]posting{}, docFreq: map[string]int{}}
}

var (
	tokenSplit = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	camelSplit = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

func tokenize(s string) []string {
	s = camelSplit.ReplaceAllString(s, "$1 $2")
	s = strings.ToLower(s)
	parts := tokenSplit.Split(s, -1)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Build replaces the current index contents with the provided documents.
// The metric name is weighted twice to bias ranking toward name matches.
func (idx *Index) Build(docs []Document) {
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

	idx.mu.Lock()
	idx.docs = docs
	idx.postings = postings
	idx.docFreq = docFreq
	idx.docLen = docLen
	idx.avgDocLen = avg
	idx.total = len(docs)
	idx.updatedAt = time.Now()
	idx.mu.Unlock()
}

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// Search returns up to limit documents ranked by BM25 with a small
// substring-match boost against the metric name. A non-positive limit returns
// every scored document.
func (idx *Index) Search(query string, limit int) []Hit {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.total == 0 {
		return nil
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}

	scores := make(map[int]float64, 64)
	for _, t := range tokens {
		df := idx.docFreq[t]
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(idx.total)-float64(df)+0.5)/(float64(df)+0.5))
		for _, p := range idx.postings[t] {
			tf := float64(p.tf)
			docLen := float64(idx.docLen[p.docID])
			denom := tf + bm25K1*(1-bm25B+bm25B*docLen/idx.avgDocLen)
			scores[p.docID] += idf * (tf * (bm25K1 + 1)) / denom
		}
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	if qLower != "" {
		for id := range scores {
			if strings.Contains(strings.ToLower(idx.docs[id].Metric), qLower) {
				scores[id] *= 1.5
			}
		}
	}

	ids := make([]int, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] != scores[ids[j]] {
			return scores[ids[i]] > scores[ids[j]]
		}
		return idx.docs[ids[i]].Metric < idx.docs[ids[j]].Metric
	})

	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	hits := make([]Hit, limit)
	for i := 0; i < limit; i++ {
		d := idx.docs[ids[i]]
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

// Size reports the number of documents currently indexed.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.total
}

// UpdatedAt reports when the index was last rebuilt.
func (idx *Index) UpdatedAt() time.Time {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.updatedAt
}
