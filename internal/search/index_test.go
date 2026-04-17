package search

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"http_request_duration_seconds", []string{"http", "request", "duration", "seconds"}},
		{"nodeMemoryMemFreeBytes", []string{"node", "memory", "mem", "free", "bytes"}},
		{"  Multiple  spaces!! ", []string{"multiple", "spaces"}},
		{"", nil},
	}
	for _, tc := range cases {
		got := tokenize(tc.in)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSearchRanksNameMatchesFirst(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "http_request_duration_seconds", Type: "histogram", Help: "Duration of HTTP requests."},
		{Metric: "http_requests_total", Type: "counter", Help: "Total HTTP requests."},
		{Metric: "node_memory_MemFree_bytes", Type: "gauge", Help: "Amount of free memory on the node."},
		{Metric: "process_cpu_seconds_total", Type: "counter", Help: "Total CPU time consumed."},
	})

	hits := idx.Search("http request latency", 10)
	if len(hits) == 0 {
		t.Fatal("expected hits, got none")
	}
	if hits[0].Metric != "http_request_duration_seconds" {
		t.Errorf("top hit for 'http request latency' was %q, want http_request_duration_seconds", hits[0].Metric)
	}
}

func TestSearchMatchesHelpText(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "node_memory_MemFree_bytes", Help: "Amount of free memory on the node."},
		{Metric: "process_cpu_seconds_total", Help: "Total CPU time consumed."},
	})

	hits := idx.Search("free memory", 5)
	if len(hits) == 0 || hits[0].Metric != "node_memory_MemFree_bytes" {
		t.Fatalf("expected node_memory_MemFree_bytes first, got %+v", hits)
	}
}

func TestSearchEmptyIndex(t *testing.T) {
	idx := NewIndex()
	if hits := idx.Search("anything", 10); hits != nil {
		t.Errorf("empty index returned %v, want nil", hits)
	}
}

func TestSearchUnknownTerm(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{{Metric: "up"}})
	if hits := idx.Search("nonexistent", 10); len(hits) != 0 {
		t.Errorf("unknown term returned %v, want empty", hits)
	}
}

func TestSearchLimit(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "http_requests_total", Help: "http requests"},
		{Metric: "http_request_duration_seconds", Help: "http request duration"},
		{Metric: "http_response_size_bytes", Help: "http response size"},
	})
	hits := idx.Search("http", 2)
	if len(hits) != 2 {
		t.Errorf("limit=2 returned %d hits", len(hits))
	}
}

func TestSearchScoresDescending(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Document{
		{Metric: "http_requests_total", Help: "total http requests"},
		{Metric: "http_request_duration_seconds", Help: "http request latency"},
		{Metric: "node_cpu_seconds_total", Help: "cpu time per mode"},
	})
	hits := idx.Search("http request", 0)
	for i := 1; i < len(hits); i++ {
		if hits[i-1].Score < hits[i].Score {
			t.Fatalf("scores not descending: %+v", hits)
		}
	}
}
