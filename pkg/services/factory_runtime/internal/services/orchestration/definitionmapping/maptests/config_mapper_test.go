package maptests

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestConfigMapping_SimplePath(t *testing.T) {
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
				Name: "transformer",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				OnFailure: []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
			},
		},
	}

	expectedNet := &factoryruntime.Net{
		Places: map[string]*factoryruntime.PetriPlace{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete": {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		Transitions: map[string]*factoryruntime.PetriTransition{
			"transformer": {ID: "transformer", Name: "transformer",
				InputArcs: []factoryruntime.PetriArc{
					{Name: "task:init:to:transformer", PlaceID: "task:init", TransitionID: "transformer"},
				},
				OutputArcs: []factoryruntime.PetriArc{
					{Name: "task:complete:from:transformer", PlaceID: "task:complete", TransitionID: "transformer"},
				},
				FailureArcs: []factoryruntime.PetriArc{
					{Name: "task:failed:failure:transformer", PlaceID: "task:failed", TransitionID: "transformer"},
				},
				RejectionArcs: []factoryruntime.PetriArc{
					{Name: "transformer:auto-rejection:task:failed", PlaceID: "task:failed", TransitionID: "transformer"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			},
		},
	}

	mapper := testConfigMapper{}
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
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), rejectionLoopWithGuardedLoopBreakerFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	if len(outputNet.Transitions) != 2 {
		t.Fatalf("expected only authored reviewer transitions, got %d", len(outputNet.Transitions))
	}
	testutil.AssertNoTransitionExhaustion(t, outputNet.Transitions, testutil.PetriTransitionAssertOptions{
		ExhaustionContext: "customer-authored mapping",
	})
	assertReviewerRejectionTransition(t, outputNet.Transitions["reviewer"])
	testutil.AssertGuardedLoopBreakerTransition(t, outputNet.Transitions["reviewer-loop-breaker"], "task:init", "task:failed", "reviewer", 3)
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
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

func assertReviewerRejectionTransition(t *testing.T, transition *factoryruntime.PetriTransition) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected transition 'reviewer' to exist")
	}
	if len(transition.RejectionArcs) != 1 {
		t.Fatalf("expected 1 rejection arc on reviewer, got %d", len(transition.RejectionArcs))
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
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

	mapper := testConfigMapper{}
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

	guard, ok := reviewer.InputArcs[0].Guard.(*factoryruntime.PetriVisitCountGuard)
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

	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	if len(outputNet.Transitions) != 2 {
		t.Fatalf("expected only authored process transitions, got %d", len(outputNet.Transitions))
	}
	testutil.AssertNoTransitionExhaustion(t, outputNet.Transitions, testutil.PetriTransitionAssertOptions{
		ExhaustionContext: "customer-authored mapping",
	})

	loopBreaker := outputNet.Transitions["process-loop-breaker"]
	if loopBreaker == nil {
		t.Fatal("expected guarded logical move loop breaker transition to exist")
	}
	if loopBreaker.WorkerType != "" {
		t.Fatalf("guarded logical move worker type = %q, want empty", loopBreaker.WorkerType)
	}
	testutil.AssertGuardedLoopBreakerTransition(t, loopBreaker, "task:init", "task:failed", "process", 3)
}

func TestConfigMapping_LowersFactoryInferenceThrottleGuardOnlyAcrossMatchingWorkerTransitions(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			Model:         "claude-sonnet",
			RefreshWindow: "15m",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "claude-worker", ModelProvider: "claude", Model: "claude-sonnet"},
			{Name: "codex-worker", ModelProvider: "codex", Model: "gpt-5-codex"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "claude-step",
				WorkerTypeName: "claude-worker",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			},
			{
				Name:           "codex-step",
				WorkerTypeName: "codex-worker",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			},
			{
				Name:    "logical-step",
				Type:    interfaces.WorkstationTypeLogical,
				Inputs:  []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs: []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected map error: %v", err)
	}

	claudeGuard := net.Transitions["claude-step"].InputArcs[0].Guard
	codexGuard := net.Transitions["codex-step"].InputArcs[0].Guard
	logicalGuard := net.Transitions["logical-step"].InputArcs[0].Guard
	if !containsInferenceThrottleGuard(claudeGuard) {
		t.Fatalf("claude-step guard = %#v, want inference throttle guard in chain", claudeGuard)
	}
	if containsInferenceThrottleGuard(codexGuard) {
		t.Fatalf("codex-step guard = %#v, want no inference throttle guard outside authored lane", codexGuard)
	}
	if containsInferenceThrottleGuard(logicalGuard) {
		t.Fatalf("logical-step guard = %#v, want no inference throttle guard on non-worker transition", logicalGuard)
	}
}

func TestConfigMapping_FactoryInferenceThrottleGuardBlocksOnlyMatchingLaneAtRuntime(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			Model:         "claude-sonnet",
			RefreshWindow: "15m",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "claude-worker", ModelProvider: "claude", Model: "claude-sonnet"},
			{Name: "codex-worker", ModelProvider: "codex", Model: "gpt-5-codex"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "claude-step",
				WorkerTypeName: "claude-worker",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			},
			{
				Name:           "codex-step",
				WorkerTypeName: "codex-worker",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected map error: %v", err)
	}

	claude := net.Transitions["claude-step"]
	if claude == nil || len(claude.InputArcs) == 0 || !containsInferenceThrottleGuard(claude.InputArcs[0].Guard) {
		t.Fatalf("claude-step guard = %#v, want mapped inference-throttle guard", claude)
	}
	codex := net.Transitions["codex-step"]
	if codex == nil || len(codex.InputArcs) == 0 {
		t.Fatalf("codex-step = %#v, want mapped input arc", codex)
	}
	if containsInferenceThrottleGuard(codex.InputArcs[0].Guard) {
		t.Fatalf("codex-step guard = %#v, want no claude-lane throttle guard", codex.InputArcs[0].Guard)
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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "matcher"}},
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

	mapper := testConfigMapper{}
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

	firstGuard, ok := transition.InputArcs[0].Guard.(*factoryruntime.PetriMatchesFieldsGuard)
	if !ok {
		t.Fatalf("expected first arc guard to be MatchesFieldsGuard, got %T", transition.InputArcs[0].Guard)
	}
	if firstGuard.InputKey != `.Tags["_last_output"]` || firstGuard.MatchBinding != "" {
		t.Fatalf("unexpected first matches-fields guard: %#v", firstGuard)
	}

	secondGuard, ok := transition.InputArcs[1].Guard.(*factoryruntime.PetriMatchesFieldsGuard)
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
					{Name: "matched", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "asset",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "matched", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "matcher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "match-triplet",
			WorkerTypeName: "matcher",
			Inputs: []interfaces.IOConfig{
				{StateName: "ready", WorkTypeName: "plan"},
				{StateName: "ready", WorkTypeName: "task"},
				{StateName: "ready", WorkTypeName: "asset"},
			},
			Outputs:   []interfaces.IOConfig{{StateName: "matched", WorkTypeName: "asset"}},
			OnFailure: []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "asset"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: ".Name"},
			}},
		}},
	}

	mapper := testConfigMapper{}
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
		guard, ok := transition.InputArcs[i].Guard.(*factoryruntime.PetriMatchesFieldsGuard)
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
