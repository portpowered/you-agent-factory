package petri

import (
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestDependencyGuard_AllDependenciesMet(t *testing.T) {
	// Dependency token A is in the required "complete" state.
	depToken := &factorytoken.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color: factorytoken.Color{
			WorkID:     "work-a",
			WorkTypeID: "task",
		},
	}

	// Candidate B depends on A being in "complete".
	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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
	depToken := &factorytoken.Token{
		ID:      "tok-a",
		PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID:     "work-a",
			WorkTypeID: "task",
		},
	}

	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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
	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-missing", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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
	candidates := []factorytoken.Token{
		{
			ID: "tok-b",
			Color: factorytoken.Color{
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
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
	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationParentChild, TargetWorkID: "work-a"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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
	depA := &factorytoken.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color:   factorytoken.Color{WorkID: "work-a", WorkTypeID: "task"},
	}
	depC := &factorytoken.Token{
		ID:      "tok-c",
		PlaceID: "task:complete",
		Color:   factorytoken.Color{WorkID: "work-c", WorkTypeID: "task"},
	}

	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
					{Type: work.RelationDependsOn, TargetWorkID: "work-c", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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
	depA := &factorytoken.Token{
		ID:      "tok-a",
		PlaceID: "task:complete",
		Color:   factorytoken.Color{WorkID: "work-a", WorkTypeID: "task"},
	}
	depC := &factorytoken.Token{
		ID:      "tok-c",
		PlaceID: "task:init", // NOT complete
		Color:   factorytoken.Color{WorkID: "work-c", WorkTypeID: "task"},
	}

	candidates := []factorytoken.Token{
		{
			ID:      "tok-b",
			PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID:     "work-b",
				WorkTypeID: "task",
				Relations: []work.Relation{
					{Type: work.RelationDependsOn, TargetWorkID: "work-a", RequiredState: "complete"},
					{Type: work.RelationDependsOn, TargetWorkID: "work-c", RequiredState: "complete"},
				},
			},
		},
	}

	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
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

func TestDependencyGuard_AllDependenciesMetAcrossBinding(t *testing.T) {
	primary := factorytoken.Token{
		ID:      "tok-primary",
		PlaceID: "plan:init",
		Color: factorytoken.Color{
			WorkID:     "work-primary",
			WorkTypeID: "plan",
		},
	}
	secondary := factorytoken.Token{
		ID:      "tok-secondary",
		PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID:     "work-secondary",
			WorkTypeID: "task",
			Relations: []work.Relation{{
				Type:          work.RelationDependsOn,
				TargetWorkID:  "work-prerequisite",
				RequiredState: "complete",
			}},
		},
	}
	prerequisite := &factorytoken.Token{
		ID:      "tok-prerequisite",
		PlaceID: "prerequisite:pending",
		Color: factorytoken.Color{
			WorkID:     "work-prerequisite",
			WorkTypeID: "prerequisite",
		},
	}
	marking := &MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
			primary.ID:      &primary,
			secondary.ID:    &secondary,
			prerequisite.ID: prerequisite,
		},
	}
	bindings := map[string][]factorytoken.Token{
		"primary":   {primary},
		"secondary": {secondary},
	}
	guard := &DependencyGuard{}
	if guard.AllDependenciesMet(bindings, marking) {
		t.Fatal("expected the secondary input dependency to block the complete binding")
	}

	completed := *prerequisite
	completed.PlaceID = "prerequisite:complete"
	marking.Tokens[completed.ID] = &completed
	if !guard.AllDependenciesMet(bindings, marking) {
		t.Fatal("expected the complete binding to pass after the prerequisite reaches complete")
	}
}

func TestSameNameGuardRuntime_UsesOrderedRegisteredParentChild(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "a-token-history",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "z-token-current"
	current.Color.WorkID = "cycle-93"

	guard := &SameNameGuard{MatchBinding: "project"}
	marking := &MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		historical.ID: &historical,
		current.ID:    &current,
	}}
	matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
			"project-work": {
				Children: []factorytoken.Token{historical, current},
				Complete: true,
			},
		}},
		[]factorytoken.Token{current, historical},
		map[string]*factorytoken.Token{"project": parent},
		marking,
	)
	if !ok || len(matched) != 1 || matched[0].Color.WorkID != "cycle-93" {
		t.Fatalf("same-name runtime binding = %#v, %t; want only registered current cycle-93", matched, ok)
	}
}

func TestSameNameGuardRuntime_PreservesUnrelatedEquality(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "shared-name"},
	}
	candidates := []factorytoken.Token{
		{ID: "unrelated-a", Color: factorytoken.Color{Name: "shared-name"}},
		{ID: "unrelated-b", Color: factorytoken.Color{Name: "shared-name", ParentID: "other-parent"}},
	}

	matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(
		RuntimeGuardContext{},
		candidates,
		map[string]*factorytoken.Token{"project": parent},
		nil,
	)
	if !ok || len(matched) != len(candidates) {
		t.Fatalf("unrelated same-name runtime match = %#v, %t; want equality-compatible candidates", matched, ok)
	}
}

func TestSameNameGuardRuntime_CanceledCurrentAdmissionDoesNotFallback(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "a-token-history",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "z-token-canceled"
	current.Color.WorkID = "cycle-93"
	marking := &MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		historical.ID: &historical,
		current.ID:    &current,
	}}
	matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(
		RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
			"project-work": {Children: []factorytoken.Token{historical, current}, Complete: false},
		}},
		[]factorytoken.Token{historical, current},
		map[string]*factorytoken.Token{"project": parent},
		marking,
	)
	if ok || len(matched) != 0 {
		t.Fatalf("canceled current admission fell back to historical cycle: matched=%#v ok=%t", matched, ok)
	}
}

func TestSameNameGuardRuntime_FailsClosedForIncompleteParentRegistration(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "cycle-history-token",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "cycle-current-token"
	current.Color.WorkID = "cycle-93"
	bindings := map[string]*factorytoken.Token{"project": parent}

	baseSnapshot := func(tokens ...factorytoken.Token) *MarkingSnapshot {
		snapshot := &MarkingSnapshot{Tokens: make(map[string]*factorytoken.Token, len(tokens))}
		for index := range tokens {
			token := tokens[index]
			snapshot.Tokens[token.ID] = &token
		}
		return snapshot
	}
	assertBlocked := func(name string, ctx RuntimeGuardContext, candidates []factorytoken.Token, marking *MarkingSnapshot) {
		t.Run(name, func(t *testing.T) {
			matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(ctx, candidates, bindings, marking)
			if ok || len(matched) != 0 {
				t.Fatalf("incomplete parent registration enabled join: matched=%#v ok=%t", matched, ok)
			}
		})
	}

	assertBlocked("missing projection", RuntimeGuardContext{}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
	assertBlocked("incomplete projection", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical, current}, Complete: false},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
	assertBlocked("registered child is not visible", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical, current}, Complete: true},
	}}, []factorytoken.Token{historical}, baseSnapshot(historical))
	assertBlocked("visible child is not registered", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical}, Complete: true},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))

	contradictory := historical
	contradictory.Color.ParentID = "other-parent"
	assertBlocked("registration has contradictory parent", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{contradictory, current}, Complete: true},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
}
