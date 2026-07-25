package search

import "testing"

// The tests in this file pin ranking behaviour that is easy to change by
// accident while refactoring the scoring code: tie-breaking, the meaning of a
// non-positive limit, and which queries produce no hits at all.

func TestSearchTieBreaksByMetricName(t *testing.T) {
	idx := NewIndex()
	// Identical documents apart from the name, so every score is equal and only
	// the tie-break rule decides the order.
	idx.Build([]Document{
		{Metric: "zzz_total", Type: "counter", Help: "Requests."},
		{Metric: "aaa_total", Type: "counter", Help: "Requests."},
		{Metric: "mmm_total", Type: "counter", Help: "Requests."},
	})

	hits := idx.Search("requests", 0, "")
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(hits))
	}
	want := []string{"aaa_total", "mmm_total", "zzz_total"}
	for i, w := range want {
		if hits[i].Metric != w {
			t.Fatalf("hit %d = %q, want %q (hits: %+v)", i, hits[i].Metric, w, hits)
		}
	}
}

func TestSearchNonPositiveLimitReturnsEverything(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "http_requests_total"},
		{Metric: "http_request_duration_seconds"},
		{Metric: "http_response_size_bytes"},
	})

	for _, limit := range []int{0, -1} {
		if hits := idx.Search("http", limit, ""); len(hits) != 3 {
			t.Errorf("limit=%d returned %d hits, want 3", limit, len(hits))
		}
	}
}

func TestSearchLimitAboveHitCountIsNotPadded(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{{Metric: "up"}, {Metric: "http_requests_total"}})

	if hits := idx.Search("up", 100, ""); len(hits) != 1 {
		t.Errorf("got %d hits, want 1", len(hits))
	}
}

func TestSearchQueryWithoutTokensReturnsNil(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{{Metric: "up"}})

	for _, q := range []string{"", "   ", "!!!"} {
		if hits := idx.Search(q, 10, ""); hits != nil {
			t.Errorf("Search(%q) = %+v, want nil", q, hits)
		}
	}
}

func TestSearchPhraseBoostPrefersSubstringOfName(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		// Both documents match the tokens "node" and "cpu"; only the first
		// contains the query as a substring of its name.
		{Metric: "node_cpu_seconds_total", Help: "Seconds."},
		{Metric: "cpu_node_ratio", Help: "Node cpu ratio."},
	})

	hits := idx.Search("node_cpu", 5, "")
	if len(hits) == 0 || hits[0].Metric != "node_cpu_seconds_total" {
		t.Fatalf("top hit = %+v, want node_cpu_seconds_total", hits)
	}
}

func TestSearchTypeFilterIsCaseInsensitiveAndDropsOthers(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "http_requests_total", Type: "counter"},
		{Metric: "http_request_duration_seconds", Type: "histogram"},
	})

	hits := idx.Search("http", 10, "  COUNTER  ")
	if len(hits) != 1 || hits[0].Metric != "http_requests_total" {
		t.Fatalf("hits = %+v, want only http_requests_total", hits)
	}
}

func TestBuildReplacesPreviousContents(t *testing.T) {
	idx := NewIndex()
	// The two names share no tokens, so a hit can only come from the document
	// that is actually indexed.
	idx.Build([]Document{{Metric: "alpha_widget"}})
	idx.Build([]Document{{Metric: "beta_gadget"}})

	if idx.Size() != 1 {
		t.Errorf("size = %d, want 1", idx.Size())
	}
	if hits := idx.Search("alpha_widget", 10, ""); len(hits) != 0 {
		t.Errorf("old document still searchable: %+v", hits)
	}
	if hits := idx.Search("beta_gadget", 10, ""); len(hits) != 1 {
		t.Errorf("new document not searchable: %+v", hits)
	}
}

func TestUpdatedAtSetByBuild(t *testing.T) {
	idx := NewIndex()
	if !idx.UpdatedAt().IsZero() {
		t.Errorf("UpdatedAt on a fresh index = %v, want zero", idx.UpdatedAt())
	}
	idx.Build(nil)
	if idx.UpdatedAt().IsZero() {
		t.Error("UpdatedAt still zero after Build")
	}
}
