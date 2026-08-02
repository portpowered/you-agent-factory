package petri

import (
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

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
