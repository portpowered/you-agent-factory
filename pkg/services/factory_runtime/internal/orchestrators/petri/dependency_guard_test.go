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
