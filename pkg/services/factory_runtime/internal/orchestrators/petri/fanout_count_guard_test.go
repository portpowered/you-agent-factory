package petri

import (
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestVisitCountGuard_LogicalRoundTripCountsOneCyclePerPair(t *testing.T) {
	guard := &VisitCountGuard{
		TransitionID: "review",
		MaxVisits:    12,
		LogicalRoundTrip: &LogicalRoundTripPolicy{
			Transitions:  [2]string{"process", "review"},
			MaxRawVisits: 24,
		},
	}

	decision := guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 7, "review": 7},
	}})
	if decision.Matched || decision.RawVisits != 14 || decision.LogicalVisits != 7 {
		t.Fatalf("seven paired visits = %#v, want raw=14 logical=7 below limit", decision)
	}

	imbalanced := guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 8, "review": 7},
	}})
	if imbalanced.Matched || imbalanced.LogicalVisits != 7 || imbalanced.RawVisits != 15 {
		t.Fatalf("imbalanced paired visits = %#v, want logical=min(process, review)", imbalanced)
	}
}

func TestVisitCountGuard_LogicalRoundTripReportsLogicalLimitAtBoundary(t *testing.T) {
	guard := &VisitCountGuard{
		TransitionID: "review",
		MaxVisits:    12,
		LogicalRoundTrip: &LogicalRoundTripPolicy{
			Transitions:  [2]string{"process", "review"},
			MaxRawVisits: 24,
		},
	}

	decision := guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 12, "review": 12},
	}})
	if !decision.Matched || decision.Limit != VisitCountLimitLogical {
		t.Fatalf("paired boundary decision = %#v, want logical limit", decision)
	}
	if reason := guard.LimitReason(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 12, "review": 12},
	}}); reason != "logical visit limit reached: 12 >= 12" {
		t.Fatalf("logical limit reason = %q, want stable boundary reason", reason)
	}
}

func TestVisitCountGuard_LogicalRoundTripReportsRawBackstopForImbalancedRoute(t *testing.T) {
	guard := &VisitCountGuard{
		TransitionID: "review",
		MaxVisits:    12,
		LogicalRoundTrip: &LogicalRoundTripPolicy{
			Transitions:  [2]string{"process", "review"},
			MaxRawVisits: 24,
		},
	}

	decision := guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 24},
	}})
	if !decision.Matched || decision.Limit != VisitCountLimitRaw || decision.LogicalVisits != 0 {
		t.Fatalf("imbalanced backstop decision = %#v, want raw limit with no logical cycles", decision)
	}
	if reason := guard.LimitReason(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 24},
	}}); reason != "absolute raw-visit backstop reached: 24 >= 24" {
		t.Fatalf("raw backstop reason = %q, want stable backstop reason", reason)
	}
}

func TestVisitCountGuard_OmittedRoundTripKeepsLegacyRawCounting(t *testing.T) {
	guard := &VisitCountGuard{TransitionID: "review", MaxVisits: 5}
	decision := guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 12, "review": 4},
	}})
	if decision.Matched || decision.RawVisits != 4 || decision.LogicalVisits != 4 {
		t.Fatalf("legacy below-threshold decision = %#v, want watched raw review count", decision)
	}
	decision = guard.Decision(factorytoken.Token{History: factorytoken.History{
		TotalVisits: map[string]int{"process": 12, "review": 5},
	}})
	if !decision.Matched {
		t.Fatalf("legacy threshold decision = %#v, want raw review count at maxVisits", decision)
	}
}

func TestFanoutCountGuard_ExactMatch(t *testing.T) {
	parent := &factorytoken.Token{Color: factorytoken.Color{WorkID: "parent-1"}}
	countToken := &factorytoken.Token{Color: factorytoken.Color{Tags: map[string]string{"expected_count": "3"}}}
	bindings := map[string]*factorytoken.Token{"parent": parent, "fanout-count": countToken}

	candidates := []factorytoken.Token{
		{ID: "c1", Color: factorytoken.Color{ParentID: "parent-1"}},
		{ID: "c2", Color: factorytoken.Color{ParentID: "parent-1"}},
		{ID: "c3", Color: factorytoken.Color{ParentID: "parent-1"}},
		{ID: "c4", Color: factorytoken.Color{ParentID: "other"}},
	}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass with 3 matching children and expected_count=3")
	}
	if len(matched) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matched))
	}
}

func TestFanoutCountGuard_CountMismatch(t *testing.T) {
	parent := &factorytoken.Token{Color: factorytoken.Color{WorkID: "parent-1"}}
	countToken := &factorytoken.Token{Color: factorytoken.Color{Tags: map[string]string{"expected_count": "3"}}}
	bindings := map[string]*factorytoken.Token{"parent": parent, "fanout-count": countToken}

	// Only 2 children — expected 3.
	candidates := []factorytoken.Token{
		{ID: "c1", Color: factorytoken.Color{ParentID: "parent-1"}},
		{ID: "c2", Color: factorytoken.Color{ParentID: "parent-1"}},
	}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when count doesn't match")
	}
}

func TestFanoutCountGuard_ZeroChildren(t *testing.T) {
	parent := &factorytoken.Token{Color: factorytoken.Color{WorkID: "parent-1"}}
	countToken := &factorytoken.Token{Color: factorytoken.Color{Tags: map[string]string{"expected_count": "0"}}}
	bindings := map[string]*factorytoken.Token{"parent": parent, "fanout-count": countToken}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	matched, ok := guard.Evaluate(nil, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass with 0 children and expected_count=0")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestFanoutCountGuard_MissingParentBinding(t *testing.T) {
	countToken := &factorytoken.Token{Color: factorytoken.Color{Tags: map[string]string{"expected_count": "1"}}}
	bindings := map[string]*factorytoken.Token{"fanout-count": countToken}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when parent binding is missing")
	}
}

func TestFanoutCountGuard_MissingCountBinding(t *testing.T) {
	parent := &factorytoken.Token{Color: factorytoken.Color{WorkID: "parent-1"}}
	bindings := map[string]*factorytoken.Token{"parent": parent}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when count binding is missing")
	}
}

func TestFanoutCountGuard_InvalidCountTag(t *testing.T) {
	parent := &factorytoken.Token{Color: factorytoken.Color{WorkID: "parent-1"}}
	countToken := &factorytoken.Token{Color: factorytoken.Color{Tags: map[string]string{"expected_count": "not-a-number"}}}
	bindings := map[string]*factorytoken.Token{"parent": parent, "fanout-count": countToken}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail with invalid expected_count")
	}
}
