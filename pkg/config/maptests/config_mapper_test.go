package maptests

import (
	"context"
	. "github.com/portpowered/infinite-you/pkg/config"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "nonexistent"}},
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
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "nonexistent-type", StateName: "failed"}},
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

func TestConfigMapping_ValidationRejectsFactoryInferenceThrottleGuardMissingModelProvider(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			RefreshWindow: "15m",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []interfaces.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []interfaces.IOConfig{
				{StateName: "complete", WorkTypeName: "task"},
			},
		}},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for factory inference throttle guard missing modelProvider")
	}
	if !strings.Contains(err.Error(), "guards[0](inference_throttle_guard).modelProvider") {
		t.Fatalf("expected modelProvider field path, got %v", err)
	}
}

func TestConfigMapping_ValidationRejectsFactoryInferenceThrottleGuardInvalidRefreshWindow(t *testing.T) {
	input := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			RefreshWindow: "later",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []interfaces.IOConfig{
				{StateName: "init", WorkTypeName: "task"},
			},
			Outputs: []interfaces.IOConfig{
				{StateName: "complete", WorkTypeName: "task"},
			},
		}},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for factory inference throttle guard invalid refreshWindow")
	}
	if !strings.Contains(err.Error(), "guards[0](inference_throttle_guard).refreshWindow") {
		t.Fatalf("expected refreshWindow field path, got %v", err)
	}
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
		Workers: []interfaces.WorkerConfig{
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

	mapper := ConfigMapper{}
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
		Workers: []interfaces.WorkerConfig{
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

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected map error: %v", err)
	}

	marking := petri.NewMarking("workflow")
	marking.AddToken(&interfaces.Token{ID: "tok-claude", PlaceID: "task:init"})
	marking.AddToken(&interfaces.Token{ID: "tok-codex", PlaceID: "task:init"})
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID:   "d-throttle",
				TransitionID: "claude-step",
				ProviderFailure: &interfaces.ProviderFailureMetadata{
					Family: interfaces.ProviderErrorFamilyThrottle,
					Type:   interfaces.ProviderErrorTypeThrottled,
				},
				EndTime: time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	evaluator := scheduler.NewEnablementEvaluator(nil, scheduler.WithEnablementClock(func() time.Time {
		return time.Date(2026, time.May, 1, 10, 5, 0, 0, time.UTC)
	}), scheduler.WithEnablementRuntimeConfig(runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"claude-worker": {ModelProvider: "claude", Model: "claude-sonnet"},
			"codex-worker":  {ModelProvider: "codex", Model: "gpt-5-codex"},
		},
	}))

	enabled := evaluator.FindEnabledTransitionsWithSnapshot(context.Background(), net, &snapshot)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %+v, want only codex-step", enabled)
	}
	if enabled[0].TransitionID != "codex-step" {
		t.Fatalf("enabled transition = %s, want codex-step", enabled[0].TransitionID)
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
