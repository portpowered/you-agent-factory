package petri

import (
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestMatchColorGuard_PositiveMatch(t *testing.T) {
	// Parent token bound as "work"
	parent := &interfaces.Token{
		ID: "parent-1",
		Color: interfaces.TokenColor{
			WorkID: "req-100",
		},
	}
	bindings := map[string]*interfaces.Token{"work": parent}

	// Candidates — one matches, one doesn't
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100"}},
		{ID: "child-2", Color: interfaces.TokenColor{ParentID: "req-999"}},
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
	parent := &interfaces.Token{
		ID:    "parent-1",
		Color: interfaces.TokenColor{WorkID: "req-100"},
	}
	bindings := map[string]*interfaces.Token{"work": parent}

	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-999"}},
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
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100"}},
	}

	guard := &MatchColorGuard{
		Field:        "parent_id",
		MatchBinding: "work",
		MatchField:   "work_id",
	}

	matched, ok := guard.Evaluate(candidates, map[string]*interfaces.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestSameNameGuard_PositiveMatch(t *testing.T) {
	bindings := map[string]*interfaces.Token{
		"plan": {ID: "plan-1", Color: interfaces.TokenColor{Name: "shared-name"}},
	}
	candidates := []interfaces.Token{
		{ID: "task-1", Color: interfaces.TokenColor{Name: "shared-name"}},
		{ID: "task-2", Color: interfaces.TokenColor{Name: "other-name"}},
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
	bindings := map[string]*interfaces.Token{
		"plan": {ID: "plan-1", Color: interfaces.TokenColor{Name: "shared-name"}},
	}
	candidates := []interfaces.Token{
		{ID: "task-1", Color: interfaces.TokenColor{Name: "other-name"}},
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
	candidates := []interfaces.Token{
		{ID: "task-1", Color: interfaces.TokenColor{Name: "shared-name"}},
	}

	guard := &SameNameGuard{MatchBinding: "plan"}
	matched, ok := guard.Evaluate(candidates, map[string]*interfaces.Token{}, nil)
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
		binding    *interfaces.Token
		candidates []interfaces.Token
	}{
		{
			name:    "missing bound name",
			binding: &interfaces.Token{ID: "plan-1", Color: interfaces.TokenColor{}},
			candidates: []interfaces.Token{
				{ID: "task-1", Color: interfaces.TokenColor{Name: "shared-name"}},
			},
		},
		{
			name:    "missing candidate name",
			binding: &interfaces.Token{ID: "plan-1", Color: interfaces.TokenColor{Name: "shared-name"}},
			candidates: []interfaces.Token{
				{ID: "task-1", Color: interfaces.TokenColor{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &SameNameGuard{MatchBinding: "plan"}
			matched, ok := guard.Evaluate(tt.candidates, map[string]*interfaces.Token{"plan": tt.binding}, nil)
			if ok {
				t.Fatal("expected guard to fail")
			}
			if len(matched) != 0 {
				t.Fatalf("expected 0 matches, got %d", len(matched))
			}
		})
	}
}

func TestMatchesFieldsGuard_DirectFieldSelector(t *testing.T) {
	guard := &MatchesFieldsGuard{
		InputKey:     ".Name",
		MatchBinding: "source",
	}
	bindings := map[string]*interfaces.Token{
		"source": {ID: "source-1", Color: interfaces.TokenColor{Name: "alpha"}},
	}
	candidates := []interfaces.Token{
		{ID: "candidate-1", Color: interfaces.TokenColor{Name: "alpha"}},
		{ID: "candidate-2", Color: interfaces.TokenColor{Name: "beta"}},
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
	bindings := map[string]*interfaces.Token{
		"source": {ID: "source-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}},
	}
	candidates := []interfaces.Token{
		{ID: "candidate-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}},
		{ID: "candidate-2", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "different"}}},
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
		bindings   map[string]*interfaces.Token
		candidates []interfaces.Token
	}{
		{
			name:       "missing bound tag",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "source"},
			bindings:   map[string]*interfaces.Token{"source": {ID: "source-1", Color: interfaces.TokenColor{}}},
			candidates: []interfaces.Token{{ID: "candidate-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}}},
		},
		{
			name:       "missing candidate tag",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`, MatchBinding: "source"},
			bindings:   map[string]*interfaces.Token{"source": {ID: "source-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}}},
			candidates: []interfaces.Token{{ID: "candidate-1", Color: interfaces.TokenColor{}}},
		},
		{
			name:       "invalid selector",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags[_last_output]`, MatchBinding: "source"},
			bindings:   map[string]*interfaces.Token{"source": {ID: "source-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}}},
			candidates: []interfaces.Token{{ID: "candidate-1", Color: interfaces.TokenColor{Tags: map[string]string{"_last_output": "shared"}}}},
		},
		{
			name:       "single-input selector must resolve",
			guard:      &MatchesFieldsGuard{InputKey: `.Tags["_last_output"]`},
			bindings:   map[string]*interfaces.Token{},
			candidates: []interfaces.Token{{ID: "candidate-1", Color: interfaces.TokenColor{}}},
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
	candidates := []interfaces.Token{
		{
			ID: "tok-1",
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"coding": 5},
			},
		},
		{
			ID: "tok-2",
			History: interfaces.TokenHistory{
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
	candidates := []interfaces.Token{
		{
			ID: "tok-1",
			History: interfaces.TokenHistory{
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

func TestVisitCountGuard_NoVisitHistory(t *testing.T) {
	candidates := []interfaces.Token{
		{ID: "tok-1", History: interfaces.TokenHistory{}},
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
	parent := &interfaces.Token{
		ID:    "parent-1",
		Color: interfaces.TokenColor{WorkID: "req-100"},
	}
	bindings := map[string]*interfaces.Token{"work": parent}

	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100", WorkID: "cc-1"}},
		{ID: "child-2", Color: interfaces.TokenColor{ParentID: "req-100", WorkID: "cc-2"}},
		{ID: "child-3", Color: interfaces.TokenColor{ParentID: "req-200", WorkID: "cc-3"}},
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
	parent := &interfaces.Token{
		ID:    "parent-1",
		Color: interfaces.TokenColor{WorkID: "req-100"},
	}
	bindings := map[string]*interfaces.Token{"work": parent}

	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-200"}},
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
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100"}},
	}

	guard := &AllWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, map[string]*interfaces.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestAnyWithParentGuard_PositiveMatch(t *testing.T) {
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100"}},
		{ID: "child-2", Color: interfaces.TokenColor{ParentID: "req-100"}},
		{ID: "child-3", Color: interfaces.TokenColor{ParentID: "req-999"}},
	}

	bindings := map[string]*interfaces.Token{
		"work": {Color: interfaces.TokenColor{WorkID: "req-100"}},
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
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-999"}},
	}

	bindings := map[string]*interfaces.Token{
		"work": {Color: interfaces.TokenColor{WorkID: "req-100"}},
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
	candidates := []interfaces.Token{
		{ID: "child-1", Color: interfaces.TokenColor{ParentID: "req-100"}},
	}

	guard := &AnyWithParentGuard{MatchBinding: "work"}

	matched, ok := guard.Evaluate(candidates, map[string]*interfaces.Token{}, nil)
	if ok {
		t.Fatal("expected guard to fail when binding is missing")
	}
	if matched != nil {
		t.Fatalf("expected nil matches, got %v", matched)
	}
}

func TestDependencyGuard_AllDependenciesMet(t *testing.T) {
	// Dependency token A is in the required "complete" state.
	depToken := &interfaces.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color: interfaces.TokenColor{
			WorkID:     "work-a",
			WorkTypeID: "task",
		},
	}

	// Candidate B depends on A being in "complete".
	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-a": depToken,
			"tok-b": &candidates[0],
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if !ok {
		t.Fatal("expected guard to pass when dependency is in required state")
	}
	if len(matched) != 1 || matched[0].ID != "tok-b" {
		t.Errorf("expected tok-b matched, got %v", matched)
	}
}

func TestDependencyGuard_DependencyNotMet(t *testing.T) {
	// Dependency token A is in "init" — not in "complete".
	depToken := &interfaces.Token{
		ID:      "tok-a",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-a",
			WorkTypeID: "task",
		},
	}

	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-a": depToken,
			"tok-b": &candidates[0],
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if ok {
		t.Fatal("expected guard to fail when dependency is not in required state")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestDependencyGuard_DependencyNotFound(t *testing.T) {
	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-missing", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-b": &candidates[0],
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if ok {
		t.Fatal("expected guard to fail when dependency token is missing")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestDependencyGuard_NilMarking(t *testing.T) {
	candidates := []interfaces.Token{
		{
			ID: "tok-b",
			Color: interfaces.TokenColor{
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
				},
			},
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, nil)
	if ok {
		t.Fatal("expected guard to fail with nil marking")
	}
	if matched != nil {
		t.Errorf("expected nil matches, got %v", matched)
	}
}

func TestDependencyGuard_NoDependencies(t *testing.T) {
	// Token with no DEPENDS_ON relations should pass.
	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationParentChild, TargetWorkID: "work-a"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-b": &candidates[0],
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if !ok {
		t.Fatal("expected guard to pass for token with no DEPENDS_ON relations")
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestDependencyGuard_MultipleDependencies(t *testing.T) {
	depA := &interfaces.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color:   interfaces.TokenColor{WorkID: "work-a", WorkTypeID: "task"},
	}
	depC := &interfaces.Token{
		ID:      "tok-c",
		PlaceID: "task:complete",
		Color:   interfaces.TokenColor{WorkID: "work-c", WorkTypeID: "task"},
	}

	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-c", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-a": depA,
			"tok-b": &candidates[0],
			"tok-c": depC,
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if !ok {
		t.Fatal("expected guard to pass when all dependencies are met")
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestDependencyGuard_PartialDependenciesMet(t *testing.T) {
	depA := &interfaces.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color:   interfaces.TokenColor{WorkID: "work-a", WorkTypeID: "task"},
	}
	depC := &interfaces.Token{
		ID:      "tok-c",
		PlaceID: "task:init", // NOT complete
		Color:   interfaces.TokenColor{WorkID: "work-c", WorkTypeID: "task"},
	}

	candidates := []interfaces.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []interfaces.Relation{
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
					{Type: interfaces.RelationDependsOn, TargetWorkID: "work-c", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-a": depA,
			"tok-b": &candidates[0],
			"tok-c": depC,
		},
	}

	guard := &DependencyGuard{}
	matched, ok := guard.Evaluate(candidates, nil, marking)
	if ok {
		t.Fatal("expected guard to fail when only some dependencies are met")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
}

func TestFanoutCountGuard_ExactMatch(t *testing.T) {
	parent := &interfaces.Token{Color: interfaces.TokenColor{WorkID: "parent-1"}}
	countToken := &interfaces.Token{Color: interfaces.TokenColor{Tags: map[string]string{"expected_count": "3"}}}
	bindings := map[string]*interfaces.Token{"parent": parent, "fanout-count": countToken}

	candidates := []interfaces.Token{
		{ID: "c1", Color: interfaces.TokenColor{ParentID: "parent-1"}},
		{ID: "c2", Color: interfaces.TokenColor{ParentID: "parent-1"}},
		{ID: "c3", Color: interfaces.TokenColor{ParentID: "parent-1"}},
		{ID: "c4", Color: interfaces.TokenColor{ParentID: "other"}},
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
	parent := &interfaces.Token{Color: interfaces.TokenColor{WorkID: "parent-1"}}
	countToken := &interfaces.Token{Color: interfaces.TokenColor{Tags: map[string]string{"expected_count": "3"}}}
	bindings := map[string]*interfaces.Token{"parent": parent, "fanout-count": countToken}

	// Only 2 children — expected 3.
	candidates := []interfaces.Token{
		{ID: "c1", Color: interfaces.TokenColor{ParentID: "parent-1"}},
		{ID: "c2", Color: interfaces.TokenColor{ParentID: "parent-1"}},
	}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(candidates, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when count doesn't match")
	}
}

func TestFanoutCountGuard_ZeroChildren(t *testing.T) {
	parent := &interfaces.Token{Color: interfaces.TokenColor{WorkID: "parent-1"}}
	countToken := &interfaces.Token{Color: interfaces.TokenColor{Tags: map[string]string{"expected_count": "0"}}}
	bindings := map[string]*interfaces.Token{"parent": parent, "fanout-count": countToken}

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
	countToken := &interfaces.Token{Color: interfaces.TokenColor{Tags: map[string]string{"expected_count": "1"}}}
	bindings := map[string]*interfaces.Token{"fanout-count": countToken}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when parent binding is missing")
	}
}

func TestFanoutCountGuard_MissingCountBinding(t *testing.T) {
	parent := &interfaces.Token{Color: interfaces.TokenColor{WorkID: "parent-1"}}
	bindings := map[string]*interfaces.Token{"parent": parent}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail when count binding is missing")
	}
}

func TestFanoutCountGuard_InvalidCountTag(t *testing.T) {
	parent := &interfaces.Token{Color: interfaces.TokenColor{WorkID: "parent-1"}}
	countToken := &interfaces.Token{Color: interfaces.TokenColor{Tags: map[string]string{"expected_count": "not-a-number"}}}
	bindings := map[string]*interfaces.Token{"parent": parent, "fanout-count": countToken}

	guard := &FanoutCountGuard{MatchBinding: "parent", CountBinding: "fanout-count"}
	_, ok := guard.Evaluate(nil, bindings, nil)
	if ok {
		t.Fatal("expected guard to fail with invalid expected_count")
	}
}

func TestTokenColorField(t *testing.T) {
	color := interfaces.TokenColor{
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
