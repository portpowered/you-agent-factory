package petri

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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

func TestAllWithParentGuard_BlocksWhenRegisteredSiblingIsProcessing(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "parent-token",
		Color: factorytoken.Color{WorkID: "parent-1"},
	}
	terminal := factorytoken.Token{
		ID:      "child-terminal",
		PlaceID: "child:complete",
		Color: factorytoken.Color{
			WorkID:     "child-1",
			WorkTypeID: "child",
			ParentID:   "parent-1",
		},
	}
	processing := terminal
	processing.ID = "child-processing"
	processing.PlaceID = "child:processing"
	processing.Color.WorkID = "child-2"

	guard := &AllWithParentGuard{MatchBinding: "parent"}
	matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
			"parent-1": {Children: []factorytoken.Token{terminal, processing}, Complete: true},
		}},
		[]factorytoken.Token{terminal},
		map[string]*factorytoken.Token{"parent": parent},
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			terminal.ID:   &terminal,
			processing.ID: &processing,
		}},
	)
	if ok || len(matched) != 0 {
		t.Fatalf("fan-in enabled with a processing sibling: matched=%v ok=%t", matched, ok)
	}
}

func TestAllWithParentGuard_LateRegistrationStaysBlockedUntilTerminal(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "parent-token",
		Color: factorytoken.Color{WorkID: "parent-1"},
	}
	first := factorytoken.Token{
		ID:      "child-first",
		PlaceID: "child:complete",
		Color:   factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"},
	}
	late := first
	late.ID = "child-late"
	late.Color.WorkID = "child-2"
	late.PlaceID = "child:processing"
	guard := &AllWithParentGuard{MatchBinding: "parent"}
	bindings := map[string]*factorytoken.Token{"parent": parent}

	initial := MarkingSnapshot{Tokens: map[string]*factorytoken.Token{first.ID: &first}}
	if matched, ok := guard.EvaluateRuntime(RuntimeGuardContext{}, []factorytoken.Token{first}, bindings, &initial); ok || len(matched) != 0 {
		t.Fatalf("runtime fan-in should fail closed without a registration projection: matched=%v ok=%t", matched, ok)
	}
	initialProjection := ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first}, Complete: false},
	}
	if matched, ok := guard.EvaluateRuntime(RuntimeGuardContext{ParentChildRegistrations: initialProjection}, []factorytoken.Token{first}, bindings, &initial); ok || len(matched) != 0 {
		t.Fatalf("fan-in should fail closed before the registered set is complete: matched=%v ok=%t", matched, ok)
	}

	withLateChild := MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		first.ID: &first,
		late.ID:  &late,
	}}
	lateProjection := ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first, late}, Complete: false},
	}
	if matched, ok := guard.EvaluateRuntime(RuntimeGuardContext{ParentChildRegistrations: lateProjection}, []factorytoken.Token{first}, bindings, &withLateChild); ok || len(matched) != 0 {
		t.Fatalf("late processing child should keep fan-in blocked until registration closes: matched=%v ok=%t", matched, ok)
	}

	late.PlaceID = "child:complete"
	terminalSnapshot := MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		first.ID: &first,
		late.ID:  &late,
	}}
	matched, ok := guard.EvaluateRuntime(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first, late}, Complete: true},
	}}, []factorytoken.Token{first, late}, bindings, &terminalSnapshot)
	if !ok || len(matched) != 2 {
		t.Fatalf("fan-in should enable exactly after the late child becomes terminal: matched=%v ok=%t", matched, ok)
	}
}

func TestAllWithParentGuard_UsesActiveDispatchAsProcessingChild(t *testing.T) {
	parent := &factorytoken.Token{ID: "parent-token", Color: factorytoken.Color{WorkID: "parent-1"}}
	terminal := factorytoken.Token{
		ID:      "child-terminal",
		PlaceID: "child:complete",
		Color:   factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"},
	}
	processing := terminal
	processing.ID = "child-processing"
	processing.Color.WorkID = "child-2"
	processing.PlaceID = "child:processing"

	guard := &AllWithParentGuard{MatchBinding: "parent"}
	matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{
			ActiveDispatches: map[string]*interfaces.DispatchEntry{
				"dispatch-child": {ConsumedTokens: []workerexecution.Token{factorytoken.ToWorker(processing)}},
			},
			ParentChildRegistrations: ParentChildRegistrationProjection{
				"parent-1": {Children: []factorytoken.Token{terminal, processing}, Complete: true},
			},
		},
		[]factorytoken.Token{terminal},
		map[string]*factorytoken.Token{"parent": parent},
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{terminal.ID: &terminal}},
	)
	if ok || len(matched) != 0 {
		t.Fatalf("fan-in enabled while a processing child was in flight: matched=%v ok=%t", matched, ok)
	}
}

func TestAllWithParentGuard_RuntimeStateAndPopulationChecks(t *testing.T) {
	parent := &factorytoken.Token{ID: "parent-token", Color: factorytoken.Color{WorkID: "parent-1"}}
	first := factorytoken.Token{
		ID:      "child-first",
		PlaceID: "child:complete",
		Color:   factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"},
	}
	second := first
	second.ID = "child-second"
	second.Color.WorkID = "child-2"
	bindings := map[string]*factorytoken.Token{"parent": parent}
	category := func(placeID string) string {
		if placeID == "child:complete" {
			return runtimeStateCategoryTerminal
		}
		return "PROCESSING"
	}
	guard := &AllWithParentGuard{MatchBinding: "parent"}
	projection := ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first, second}, Complete: true},
	}

	snapshot := &MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		first.ID:  &first,
		second.ID: &second,
	}}
	matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{StateCategoryForPlace: category, ParentChildRegistrations: projection},
		[]factorytoken.Token{first, second},
		bindings,
		snapshot,
	)
	if !ok || len(matched) != 2 {
		t.Fatalf("terminal registered children = %v, %t; want both children", matched, ok)
	}

	processingCandidate := second
	processingCandidate.PlaceID = "child:processing"
	if matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{StateCategoryForPlace: category, ParentChildRegistrations: projection},
		[]factorytoken.Token{first, processingCandidate},
		bindings,
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			first.ID:  &first,
			second.ID: &second,
		}},
	); ok || len(matched) != 0 {
		t.Fatalf("non-terminal candidate passed runtime state check: %v, %t", matched, ok)
	}

	processingRegistered := second
	processingRegistered.PlaceID = "child:processing"
	if matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{StateCategoryForPlace: category, ParentChildRegistrations: projection},
		[]factorytoken.Token{first, second},
		bindings,
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			first.ID:  &first,
			second.ID: &processingRegistered,
		}},
	); ok || len(matched) != 0 {
		t.Fatalf("non-terminal registered child passed runtime state check: %v, %t", matched, ok)
	}

	if matched, ok := guard.Evaluate(
		[]factorytoken.Token{first},
		bindings,
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{}},
	); ok || len(matched) != 0 {
		t.Fatalf("incomplete runtime population passed: %v, %t", matched, ok)
	}
}

func TestAllWithParentGuard_RejectsUnknownAndMismatchedRegistrationProjection(t *testing.T) {
	parent := &factorytoken.Token{ID: "parent-token", Color: factorytoken.Color{WorkID: "parent-1"}}
	first := factorytoken.Token{
		ID:      "child-first",
		PlaceID: "child:complete",
		Color:   factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"},
	}
	second := first
	second.ID = "child-second"
	second.Color.WorkID = "child-2"
	bindings := map[string]*factorytoken.Token{"parent": parent}
	guard := &AllWithParentGuard{MatchBinding: "parent"}

	assertBlocked := func(ctx RuntimeGuardContext, candidates []factorytoken.Token, marking *MarkingSnapshot) {
		t.Helper()
		matched, ok := guard.EvaluateRuntime(ctx, candidates, bindings, marking)
		if ok || len(matched) != 0 {
			t.Fatalf("mismatched registration projection enabled fan-in: matched=%v ok=%t", matched, ok)
		}
	}

	baseSnapshot := func(tokens ...factorytoken.Token) *MarkingSnapshot {
		snapshot := &MarkingSnapshot{Tokens: make(map[string]*factorytoken.Token, len(tokens))}
		for i := range tokens {
			token := tokens[i]
			snapshot.Tokens[token.ID] = &token
		}
		return snapshot
	}

	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"other-parent": {Children: []factorytoken.Token{first}, Complete: true},
	}}, []factorytoken.Token{first}, baseSnapshot(first))
	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first}, Complete: false},
	}}, []factorytoken.Token{first}, baseSnapshot(first))
	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Complete: true},
	}}, []factorytoken.Token{first}, baseSnapshot(first))

	// A visible child outside the registered set is a population mismatch.
	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first}, Complete: true},
	}}, []factorytoken.Token{first}, baseSnapshot(first, second))
	// A registered child that is not visible is also incomplete.
	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first, second}, Complete: true},
	}}, []factorytoken.Token{first}, baseSnapshot(first))
	// The visible runtime identity must agree with the registration identity.
	assertBlocked(RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"parent-1": {Children: []factorytoken.Token{first}, Complete: true},
	}}, []factorytoken.Token{second}, baseSnapshot(first))
}

func TestAllWithParentGuard_DirectEvaluationChecksSnapshotPopulation(t *testing.T) {
	parent := &factorytoken.Token{ID: "parent-token", Color: factorytoken.Color{WorkID: "parent-1"}}
	terminal := factorytoken.Token{
		ID:      "child-terminal",
		PlaceID: "child:complete",
		Color:   factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"},
	}
	processing := terminal
	processing.ID = "child-processing"
	processing.Color.WorkID = "child-2"
	processing.PlaceID = "child:processing"
	guard := &AllWithParentGuard{MatchBinding: "parent"}
	bindings := map[string]*factorytoken.Token{"parent": parent}

	matched, ok := guard.Evaluate(
		[]factorytoken.Token{terminal},
		bindings,
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			terminal.ID:   &terminal,
			processing.ID: &processing,
		}},
	)
	if ok || len(matched) != 0 {
		t.Fatalf("direct evaluation ignored a sibling in another place: matched=%v ok=%t", matched, ok)
	}

	matched, ok = guard.Evaluate(
		[]factorytoken.Token{terminal},
		bindings,
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{terminal.ID: &terminal}},
	)
	if !ok || len(matched) != 1 || matched[0].ID != terminal.ID {
		t.Fatalf("direct evaluation with one visible terminal child = %v, %t; want terminal child", matched, ok)
	}
}

func TestParentChildTokenHelpersFilterAndDeduplicateRuntimeFacts(t *testing.T) {
	valid := factorytoken.Token{ID: "valid", PlaceID: "child:complete", Color: factorytoken.Color{WorkID: "child-1", WorkTypeID: "child", ParentID: "parent-1"}}
	resource := valid
	resource.ID = "resource"
	resource.Color.DataType = factorytoken.DataTypeResource
	wrongParent := valid
	wrongParent.ID = "wrong-parent"
	wrongParent.Color.ParentID = "parent-2"
	wrongType := valid
	wrongType.ID = "wrong-type"
	wrongType.Color.WorkTypeID = "other"
	missingWorkID := valid
	missingWorkID.ID = "missing-work-id"
	missingWorkID.Color.WorkID = ""

	children := parentChildTokens(
		&MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			valid.ID:         &valid,
			"nil-token":      nil,
			resource.ID:      &resource,
			wrongParent.ID:   &wrongParent,
			wrongType.ID:     &wrongType,
			missingWorkID.ID: &missingWorkID,
		}},
		map[string]*interfaces.DispatchEntry{
			"nil-dispatch":      nil,
			"matching-dispatch": {ConsumedTokens: []workerexecution.Token{factorytoken.ToWorker(valid)}},
		},
		"parent-1",
		"child",
	)
	if len(children) != 1 || children["work:child-1"].ID != valid.ID {
		t.Fatalf("filtered child population = %#v, want one deduplicated child", children)
	}

	if got := matchingParentChildren([]factorytoken.Token{valid, wrongType}, "parent-1", "child"); len(got) != 1 || got[0].ID != valid.ID {
		t.Fatalf("typed parent matches = %#v, want valid child only", got)
	}
	if got := tokenIdentity(factorytoken.Token{ID: "token-only"}); got != "token:token-only" {
		t.Fatalf("token-only identity = %q, want token:token-only", got)
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
