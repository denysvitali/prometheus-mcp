package search

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// This file holds the ranking rules: how text becomes terms, how a query scores
// documents, and how scored documents become an ordered page of hits. Everything
// here reads one immutable snapshot, so none of it needs synchronisation.

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

const (
	bm25K1 = 1.2
	bm25B  = 0.75
	// prefixWeight discounts prefix-only term matches relative to exact ones.
	prefixWeight = 0.5
)

// documentTerms counts the weighted terms of one document. The metric name is
// weighted twice to bias ranking toward name matches; help, unit and type
// contribute once each.
func documentTerms(d Document) map[string]int {
	terms := map[string]int{}
	for _, t := range tokenize(d.Metric) {
		terms[t] += 2
	}
	for _, field := range []string{d.Help, d.Unit, d.Type} {
		for _, t := range tokenize(field) {
			terms[t]++
		}
	}
	return terms
}

// phraseBoost multiplies the score of a document whose metric name contains the
// whole query as a substring, so an exact phrase in the name outranks documents
// that merely match the same tokens.
const phraseBoost = 1.5

// score returns the relevance of every document that matched at least one token,
// keyed by document ID. query is passed alongside its tokens to apply the
// metric-name phrase boost.
func (s *snapshot) score(tokens []string, query string) map[int]float64 {
	scores := make(map[int]float64, 64)
	for _, t := range tokens {
		s.scoreTerm(t, scores)
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	if qLower == "" {
		return scores
	}
	for id := range scores {
		if strings.Contains(strings.ToLower(s.docs[id].Metric), qLower) {
			scores[id] *= phraseBoost
		}
	}
	return scores
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

// rank returns the IDs of the documents worth showing, best first. Documents
// that scored nothing are dropped, as are those whose type does not match
// typeFilter when one is given. Equal scores are broken by metric name, so the
// order is stable for identical documents.
func (s *snapshot) rank(scores map[int]float64, typeFilter string) []int {
	typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))

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
	return ids
}

// hits renders the first limit ranked documents. A non-positive limit, or one
// larger than the number of ranked documents, returns all of them.
func (s *snapshot) hits(ids []int, scores map[int]float64, limit int) []Hit {
	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	out := make([]Hit, limit)
	for i := 0; i < limit; i++ {
		d := s.docs[ids[i]]
		out[i] = Hit{
			Metric: d.Metric,
			Type:   d.Type,
			Help:   d.Help,
			Unit:   d.Unit,
			Score:  scores[ids[i]],
		}
	}
	return out
}
