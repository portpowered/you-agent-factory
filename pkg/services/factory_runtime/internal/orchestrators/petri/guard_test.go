package petri

import (
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestMatchColorGuard_PositiveMatch(t *testing.T) {
	// Parent token bound as "work"
	parent := &factorytoken.Token{
		ID: "parent-1",
		Color: factorytoken.Color{
			WorkID: "req-100",
		},
	}
	bindings := map[string]*factorytoken.Token{"work": parent}

	// Candidates — one matches, one doesn't
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100"}},
		{ID: "child-2", Color: factorytoken.Color{ParentID: "req-999"}},
	}

	guard := &MatchColorGuard{
		Field:        "parent_id",
		MatchBinding: "work",
		MatchField:   "work_id",
	}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "child-1" {
		t.Errorf("expected child-1, got %s", matched[0].ID)
	}
}

func TestMatchColorGuard_NoMatch(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "parent-1",
		Color: factorytoken.Color{WorkID: "req-100"},
	}
	bindings := map[string]*factorytoken.Token{"work": parent}

	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-999"}},
	}

	guard := &MatchColorGuard{
		Field:        "parent_id",
		MatchBinding: "work",
		MatchField:   "work_id",
	}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestMatchColorGuard_MissingBinding(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100"}},
	}

	guard := &MatchColorGuard{
		Field:        "parent_id",
		MatchBinding: "work",
		MatchField:   "work_id",
	}

	matched, ok := guard.Evaluate(candidates, map[string]*factorytoken.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestSameNameGuard_PositiveMatch(t *testing.T) {
	bindings := map[string]*factorytoken.Token{
		"plan": {ID: "plan-1", Color: factorytoken.Color{Name: "shared-name"}},
	}
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{Name: "shared-name"}},
		{ID: "task-2", Color: factorytoken.Color{Name: "other-name"}},
	}

	guard := &SameNameGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "task-1" {
		t.Fatalf("expected task-1, got %s", matched[0].ID)
	}
}

func TestSameNameGuard_NoMatch(t *testing.T) {
	bindings := map[string]*factorytoken.Token{
		"plan": {ID: "plan-1", Color: factorytoken.Color{Name: "shared-name"}},
	}
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{Name: "other-name"}},
	}

	guard := &SameNameGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestSameNameGuard_MissingBinding(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{Name: "shared-name"}},
	}

	guard := &SameNameGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, map[string]*factorytoken.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestSameNameGuard_MissingNameFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		binding    *factorytoken.Token
		candidates []factorytoken.Token
	}{
		{
			name:    "missing bound name",
			binding: &factorytoken.Token{ID: "plan-1", Color: factorytoken.Color{}},
			candidates: []factorytoken.Token{
				{ID: "task-1", Color: factorytoken.Color{Name: "shared-name"}},
			},
		},
		{
			name:    "missing candidate name",
			binding: &factorytoken.Token{ID: "plan-1", Color: factorytoken.Color{Name: "shared-name"}},
			candidates: []factorytoken.Token{
				{ID: "task-1", Color: factorytoken.Color{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &SameNameGuard{MatchBinding: "plan"}
			matched, ok := guard.Evaluate(tt.candidates, map[string]*factorytoken.Token{"plan": tt.binding}, nil)
			if ok {
				t.Fatal("expected guard to fail")
			}
			if len(matched) != 0 {
				t.Fatalf("expected 0 matches, got %d", len(matched))
			}
		})
	}
}

func TestSameTraceIDGuard_PositiveMatchUsesCurrentChainingTraceID(t *testing.T) {
	bindings := map[string]*factorytoken.Token{
		"plan": {ID: "plan-1", Color: factorytoken.Color{
			CurrentChainingTraceID: "chain-shared",
			TraceID:                "trace-legacy-plan",
		}},
	}
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{
			CurrentChainingTraceID: "chain-shared",
			TraceID:                "trace-legacy-task",
		}},
		{ID: "task-2", Color: factorytoken.Color{
			CurrentChainingTraceID: "chain-other",
			TraceID:                "trace-legacy-task",
		}},
	}

	guard := &SameTraceIDGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "task-1" {
		t.Fatalf("expected task-1, got %s", matched[0].ID)
	}
}

func TestSameTraceIDGuard_FallsBackToLegacyTraceID(t *testing.T) {
	bindings := map[string]*factorytoken.Token{
		"plan": {ID: "plan-1", Color: factorytoken.Color{TraceID: "trace-shared"}},
	}
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{TraceID: "trace-shared"}},
		{ID: "task-2", Color: factorytoken.Color{TraceID: "trace-other"}},
	}

	guard := &SameTraceIDGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "task-1" {
		t.Fatalf("expected task-1, got %s", matched[0].ID)
	}
}

func TestSameTraceIDGuard_MissingTraceIdentityFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		binding    *factorytoken.Token
		candidates []factorytoken.Token
	}{
		{
			name:    "missing bound trace identity",
			binding: &factorytoken.Token{ID: "plan-1", Color: factorytoken.Color{}},
			candidates: []factorytoken.Token{
				{ID: "task-1", Color: factorytoken.Color{CurrentChainingTraceID: "chain-shared"}},
			},
		},
		{
			name:    "missing candidate trace identity",
			binding: &factorytoken.Token{ID: "plan-1", Color: factorytoken.Color{CurrentChainingTraceID: "chain-shared"}},
			candidates: []factorytoken.Token{
				{ID: "task-1", Color: factorytoken.Color{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &SameTraceIDGuard{MatchBinding: "plan"}
			matched, ok := guard.Evaluate(tt.candidates, map[string]*factorytoken.Token{"plan": tt.binding}, nil)
			if ok {
				t.Fatal("expected guard to fail")
			}
			if len(matched) != 0 {
				t.Fatalf("expected 0 matches, got %d", len(matched))
			}
		})
	}
}

func TestSameTraceIDGuard_MissingBinding(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "task-1", Color: factorytoken.Color{CurrentChainingTraceID: "chain-shared"}},
	}

	guard := &SameTraceIDGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, map[string]*factorytoken.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestMatchesFieldsGuard_DirectFieldSelector(t *testing.T) {
	guard := &MatchesFieldsGuard{
		InputKey:     ".Name",
		MatchBinding: "source",
	}
	bindings := map[string]*factorytoken.Token{
		"source": {ID: "source-1", Color: factorytoken.Color{Name: "alpha"}},
	}
	candidates := []factorytoken.Token{
		{ID: "candidate-1", Color: factorytoken.Color{Name: "alpha"}},
		{ID: "candidate-2", Color: factorytoken.Color{Name: "beta"}},
	}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected matches-fields guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "candidate-1" {
		t.Fatalf("expected candidate-1, got %s", matched[0].ID)
	}
}

func TestMatchesFieldsGuard_TagSelector(t *testing.T) {
	guard := &MatchesFieldsGuard{
		InputKey:     `.Tags["_last_output"]`,
		MatchBinding: "source",
	}
	bindings := map[string]*factorytoken.Token{
		"source": {ID: "source-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}},
	}
	candidates := []factorytoken.Token{
		{ID: "candidate-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}},
		{ID: "candidate-2", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "different"}}},
	}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected tag-selector matches-fields guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "candidate-1" {
		t.Fatalf("expected candidate-1, got %s", matched[0].ID)
	}
}

func TestMatchesFieldsGuard_MissingRequiredValueFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		guard      *MatchesFieldsGuard
		bindings   map[string]*factorytoken.Token
		candidates []factorytoken.Token
	}{
		{
			name:       "missing bound tag",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "source"},
			bindings:   map[string]*factorytoken.Token{"source": {ID: "source-1", Color: factorytoken.Color{}}},
			candidates: []factorytoken.Token{{ID: "candidate-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}}},
		},
		{
			name:       "missing candidate tag",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "source"},
			bindings:   map[string]*factorytoken.Token{"source": {ID: "source-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}}},
			candidates: []factorytoken.Token{{ID: "candidate-1", Color: factorytoken.Color{}}},
		},
		{
			name:       "invalid selector",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags[_last_output]`, MatchBinding: "source"},
			bindings:   map[string]*factorytoken.Token{"source": {ID: "source-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}}},
			candidates: []factorytoken.Token{{ID: "candidate-1", Color: factorytoken.Color{Tags: map[string]string{"_last_output": "shared"}}}},
		},
		{
			name:       "single-input selector must resolve",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
			bindings:   map[string]*factorytoken.Token{},
			candidates: []factorytoken.Token{{ID: "candidate-1", Color: factorytoken.Color{}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, ok := tt.guard.Evaluate(tt.candidates, tt.bindings, nil)
			if ok {
				t.Fatal("expected matches-fields guard to fail closed")
			}
			if matched != nil {
				t.Fatalf("expected nil matches, got %#v", matched)
			}
		})
	}
}

func TestVisitCountGuard_ExceedsThreshold(t *testing.T) {
	candidates := []factorytoken.Token{
		{
			ID: "tok-1",
			History: factorytoken.History{
				TotalVisits: map[string]int{"coding": 5},
			},
		},
		{
			ID: "tok-2",
			History: factorytoken.History{
				TotalVisits: map[string]int{"coding": 3},
			},
		},
	}

	guard := &VisitCountGuard{
		TransitionID: "coding",
		MaxVisits:    5,
	}

	matched, ok := guard.Evaluate(candidates, nil, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "tok-1" {
		t.Errorf("expected tok-1, got %s", matched[0].ID)
	}
}

func TestVisitCountGuard_BelowThreshold(t *testing.T) {
	candidates := []factorytoken.Token{
		{
			ID: "tok-1",
			History: factorytoken.History{
				TotalVisits: map[string]int{"coding": 2},
			},
		},
	}

	guard := &VisitCountGuard{
		TransitionID: "coding",
		MaxVisits:    5,
	}

	matched, ok := guard.Evaluate(candidates, nil, nil)
	if ok {
		t.Fatal("expected guard to fail")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestVisitCountGuard_InvocationArgumentTightensFixedCeiling(t *testing.T) {
	candidates := []factorytoken.Token{{
		ID: "tok-1",
		Color: factorytoken.Color{InvocationArguments: &work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"maxCycles": {Values: []string{"2"}},
			},
		}},
		History: factorytoken.History{TotalVisits: map[string]int{"planning": 2}},
	}}
	guard := &VisitCountGuard{
		TransitionID:      "planning",
		MaxVisits:         8,
		MaxVisitsArgument: "maxCycles",
	}

	matched, ok := guard.Evaluate(candidates, nil, nil)
	if !ok || len(matched) != 1 {
		t.Fatalf("dynamic visit guard = (%#v, %v), want candidate at caller-selected bound", matched, ok)
	}
}

func TestVisitCountGuard_InvalidInvocationArgumentUsesFixedCeiling(t *testing.T) {
	candidates := []factorytoken.Token{{
		ID: "tok-1",
		Color: factorytoken.Color{InvocationArguments: &work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"maxCycles": {Values: []string{"99"}},
			},
		}},
		History: factorytoken.History{TotalVisits: map[string]int{"planning": 2}},
	}}
	guard := &VisitCountGuard{
		TransitionID:      "planning",
		MaxVisits:         8,
		MaxVisitsArgument: "maxCycles",
	}

	if matched, ok := guard.Evaluate(candidates, nil, nil); ok || len(matched) != 0 {
		t.Fatalf("dynamic visit guard = (%#v, %v), want fixed-ceiling fallback", matched, ok)
	}
}

func TestVisitCountGuard_NoVisitHistory(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "tok-1", History: factorytoken.History{}},
	}

	guard := &VisitCountGuard{
		TransitionID: "coding",
		MaxVisits:    5,
	}

	matched, ok := guard.Evaluate(candidates, nil, nil)
	if ok {
		t.Fatal("expected guard to fail for token with no visit history")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestAllWithParentGuard_PositiveMatch(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "parent-1",
		Color: factorytoken.Color{WorkID: "req-100"},
	}
	bindings := map[string]*factorytoken.Token{"work": parent}

	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100", WorkID: "cc-1"}},
		{ID: "child-2", Color: factorytoken.Color{ParentID: "req-100", WorkID: "cc-2"}},
		{ID: "child-3", Color: factorytoken.Color{ParentID: "req-200", WorkID: "cc-3"}},
	}

	guard := &AllWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}
	if matched[0].ID != "child-1" || matched[1].ID != "child-2" {
		t.Errorf("expected child-1 and child-2, got %s and %s", matched[0].ID, matched[1].ID)
	}
}

func TestAllWithParentGuard_NoMatch(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "parent-1",
		Color: factorytoken.Color{WorkID: "req-100"},
	}
	bindings := map[string]*factorytoken.Token{"work": parent}

	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-200"}},
	}

	guard := &AllWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestAllWithParentGuard_MissingBinding(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100"}},
	}

	guard := &AllWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, map[string]*factorytoken.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestAnyWithParentGuard_PositiveMatch(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100"}},
		{ID: "child-2", Color: factorytoken.Color{ParentID: "req-100"}},
		{ID: "child-3", Color: factorytoken.Color{ParentID: "req-999"}},
	}

	bindings := map[string]*factorytoken.Token{
		"work": {Color: factorytoken.Color{WorkID: "req-100"}},
	}

	guard := &AnyWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if !ok {
		t.Fatal("expected guard to pass")
	}
	// AnyWithParentGuard returns only the first match.
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != "child-1" {
		t.Errorf("expected first matching child, got %s", matched[0].ID)
	}
}

func TestAnyWithParentGuard_NoMatch(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-999"}},
	}

	bindings := map[string]*factorytoken.Token{
		"work": {Color: factorytoken.Color{WorkID: "req-100"}},
	}

	guard := &AnyWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail")
	}
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
}

func TestAnyWithParentGuard_MissingBinding(t *testing.T) {
	candidates := []factorytoken.Token{
		{ID: "child-1", Color: factorytoken.Color{ParentID: "req-100"}},
	}

	guard := &AnyWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, map[string]*factorytoken.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestTokenColorField(t *testing.T) {
	color := factorytoken.Color{
		WorkID:     "w-1",
		WorkTypeID: "wt-1",
		TraceID:    "t-1",
		ParentID:   "p-1",
	}

	tests := []struct {
		field string
		want  string
	}{
		{"work_id", "w-1"},
		{"work_type_id", "wt-1"},
		{"trace_id", "t-1"},
		{"parent_id", "p-1"},
		{"unknown_field", ""},
	}

	for _, tt := range tests {
		got := tokenColorField(color, tt.field)
		if got != tt.want {
			t.Errorf("tokenColorField(%q) = %q, want %q", tt.field, got, tt.want)
		}
	}
}
