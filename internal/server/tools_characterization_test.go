package server

import (
	"context"
	"testing"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// The tests in this file pin behaviour that is not obvious from the code and
// that a refactor of the rule/target filtering could change silently.

func rulesFixture() promv1.RulesResult {
	return promv1.RulesResult{Groups: []promv1.RuleGroup{
		{
			Name: "mixed",
			Rules: promv1.Rules{
				promv1.AlertingRule{Name: "alert1"},
				promv1.RecordingRule{Name: "record1"},
			},
		},
		{
			Name:  "alerts-only",
			Rules: promv1.Rules{promv1.AlertingRule{Name: "alert2"}},
		},
	}}
}

func TestFilterRulesKeepsOnlyRequestedKindAndDropsEmptyGroups(t *testing.T) {
	tests := map[string]struct {
		filter    string
		wantNames []string // rule names, in group order
	}{
		"alert":  {filter: "alert", wantNames: []string{"alert1", "alert2"}},
		"record": {filter: "record", wantNames: []string{"record1"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out := filterRules(rulesFixture(), tc.filter)

			var got []string
			for _, g := range out.Groups {
				if len(g.Rules) == 0 {
					t.Errorf("group %q survived with no rules", g.Name)
				}
				for _, r := range g.Rules {
					switch v := r.(type) {
					case promv1.AlertingRule:
						got = append(got, v.Name)
					case promv1.RecordingRule:
						got = append(got, v.Name)
					default:
						t.Fatalf("unexpected rule type %T", r)
					}
				}
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("kept %v, want %v", got, tc.wantNames)
			}
			for i, w := range tc.wantNames {
				if got[i] != w {
					t.Errorf("rule %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// unknownRule stands in for a rule kind the Prometheus client may add later.
type unknownRule struct{ Name string }

func TestFilterRulesKeepsUnknownRuleKinds(t *testing.T) {
	in := promv1.RulesResult{Groups: []promv1.RuleGroup{{
		Name:  "g",
		Rules: promv1.Rules{unknownRule{Name: "mystery"}, promv1.RecordingRule{Name: "record1"}},
	}}}

	out := filterRules(in, "alert")
	if len(out.Groups) != 1 {
		t.Fatalf("groups = %d, want 1 (the unknown rule keeps the group alive)", len(out.Groups))
	}
	if len(out.Groups[0].Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(out.Groups[0].Rules))
	}
	if _, ok := out.Groups[0].Rules[0].(unknownRule); !ok {
		t.Errorf("kept %T, want the unknown rule kind", out.Groups[0].Rules[0])
	}
}

func TestRulesRejectsUnknownTypeFilter(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	_, handler := s.toolRules()

	if _, err := call(t, handler, rulesArgs{Type: "bogus"}); err == nil {
		t.Error("expected an error for an unknown rule type filter")
	}
}

func TestRulesAllReturnsEveryKind(t *testing.T) {
	api := &fakeAPI{RulesFn: func(context.Context) (promv1.RulesResult, error) {
		return rulesFixture(), nil
	}}
	s := newTestServer(api)
	_, handler := s.toolRules()

	for _, filter := range []string{"", "all"} {
		payload := mustCall(t, handler, rulesArgs{Type: filter})
		groups := payload["groups"].([]any)
		if len(groups) != 2 {
			t.Errorf("type=%q returned %d groups, want 2", filter, len(groups))
		}
	}
}

func targetsFixture(active, dropped int) promv1.TargetsResult {
	res := promv1.TargetsResult{}
	for i := 0; i < active; i++ {
		res.Active = append(res.Active, promv1.ActiveTarget{ScrapeURL: "http://active"})
	}
	for i := 0; i < dropped; i++ {
		res.Dropped = append(res.Dropped, promv1.DroppedTarget{DiscoveredLabels: map[string]string{"job": "d"}})
	}
	return res
}

func TestTargetsPayloadKeysPerState(t *testing.T) {
	tests := map[string]struct {
		state       string
		limit       *int
		wantKeys    []string
		absentKeys  []string
		wantActive  float64
		wantDropped float64
	}{
		"default is active": {
			state:      "",
			wantKeys:   []string{"active", "active_total"},
			absentKeys: []string{"dropped", "dropped_total", "active_truncated"},
			wantActive: 3,
		},
		"explicit active": {
			state:      "active",
			wantKeys:   []string{"active", "active_total"},
			absentKeys: []string{"dropped", "dropped_total"},
			wantActive: 3,
		},
		"dropped": {
			state:       "dropped",
			wantKeys:    []string{"dropped", "dropped_total"},
			absentKeys:  []string{"active", "active_total"},
			wantDropped: 2,
		},
		"all": {
			state:       "all",
			wantKeys:    []string{"active", "active_total", "dropped", "dropped_total"},
			wantActive:  3,
			wantDropped: 2,
		},
		"truncation flags both states": {
			state:       "all",
			limit:       intPtr(1),
			wantKeys:    []string{"active_truncated", "dropped_truncated"},
			wantActive:  3,
			wantDropped: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{TargetsFn: func(context.Context) (promv1.TargetsResult, error) {
				return targetsFixture(3, 2), nil
			}}
			s := newTestServer(api)
			_, handler := s.toolTargets()

			payload := mustCall(t, handler, targetsArgs{State: tc.state, Limit: tc.limit})
			for _, k := range tc.wantKeys {
				if _, ok := payload[k]; !ok {
					t.Errorf("payload is missing key %q (got %v)", k, payload)
				}
			}
			for _, k := range tc.absentKeys {
				if _, ok := payload[k]; ok {
					t.Errorf("payload should not contain key %q", k)
				}
			}
			// Totals always report the pre-truncation count.
			if tc.wantActive != 0 && payload["active_total"].(float64) != tc.wantActive {
				t.Errorf("active_total = %v, want %v", payload["active_total"], tc.wantActive)
			}
			if tc.wantDropped != 0 && payload["dropped_total"].(float64) != tc.wantDropped {
				t.Errorf("dropped_total = %v, want %v", payload["dropped_total"], tc.wantDropped)
			}
		})
	}
}

func TestTargetsRejectsUnknownState(t *testing.T) {
	s := newTestServer(&fakeAPI{})
	_, handler := s.toolTargets()

	if _, err := call(t, handler, targetsArgs{State: "bogus"}); err == nil {
		t.Error("expected an error for an unknown target state")
	}
}

func TestBoundedLimit(t *testing.T) {
	tests := map[string]struct {
		in   *int
		def  int
		want int
	}{
		"absent uses default":   {in: nil, def: 42, want: 42},
		"negative uses default": {in: intPtr(-7), def: 42, want: 42},
		"zero means unlimited":  {in: intPtr(0), def: 42, want: 0},
		"explicit value wins":   {in: intPtr(5), def: 42, want: 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := boundedLimit(tc.in, tc.def); got != tc.want {
				t.Errorf("boundedLimit(%v, %d) = %d, want %d", tc.in, tc.def, got, tc.want)
			}
		})
	}
}
