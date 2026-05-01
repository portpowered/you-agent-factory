package config

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/workstationconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestConfigMapping_SimplePath(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "transformer",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	expectedNet := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete": {ID: "task:complete", TypeID: "task", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"transformer": {ID: "transformer", Name: "transformer",
				InputArcs: []petri.Arc{
					{Name: "task:init:to:transformer", PlaceID: "task:init", TransitionID: "transformer"},
				},
				OutputArcs: []petri.Arc{
					{Name: "task:complete:from:transformer", PlaceID: "task:complete", TransitionID: "transformer"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	assertEquality(t, expectedNet, outputNet)
}

func TestConfigMapping_RejectionAndFailure(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
				OnFailure:   &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Verify the transition has rejection and failure arcs.
	tr := outputNet.Transitions["processor"]
	if tr == nil {
		t.Fatal("expected transition 'processor' to exist")
	}

	if len(tr.RejectionArcs) != 1 {
		t.Fatalf("expected 1 rejection arc, got %d", len(tr.RejectionArcs))
	}
	if tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Errorf("rejection arc place: expected task:init, got %s", tr.RejectionArcs[0].PlaceID)
	}
	if tr.RejectionArcs[0].Name != "task:init:rejection:processor" {
		t.Errorf("rejection arc name: expected task:init:rejection:processor, got %s", tr.RejectionArcs[0].Name)
	}

	if len(tr.FailureArcs) != 1 {
		t.Fatalf("expected 1 failure arc, got %d", len(tr.FailureArcs))
	}
	if tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Errorf("failure arc place: expected task:failed, got %s", tr.FailureArcs[0].PlaceID)
	}
	if tr.FailureArcs[0].Name != "task:failed:failure:processor" {
		t.Errorf("failure arc name: expected task:failed:failure:processor, got %s", tr.FailureArcs[0].Name)
	}
}

func TestConfigMapping_RejectionLoopWithGuardedLoopBreaker(t *testing.T) {
	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), rejectionLoopWithGuardedLoopBreakerFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	if len(outputNet.Transitions) != 2 {
		t.Fatalf("expected only authored reviewer transitions, got %d", len(outputNet.Transitions))
	}
	assertNoTransitionExhaustion(t, outputNet.Transitions)
	assertReviewerRejectionTransition(t, outputNet.Transitions["reviewer"])
	assertGuardedLoopBreakerTransition(t, outputNet.Transitions["reviewer-loop-breaker"], "task:init", "task:failed", "reviewer", 3)
}

func rejectionLoopWithGuardedLoopBreakerFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:        "reviewer",
				Inputs:      []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:     []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
			},
			{
				Name:    "reviewer-loop-breaker",
				Type:    interfaces.WorkstationTypeLogical,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
				Guards: []interfaces.GuardConfig{{
					Type:        interfaces.GuardTypeVisitCount,
					Workstation: "reviewer",
					MaxVisits:   3,
				}},
			},
		},
	}
}

func assertReviewerRejectionTransition(t *testing.T, transition *petri.Transition) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected transition 'reviewer' to exist")
	}
	if len(transition.RejectionArcs) != 1 {
		t.Fatalf("expected 1 rejection arc on reviewer, got %d", len(transition.RejectionArcs))
	}
}

func TestConfigMapping_ValidationRejectsInvalidOnRejection(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "nonexistent"},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for on_rejection pointing to non-existent state")
	}
}

func TestConfigMapping_ValidationRejectsInvalidOnFailure(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnFailure: &interfaces.IOConfig{WorkTypeName: "nonexistent-type", StateName: "failed"},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for on_failure referencing non-existent work type")
	}
}

func TestConfigMapping_VisitCountGuardOnWorkstation(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "coding",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "review", WorkTypeName: "task"},
				},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
			},
			{
				Name: "reviewer",
				Inputs: []interfaces.IOConfig{
					{StateName: "review", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{
						Type:        interfaces.GuardTypeVisitCount,
						Workstation: "coding",
						MaxVisits:   3,
					},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Verify the reviewer transition has a VisitCountGuard on its first input arc.
	reviewer := outputNet.Transitions["reviewer"]
	if reviewer == nil {
		t.Fatal("expected transition 'reviewer' to exist")
	}
	if len(reviewer.InputArcs) != 1 {
		t.Fatalf("expected 1 input arc on reviewer, got %d", len(reviewer.InputArcs))
	}

	guard, ok := reviewer.InputArcs[0].Guard.(*petri.VisitCountGuard)
	if !ok {
		t.Fatalf("expected VisitCountGuard on reviewer input arc, got %T", reviewer.InputArcs[0].Guard)
	}
	if guard.TransitionID != "coding" {
		t.Errorf("guard transition ID: expected coding, got %s", guard.TransitionID)
	}
	if guard.MaxVisits != 3 {
		t.Errorf("guard max visits: expected 3, got %d", guard.MaxVisits)
	}
}

func TestConfigMapping_GuardedLogicalMoveLoopBreakerRemainsNormalTransition(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "process",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "failed", WorkTypeName: "task"},
				},
			},
			{
				Name: "process-loop-breaker",
				Type: interfaces.WorkstationTypeLogical,
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "failed", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{
						Type:        interfaces.GuardTypeVisitCount,
						Workstation: "process",
						MaxVisits:   3,
					},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	if len(outputNet.Transitions) != 2 {
		t.Fatalf("expected only authored process transitions, got %d", len(outputNet.Transitions))
	}
	assertNoTransitionExhaustion(t, outputNet.Transitions)

	loopBreaker := outputNet.Transitions["process-loop-breaker"]
	if loopBreaker == nil {
		t.Fatal("expected guarded logical move loop breaker transition to exist")
	}
	if loopBreaker.WorkerType != "" {
		t.Fatalf("guarded logical move worker type = %q, want empty", loopBreaker.WorkerType)
	}
	assertGuardedLoopBreakerTransition(t, loopBreaker, "task:init", "task:failed", "process", 3)
}

func TestConfigMapping_ValidationRejectsWorkstationLevelChildFanInGuards(t *testing.T) {
	tests := []struct {
		name      string
		guardType interfaces.GuardType
	}{
		{name: "all children complete", guardType: interfaces.GuardTypeAllChildrenComplete},
		{name: "any child failed", guardType: interfaces.GuardTypeAnyChildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &interfaces.FactoryConfig{
				WorkTypes: []interfaces.WorkTypeConfig{
					{
						Name: "task",
						States: []interfaces.StateConfig{
							{Name: "init", Type: interfaces.StateTypeInitial},
							{Name: "complete", Type: interfaces.StateTypeTerminal},
							{Name: "failed", Type: interfaces.StateTypeFailed},
						},
					},
				},
				Workstations: []interfaces.FactoryWorkstationConfig{
					{
						Name: "collector",
						Inputs: []interfaces.IOConfig{
							{StateName: "init", WorkTypeName: "task"},
						},
						Outputs: []interfaces.IOConfig{
							{StateName: "complete", WorkTypeName: "task"},
						},
						Guards: []interfaces.GuardConfig{
							{Type: tt.guardType},
						},
					},
				},
			}

			mapper := ConfigMapper{}
			_, err := mapper.Map(context.Background(), input)
			if err == nil {
				t.Fatalf("expected validation error for workstation-level %s guard", tt.guardType)
			}
			if !strings.Contains(err.Error(), "use per-input guards for child fan-in") {
				t.Fatalf("expected per-input guard guidance, got %v", err)
			}
		})
	}
}

func TestConfigMapping_ValidationRejectsUnknownGuardType(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{Type: "nonexistent_guard"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unknown guard type")
	}
}

func TestConfigMapping_ValidationRejectsMatchesFieldsMissingInputKey(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards:         []interfaces.GuardConfig{{Type: interfaces.GuardTypeMatchesFields}},
		}},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard missing matchConfig.inputKey")
	}
}

func TestConfigMapping_ValidationRejectsMatchesFieldsEmptyInputKey(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "processor",
			WorkerTypeName: "matcher",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: " "},
			}},
		}},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for matches_fields guard empty matchConfig.inputKey")
	}
}

func TestConfigMapping_MatchesFieldsGuardBuildsSelectorGuardsAcrossInputs(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "match-items",
			WorkerTypeName: "matcher",
			Inputs: []interfaces.IOConfig{
				{StateName: "ready", WorkTypeName: "plan"},
				{StateName: "ready", WorkTypeName: "task"},
			},
			Outputs: []interfaces.IOConfig{{StateName: "matched", WorkTypeName: "task"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: `.Tags["_last_output"]`},
			}},
		}},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	transition := outputNet.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected transition 'match-items' to exist")
	}
	if len(transition.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(transition.InputArcs))
	}

	firstGuard, ok := transition.InputArcs[0].Guard.(*petri.MatchesFieldsGuard)
	if !ok {
		t.Fatalf("expected first arc guard to be MatchesFieldsGuard, got %T", transition.InputArcs[0].Guard)
	}
	if firstGuard.InputKey != `.Tags["_last_output"]` || firstGuard.MatchBinding != "" {
		t.Fatalf("unexpected first matches-fields guard: %#v", firstGuard)
	}

	secondGuard, ok := transition.InputArcs[1].Guard.(*petri.MatchesFieldsGuard)
	if !ok {
		t.Fatalf("expected second arc guard to be MatchesFieldsGuard, got %T", transition.InputArcs[1].Guard)
	}
	if secondGuard.InputKey != `.Tags["_last_output"]` {
		t.Fatalf("unexpected second guard selector: %#v", secondGuard)
	}
	if secondGuard.MatchBinding != transition.InputArcs[0].Name {
		t.Fatalf("expected second guard to bind to first input arc %q, got %q", transition.InputArcs[0].Name, secondGuard.MatchBinding)
	}
}

func TestConfigMapping_MatchesFieldsGuardBuildsSelectorGuardsAcrossAllInputsByDefault(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "asset",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "match-triplet",
			WorkerTypeName: "matcher",
			Inputs: []interfaces.IOConfig{
				{StateName: "ready", WorkTypeName: "plan"},
				{StateName: "ready", WorkTypeName: "task"},
				{StateName: "ready", WorkTypeName: "asset"},
			},
			Outputs: []interfaces.IOConfig{{StateName: "matched", WorkTypeName: "asset"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: ".Name"},
			}},
		}},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	transition := outputNet.Transitions["match-triplet"]
	if transition == nil {
		t.Fatal("expected transition 'match-triplet' to exist")
	}
	if len(transition.InputArcs) != 3 {
		t.Fatalf("expected 3 input arcs, got %d", len(transition.InputArcs))
	}

	sourceBinding := transition.InputArcs[0].Name
	for i := range transition.InputArcs {
		guard, ok := transition.InputArcs[i].Guard.(*petri.MatchesFieldsGuard)
		if !ok {
			t.Fatalf("expected input arc %d guard to be MatchesFieldsGuard, got %T", i, transition.InputArcs[i].Guard)
		}
		if guard.InputKey != ".Name" {
			t.Fatalf("unexpected selector on input arc %d: %#v", i, guard)
		}
		if i == 0 {
			if guard.MatchBinding != "" {
				t.Fatalf("expected source arc to have empty match binding, got %#v", guard)
			}
			continue
		}
		if guard.MatchBinding != sourceBinding {
			t.Fatalf("expected input arc %d to bind against %q, got %q", i, sourceBinding, guard.MatchBinding)
		}
	}
}

func TestConfigMapping_ValidationRejectsVisitCountGuardMissingParams(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{Type: interfaces.GuardTypeVisitCount},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for visit_count guard missing workstation")
	}
}

func TestConfigMapping_ValidationRejectsGuardReferencingNonexistentWorkstation(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Guards: []interfaces.GuardConfig{
					{
						Type:        interfaces.GuardTypeVisitCount,
						Workstation: "nonexistent",
						MaxVisits:   3,
					},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for guard referencing non-existent workstation")
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-resource-fixture review=2026-07-18 removal=split-resource-config-fixture-before-next-resource-change
func TestConfigMapping_ResourceUsage(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "gpu-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "gpu-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Verify resource place was created with correct ID and state.
	resourcePlace := outputNet.Places["gpu:available"]
	if resourcePlace == nil {
		t.Fatal("expected resource place 'gpu:available' to exist")
	}
	if resourcePlace.TypeID != "gpu" {
		t.Errorf("resource place type: expected 'gpu', got %q", resourcePlace.TypeID)
	}
	if resourcePlace.State != "available" {
		t.Errorf("resource place state: expected 'available', got %q", resourcePlace.State)
	}

	// Verify transition has consume and release arcs.
	tr := outputNet.Transitions["processor"]
	if tr == nil {
		t.Fatal("expected transition 'processor' to exist")
	}

	// Should have 2 input arcs: normal input + resource consume.
	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(tr.InputArcs))
	}
	consumeArc := tr.InputArcs[1]
	if consumeArc.PlaceID != "gpu:available" {
		t.Errorf("consume arc place: expected 'gpu:available', got %q", consumeArc.PlaceID)
	}
	if consumeArc.Name != "gpu:consume:processor" {
		t.Errorf("consume arc name: expected 'gpu:consume:processor', got %q", consumeArc.Name)
	}
	if consumeArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("consume arc mode: expected CONSUME, got %d", consumeArc.Mode)
	}
	if consumeArc.Cardinality.Mode != petri.CardinalityN || consumeArc.Cardinality.Count != 1 {
		t.Errorf("consume arc cardinality: expected N(1), got %d(%d)", consumeArc.Cardinality.Mode, consumeArc.Cardinality.Count)
	}

	// Should have 2 output arcs: normal output + resource release.
	if len(tr.OutputArcs) != 2 {
		t.Fatalf("expected 2 output arcs, got %d", len(tr.OutputArcs))
	}
	releaseArc := tr.OutputArcs[1]
	if releaseArc.PlaceID != "gpu:available" {
		t.Errorf("release arc place: expected 'gpu:available', got %q", releaseArc.PlaceID)
	}
	if releaseArc.Name != "gpu:release:processor" {
		t.Errorf("release arc name: expected 'gpu:release:processor', got %q", releaseArc.Name)
	}
	if releaseArc.Cardinality.Mode != petri.CardinalityN || releaseArc.Cardinality.Count != 1 {
		t.Errorf("release arc cardinality: expected N(1), got %d(%d)", releaseArc.Cardinality.Mode, releaseArc.Cardinality.Count)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-resource-fixture review=2026-07-18 removal=split-shared-resource-fixture-before-next-resource-change
func TestConfigMapping_TwoWorkstationsSharingResource(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 1},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "worker-a"},
			{Name: "worker-b"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "step1",
				WorkerTypeName: "worker-a",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 1},
				},
			},
			{
				Name:           "step2",
				WorkerTypeName: "worker-b",
				Inputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Both transitions should have resource consume and release arcs.
	for _, name := range []string{"step1", "step2"} {
		tr := outputNet.Transitions[name]
		if tr == nil {
			t.Fatalf("expected transition %q to exist", name)
		}
		// 1 normal input + 1 consume = 2 input arcs.
		if len(tr.InputArcs) != 2 {
			t.Errorf("transition %q: expected 2 input arcs, got %d", name, len(tr.InputArcs))
		}
		// 1 normal output + 1 release = 2 output arcs.
		if len(tr.OutputArcs) != 2 {
			t.Errorf("transition %q: expected 2 output arcs, got %d", name, len(tr.OutputArcs))
		}

		// Verify consume arc references the shared resource.
		consumeArc := tr.InputArcs[1]
		if consumeArc.PlaceID != "gpu:available" {
			t.Errorf("transition %q: consume arc place expected 'gpu:available', got %q", name, consumeArc.PlaceID)
		}

		// Verify release arc references the shared resource.
		releaseArc := tr.OutputArcs[1]
		if releaseArc.PlaceID != "gpu:available" {
			t.Errorf("transition %q: release arc place expected 'gpu:available', got %q", name, releaseArc.PlaceID)
		}
	}
}

func TestConfigMapping_ValidationRejectsNonexistentResource(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "nonexistent-gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage referencing non-existent resource")
	}
}

func TestConfigMapping_ValidationRejectsInvalidResourceCount(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 0},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage with zero count")
	}
}

// --- per-input guard tests ---

// portos:func-length-exception owner=agent-factory reason=legacy-input-guard-fixture review=2026-07-18 removal=split-static-guard-fixture-before-next-input-guard-change
func TestConfigMapping_PerInputGuard_StaticAllChildrenComplete(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "request",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "waiting", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "collect-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "collector",
				WorkerTypeName: "collect-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "waiting", WorkTypeName: "request"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "request",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "request"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	collector := outputNet.Transitions["collector"]
	if collector == nil {
		t.Fatal("expected transition 'collector' to exist")
	}

	// Should have 2 input arcs: parent consume + child observe.
	if len(collector.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs on collector, got %d", len(collector.InputArcs))
	}

	// First arc: parent consume with named binding "parent".
	parentArc := collector.InputArcs[0]
	if parentArc.Name != "parent" {
		t.Errorf("first input arc name: expected 'parent', got %q", parentArc.Name)
	}
	if parentArc.PlaceID != "request:waiting" {
		t.Errorf("parent arc place: expected 'request:waiting', got %q", parentArc.PlaceID)
	}

	// Second arc: child observation with AllWithParentGuard.
	childArc := collector.InputArcs[1]
	if childArc.PlaceID != "page:complete" {
		t.Errorf("child arc place: expected 'page:complete', got %q", childArc.PlaceID)
	}
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("child arc mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityAll {
		t.Errorf("child arc cardinality: expected ALL, got %d", childArc.Cardinality.Mode)
	}

	guard, ok := childArc.Guard.(*petri.AllWithParentGuard)
	if !ok {
		t.Fatalf("expected AllWithParentGuard on child arc, got %T", childArc.Guard)
	}
	if guard.MatchBinding != "parent" {
		t.Errorf("guard match binding: expected 'parent', got %q", guard.MatchBinding)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-input-guard-fixture review=2026-07-18 removal=split-dynamic-guard-fixture-before-next-input-guard-change
func TestConfigMapping_PerInputGuard_DynamicFanout(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "chapter",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "parse-worker"},
			{Name: "complete-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "parser",
				WorkerTypeName: "parse-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "chapter"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "chapter"},
				},
			},
			{
				Name:           "chapter-complete",
				WorkerTypeName: "complete-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "chapter"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "chapter",
							SpawnedBy:   "parser",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "chapter"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	tr := outputNet.Transitions["chapter-complete"]
	if tr == nil {
		t.Fatal("expected transition 'chapter-complete' to exist")
	}

	// Should have 3 input arcs: parent consume + count consume + child observe.
	if len(tr.InputArcs) != 3 {
		t.Fatalf("expected 3 input arcs, got %d", len(tr.InputArcs))
	}

	// First arc: parent (chapter:processing).
	if tr.InputArcs[0].Name != "parent" {
		t.Errorf("arc[0] name: expected 'parent', got %q", tr.InputArcs[0].Name)
	}
	if tr.InputArcs[0].PlaceID != "chapter:processing" {
		t.Errorf("arc[0] place: expected 'chapter:processing', got %q", tr.InputArcs[0].PlaceID)
	}

	// Second arc: fanout count consume (replaced the original page:complete arc).
	countArc := tr.InputArcs[1]
	if countArc.Name != "fanout-count" {
		t.Errorf("arc[1] name: expected 'fanout-count', got %q", countArc.Name)
	}
	if countArc.PlaceID != "parser:fanout-count" {
		t.Errorf("arc[1] place: expected 'parser:fanout-count', got %q", countArc.PlaceID)
	}
	if countArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("arc[1] mode: expected CONSUME, got %d", countArc.Mode)
	}
	_, isMatch := countArc.Guard.(*petri.MatchColorGuard)
	if !isMatch {
		t.Fatalf("arc[1] guard: expected MatchColorGuard, got %T", countArc.Guard)
	}

	// Third arc: child observation with FanoutCountGuard.
	childArc := tr.InputArcs[2]
	if childArc.PlaceID != "page:complete" {
		t.Errorf("arc[2] place: expected 'page:complete', got %q", childArc.PlaceID)
	}
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("arc[2] mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityZeroOrMore {
		t.Errorf("arc[2] cardinality: expected ZERO_OR_MORE, got %d", childArc.Cardinality.Mode)
	}
	fcGuard, ok := childArc.Guard.(*petri.FanoutCountGuard)
	if !ok {
		t.Fatalf("arc[2] guard: expected FanoutCountGuard, got %T", childArc.Guard)
	}
	if fcGuard.MatchBinding != "parent" {
		t.Errorf("fanout guard match binding: expected 'parent', got %q", fcGuard.MatchBinding)
	}
	if fcGuard.CountBinding != "fanout-count" {
		t.Errorf("fanout guard count binding: expected 'fanout-count', got %q", fcGuard.CountBinding)
	}

	// Verify fanout group was created.
	if outputNet.FanoutGroups == nil {
		t.Fatal("expected FanoutGroups to be set")
	}
	if outputNet.FanoutGroups["parser"] != "parser:fanout-count" {
		t.Errorf("FanoutGroups[parser]: expected 'parser:fanout-count', got %q", outputNet.FanoutGroups["parser"])
	}
}

func TestConfigMapping_PerInputGuard_AnyChildFailed(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "request",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "waiting", Type: interfaces.StateTypeProcessing},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "check-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "failure-checker",
				WorkerTypeName: "check-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "waiting", WorkTypeName: "request"},
					{
						StateName:    "failed",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAnyChildFailed,
							ParentInput: "request",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "failed", WorkTypeName: "request"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	tr := outputNet.Transitions["failure-checker"]
	if tr == nil {
		t.Fatal("expected transition 'failure-checker' to exist")
	}

	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(tr.InputArcs))
	}

	childArc := tr.InputArcs[1]
	if childArc.Mode != interfaces.ArcModeObserve {
		t.Errorf("child arc mode: expected OBSERVE, got %d", childArc.Mode)
	}
	if childArc.Cardinality.Mode != petri.CardinalityOne {
		t.Errorf("child arc cardinality: expected ONE, got %d", childArc.Cardinality.Mode)
	}

	guard, ok := childArc.Guard.(*petri.AnyWithParentGuard)
	if !ok {
		t.Fatalf("expected AnyWithParentGuard, got %T", childArc.Guard)
	}
	if guard.MatchBinding != "parent" {
		t.Errorf("guard match binding: expected 'parent', got %q", guard.MatchBinding)
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsMissingParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeAllChildrenComplete,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard missing parent_input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "bad-guard",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "init",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "page", // Self-reference.
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "page"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing its own input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsInvalidParentInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "nonexistent",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent parent input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsInvalidSpawnedBy(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "task",
							SpawnedBy:   "nonexistent-workstation",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for per-input guard referencing non-existent spawned_by workstation")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsUnsupportedType(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "collector",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "page",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeVisitCount,
							ParentInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unsupported per-input guard type")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameMissingMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type: interfaces.GuardTypeSameName,
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard missing match_input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameSelfReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "ready",
						WorkTypeName: "plan",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
					},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing its own input")
	}
}

func TestConfigMapping_PerInputGuard_ValidationRejectsSameNameUnknownMatchInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "other",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for same-name guard referencing non-existent input")
	}
}

func TestConfigMapping_PerInputGuard_SameNameBuildsConsumeGuardAgainstPeerInput(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "plan",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "match-items",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "plan"},
					{
						StateName:    "ready",
						WorkTypeName: "task",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "plan",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "matched", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected match-items transition")
	}

	var planArc *petri.Arc
	var taskArc *petri.Arc
	for i := range transition.InputArcs {
		arc := &transition.InputArcs[i]
		switch arc.PlaceID {
		case "plan:ready":
			planArc = arc
		case "task:ready":
			taskArc = arc
		}
	}

	if planArc == nil || taskArc == nil {
		t.Fatalf("expected plan/task input arcs, got %#v", transition.InputArcs)
	}
	if taskArc.Mode != interfaces.ArcModeConsume {
		t.Fatalf("same-name guarded arc mode = %v, want consume", taskArc.Mode)
	}
	if taskArc.Cardinality.Mode != petri.CardinalityOne {
		t.Fatalf("same-name guarded arc cardinality = %v, want one", taskArc.Cardinality.Mode)
	}
	guard, ok := taskArc.Guard.(*petri.SameNameGuard)
	if !ok {
		t.Fatalf("same-name guarded arc guard = %T, want *petri.SameNameGuard", taskArc.Guard)
	}
	if guard.MatchBinding != planArc.Name {
		t.Fatalf("same-name guard binding = %q, want %q", guard.MatchBinding, planArc.Name)
	}
}

func TestConfigMapping_WorkstationTypeDefaultsToStandard(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				// Type not set — should default to "standard"
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if tr == nil {
		t.Fatal("expected mapped transition for processor")
	}
	if len(tr.RejectionArcs) != 0 {
		t.Fatalf("default standard workstation should not add rejection arcs, got %+v", tr.RejectionArcs)
	}
}

func TestConfigMapping_WorkstationTypeExplicitStandard(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: interfaces.WorkstationKindStandard,
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if got := workstationconfig.Kind(tr, factoryConfigWorkstationLookup(input)); got != interfaces.WorkstationKindStandard {
		t.Errorf("expected derived workstation kind %q, got %q", interfaces.WorkstationKindStandard, got)
	}
}

func TestConfigMapping_WorkstationTypeRepeater(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: interfaces.WorkstationKindRepeater,
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["processor"]
	if got := workstationconfig.Kind(tr, factoryConfigWorkstationLookup(input)); got != interfaces.WorkstationKindRepeater {
		t.Errorf("expected derived workstation kind %q, got %q", interfaces.WorkstationKindRepeater, got)
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Errorf("expected auto rejection arc to task:init, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Errorf("expected auto failure arc to task:failed, got %+v", tr.FailureArcs)
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-mapping-fixture review=2026-07-18 removal=split-cron-fixture-before-next-cron-topology-change
func TestConfigMapping_WorkstationTypeCron(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{{Name: "cron-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "daily-refresh",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "cron-worker",
				Cron:           &interfaces.CronConfig{Schedule: "*/30 * * * *"},
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["daily-refresh"]
	if tr == nil {
		t.Fatal("expected cron transition")
	}
	if got := workstationconfig.Kind(tr, factoryConfigWorkstationLookup(input)); got != interfaces.WorkstationKindCron {
		t.Errorf("expected derived workstation kind %q, got %q", interfaces.WorkstationKindCron, got)
	}
	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected required cron input plus time input, got %+v", tr.InputArcs)
	}
	if tr.InputArcs[0].PlaceID != "task:ready" {
		t.Fatalf("expected required cron input to be preserved, got %+v", tr.InputArcs)
	}
	timeArc := tr.InputArcs[1]
	if timeArc.PlaceID != interfaces.SystemTimePendingPlaceID {
		t.Fatalf("expected cron time input from %q, got %+v", interfaces.SystemTimePendingPlaceID, tr.InputArcs)
	}
	if _, ok := timeArc.Guard.(*petri.CronTimeWindowGuard); !ok {
		t.Fatalf("expected cron time guard, got %T", timeArc.Guard)
	}
	if timeArc.Mode != interfaces.ArcModeConsume {
		t.Fatalf("expected cron time arc to consume, got %v", timeArc.Mode)
	}
	if net.Places[interfaces.SystemTimePendingPlaceID] == nil {
		t.Fatalf("expected system time pending place to be materialized")
	}
	if net.WorkTypes[interfaces.SystemTimeWorkTypeID] == nil {
		t.Fatalf("expected system time work type to be materialized")
	}
	expiry := net.Transitions[interfaces.SystemTimeExpiryTransitionID]
	if expiry == nil {
		t.Fatalf("expected system time expiry transition")
	}
	if expiry.Type != petri.TransitionExhaustion {
		t.Fatalf("expected expiry transition type %s, got %s", petri.TransitionExhaustion, expiry.Type)
	}
	if expiry.WorkerType != "" {
		t.Fatalf("expected expiry transition not to invoke a worker, got %q", expiry.WorkerType)
	}
	if len(expiry.OutputArcs) != 0 {
		t.Fatalf("expected expiry transition to consume without output arcs, got %+v", expiry.OutputArcs)
	}
	if len(expiry.InputArcs) != 1 {
		t.Fatalf("expected one expiry input arc, got %+v", expiry.InputArcs)
	}
	expiryArc := expiry.InputArcs[0]
	if expiryArc.PlaceID != interfaces.SystemTimePendingPlaceID {
		t.Fatalf("expected expiry to consume from %q, got %+v", interfaces.SystemTimePendingPlaceID, expiryArc)
	}
	if _, ok := expiryArc.Guard.(*petri.ExpiredTimeWorkGuard); !ok {
		t.Fatalf("expected expiry guard, got %T", expiryArc.Guard)
	}
	if expiryArc.Mode != interfaces.ArcModeConsume || expiryArc.Cardinality.Mode != petri.CardinalityAll {
		t.Fatalf("expected expiry to consume all expired time tokens, got mode=%v cardinality=%v", expiryArc.Mode, expiryArc.Cardinality.Mode)
	}
	if len(tr.OutputArcs) != 1 || tr.OutputArcs[0].PlaceID != "task:init" {
		t.Fatalf("expected cron output to be preserved, got %+v", tr.OutputArcs)
	}
}

func TestConfigMapping_CronTimeArcDoesNotReceiveDependencyGuard(t *testing.T) {
	input := cronRequiredInputFactoryConfig()

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["daily-refresh"]
	if tr == nil {
		t.Fatal("expected cron transition")
	}
	var foundTimeArc bool
	for _, arc := range tr.InputArcs {
		if arc.PlaceID != interfaces.SystemTimePendingPlaceID {
			continue
		}
		foundTimeArc = true
		if _, ok := arc.Guard.(*petri.CronTimeWindowGuard); !ok {
			t.Fatalf("expected cron time guard to survive dependency injection, got %T", arc.Guard)
		}
	}
	if !foundTimeArc {
		t.Fatal("expected cron time input arc")
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-enableability-fixture review=2026-07-18 removal=split-cron-enableability-fixture-before-next-cron-topology-change
func TestConfigMapping_CronTimeEnablementUsesSharedTimePlace(t *testing.T) {
	now := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	input := cronRequiredInputFactoryConfig()

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		tokens   []*interfaces.Token
		want     bool
		wantBind []string
	}{
		{
			name: "ready input and due time token enables cron",
			tokens: []*interfaces.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want:     true,
			wantBind: []string{"task:ready:to:daily-refresh", interfaces.SystemTimePendingPlaceID + ":to:daily-refresh"},
		},
		{
			name: "missing configured input disables cron",
			tokens: []*interfaces.Token{
				configMapperCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want: false,
		},
		{
			name: "not-yet-due time token disables cron",
			tokens: []*interfaces.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-early", "daily-refresh", now.Add(time.Second), now.Add(time.Minute)),
			},
			want: false,
		},
		{
			name: "expired time token disables cron",
			tokens: []*interfaces.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-expired", "daily-refresh", now.Add(-time.Minute), now),
			},
			want: false,
		},
		{
			name: "wrong workstation time token disables cron",
			tokens: []*interfaces.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-wrong", "other-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marking := petri.NewMarking("workflow")
			for _, token := range tt.tokens {
				marking.AddToken(token)
			}
			snapshot := marking.Snapshot()
			evaluator := scheduler.NewEnablementEvaluator(nil, scheduler.WithEnablementClock(func() time.Time {
				return now
			}))

			enabled := evaluator.FindEnabledTransitions(context.Background(), net, &snapshot)
			got := false
			for _, candidate := range enabled {
				if candidate.TransitionID == "daily-refresh" {
					got = true
				}
			}
			if got != tt.want {
				t.Fatalf("enabled = %v, want %v; transitions=%+v", got, tt.want, enabled)
			}
			if !tt.want {
				return
			}
			for _, binding := range tt.wantBind {
				if len(enabled[0].Bindings[binding]) != 1 {
					t.Fatalf("expected binding %q to have one token, got %+v", binding, enabled[0].Bindings)
				}
			}
		})
	}
}

func TestConfigMapping_DefaultExpiryTargetsExpiredTokenCronCannotUse(t *testing.T) {
	now := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	input := cronRequiredInputFactoryConfig()

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	marking := petri.NewMarking("workflow")
	marking.AddToken(configMapperWorkToken("task-ready", "task", "ready"))
	marking.AddToken(configMapperCronTimeToken("time-expired", "daily-refresh", now.Add(-time.Minute), now))
	snapshot := marking.Snapshot()
	evaluator := scheduler.NewEnablementEvaluator(nil, scheduler.WithEnablementClock(func() time.Time {
		return now
	}))
	var expiryEnabled bool
	for _, enabled := range evaluator.FindEnabledTransitions(context.Background(), net, &snapshot) {
		if enabled.TransitionID == "daily-refresh" {
			t.Fatalf("cron transition should reject expired time token, got %+v", enabled)
		}
		if enabled.TransitionID == interfaces.SystemTimeExpiryTransitionID {
			expiryEnabled = true
			if got := enabled.Bindings[interfaces.SystemTimePendingPlaceID+":to:"+interfaces.SystemTimeExpiryTransitionID]; len(got) != 1 || got[0].ID != "time-expired" {
				t.Fatalf("expected expiry binding to select time-expired, got %+v", enabled.Bindings)
			}
		}
	}
	if !expiryEnabled {
		t.Fatalf("expected expiry transition to target the stale time token")
	}
}

func TestConfigMapping_ValidationRejectsSingleInputWithTwoSameTypeOutputs(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "splitter",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "review", WorkTypeName: "task"},
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected type-alignment validation error")
	}
	if !strings.Contains(err.Error(), "TYPE_COUNT_COLLISION") {
		t.Fatalf("expected TYPE_COUNT_COLLISION in error, got %v", err)
	}
}

func TestConfigMapping_ValidationRejectsMismatchedCountsAcrossMultiInputSameTypeRoutes(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready-a", Type: interfaces.StateTypeInitial},
					{Name: "ready-b", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "combiner",
				Inputs: []interfaces.IOConfig{
					{StateName: "ready-a", WorkTypeName: "task"},
					{StateName: "ready-b", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected type-alignment validation error")
	}
	if !strings.Contains(err.Error(), "TYPE_COUNT_COLLISION") {
		t.Fatalf("expected TYPE_COUNT_COLLISION in error, got %v", err)
	}
}

func TestConfigMapping_ValidationAllowsCrossTypeFanout(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "asset",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "fanout",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "page"},
					{StateName: "complete", WorkTypeName: "asset"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	if _, err := mapper.Map(context.Background(), input); err != nil {
		t.Fatalf("expected cross-type fanout to be allowed, got %v", err)
	}
}

func TestConfigMapping_ValidationRejectsUnknownWorkstationKind(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: "unknown_kind",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unknown workstation kind")
	}
}

func TestConfigMapping_ValidationRejectsNonexistentWorker(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "real-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "ghost-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for workstation referencing non-existent worker")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, `references non-existent worker "ghost-worker"`) {
		t.Errorf("unexpected error message:\ngot: %s\nwant it to mention: references non-existent worker \"ghost-worker\"", errMsg)
	}
}

func TestConfigMapping_ValidationAcceptsValidWorkerReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "executor"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "executor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func cronRequiredInputFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{{Name: "cron-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "daily-refresh",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "cron-worker",
				Cron:           &interfaces.CronConfig{Schedule: "*/30 * * * *"},
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
			},
		},
	}
}

func configMapperWorkToken(id string, workType string, state string) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: fmt.Sprintf("%s:%s", workType, state),
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: workType,
			DataType:   interfaces.DataTypeWork,
		},
	}
}

func configMapperCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.UTC().Format(time.RFC3339Nano),
			},
		},
	}
}

// --- assertion helpers ---

func assertEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	assertPlaceEquality(t, expectedNet, outputNet)
	assertTransitionEquality(t, expectedNet, outputNet)
}

func assertNoTransitionExhaustion(t *testing.T, transitions map[string]*petri.Transition) {
	t.Helper()

	for name, transition := range transitions {
		if transition.Type == petri.TransitionExhaustion {
			t.Fatalf("unexpected TransitionExhaustion transition %q in customer-authored mapping", name)
		}
	}
}

func assertGuardedLoopBreakerTransition(t *testing.T, transition *petri.Transition, inputPlace string, outputPlace string, watchedTransition string, maxVisits int) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected guarded loop-breaker transition to exist")
	}
	if transition.Type != petri.TransitionNormal {
		t.Fatalf("guarded loop-breaker type = %s, want %s", transition.Type, petri.TransitionNormal)
	}
	if len(transition.InputArcs) != 1 {
		t.Fatalf("guarded loop-breaker input arcs = %d, want 1", len(transition.InputArcs))
	}
	if transition.InputArcs[0].PlaceID != inputPlace {
		t.Fatalf("guarded loop-breaker input place = %q, want %q", transition.InputArcs[0].PlaceID, inputPlace)
	}
	guard, ok := transition.InputArcs[0].Guard.(*petri.VisitCountGuard)
	if !ok {
		t.Fatalf("expected VisitCountGuard on guarded loop breaker, got %T", transition.InputArcs[0].Guard)
	}
	if guard.TransitionID != watchedTransition {
		t.Fatalf("guarded loop-breaker guard transition = %q, want %s", guard.TransitionID, watchedTransition)
	}
	if guard.MaxVisits != maxVisits {
		t.Fatalf("guarded loop-breaker guard max visits = %d, want %d", guard.MaxVisits, maxVisits)
	}
	if len(transition.OutputArcs) != 1 {
		t.Fatalf("guarded loop-breaker output arcs = %d, want 1", len(transition.OutputArcs))
	}
	if transition.OutputArcs[0].PlaceID != outputPlace {
		t.Fatalf("guarded loop-breaker output place = %q, want %q", transition.OutputArcs[0].PlaceID, outputPlace)
	}
}

func assertPlaceEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	for placeName, place := range expectedNet.Places {
		outputTarget := outputNet.Places[placeName]
		if outputTarget == nil {
			t.Errorf("failed to find a corresponding place for input place %s", placeName)
			continue
		}
		assertEqualPlaces(t, place, outputTarget)
	}

	for placeName := range outputNet.Places {
		if expectedNet.Places[placeName] == nil {
			t.Errorf("declared place that was not expected %s", placeName)
		}
	}
}

func assertEqualPlaces(t *testing.T, place, outputTarget *petri.Place) {
	t.Helper()
	if place.ID != outputTarget.ID {
		t.Errorf("ids not matching, expected %s, output %s", place.ID, outputTarget.ID)
	}
	if place.State != outputTarget.State {
		t.Errorf("states not matching, expected %q, output %q", place.State, outputTarget.State)
	}
	if place.TypeID != outputTarget.TypeID {
		t.Errorf("types not matching, expected %q, output %q", place.TypeID, outputTarget.TypeID)
	}
}

func assertEqualTransitions(t *testing.T, expected, output *petri.Transition) {
	t.Helper()
	if expected.ID != output.ID {
		t.Errorf("ids not matching, expected %s, output %s", expected.ID, output.ID)
	}
	if expected.Name != output.Name {
		t.Errorf("names not matching, expected %s, output %s", expected.Name, output.Name)
	}
	if expected.WorkerType != output.WorkerType {
		t.Errorf("worker types not matching, expected %s, output %s", expected.WorkerType, output.WorkerType)
	}
	assertArcEquality(t, "input", expected.InputArcs, output.InputArcs)
	assertArcEquality(t, "output", expected.OutputArcs, output.OutputArcs)
	assertArcEquality(t, "rejection", expected.RejectionArcs, output.RejectionArcs)
	assertArcEquality(t, "failure", expected.FailureArcs, output.FailureArcs)
}

func assertArcEquality(t *testing.T, arcType string, expected, output []petri.Arc) {
	t.Helper()
	expecteds := make(map[string]*petri.Arc)
	outputs := make(map[string]*petri.Arc)
	for _, expectedArc := range expected {
		id := getArcID(expectedArc)
		expecteds[id] = &expectedArc
	}
	for _, outputArc := range output {
		id := getArcID(outputArc)
		outputs[id] = &outputArc
	}

	for id, arc := range expecteds {
		outputArc := outputs[id]
		if outputArc == nil {
			t.Errorf("failed to find a corresponding %s arc for %s", arcType, id)
			continue
		}
		assertEqualArcs(t, arc, outputArc)
	}
	for id := range outputs {
		if expecteds[id] == nil {
			t.Errorf("declared %s arc that was not expected %s", arcType, id)
		}
	}
}

func getArcID(arc petri.Arc) string {
	return fmt.Sprintf("%s-%s", arc.PlaceID, arc.TransitionID)
}

func assertEqualArcs(t *testing.T, expected, output *petri.Arc) {
	t.Helper()
	if expected.Name != output.Name {
		t.Errorf("names not matching, expected %s, output %s", expected.Name, output.Name)
	}
	if expected.PlaceID != output.PlaceID {
		t.Errorf("place ids not matching, expected %s, output %s", expected.PlaceID, output.PlaceID)
	}
	if expected.TransitionID != output.TransitionID {
		t.Errorf("transition ids not matching, expected %s, output %s", expected.TransitionID, output.TransitionID)
	}
}

func assertTransitionEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	for name, expected := range expectedNet.Transitions {
		output := outputNet.Transitions[name]
		if output == nil {
			t.Errorf("failed to find a corresponding transition for %s", name)
			continue
		}
		assertEqualTransitions(t, expected, output)
	}
	for name := range outputNet.Transitions {
		if expectedNet.Transitions[name] == nil {
			t.Errorf("declared transition that was not expected %s", name)
		}
	}
}
