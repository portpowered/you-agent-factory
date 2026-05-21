package config

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestConfigMapping_WorkstationTypeDefaultsToStandard(t *testing.T) {
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
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("default standard workstation should reject through failure routing, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("default standard workstation should fail through failed-state routing, got %+v", tr.FailureArcs)
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
					{Name: "failed", Type: interfaces.StateTypeFailed},
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
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("explicit standard workstation should reject through failure routing, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("explicit standard workstation should fail to task:failed, got %+v", tr.FailureArcs)
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
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Fatalf("expected auto rejection arc to task:init, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("expected auto failure arc to task:failed, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_UsesEffectiveRuntimeConfigWorkstationKindsForNormalization(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"name":     "retry-task",
			"behavior": "REPEATER",
			"worker":   "executor",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: MODEL_WORKER
modelProvider: openai
model: gpt-5.4
---
Execute work.
`)
	writeRuntimeWorkstationAgentsMD(t, factoryDir, "retry-task", `---
type: MODEL_WORKSTATION
worker: executor
---
Retry work.
`)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), loaded.FactoryConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["retry-task"]
	if tr == nil {
		t.Fatal("expected mapped transition for retry-task")
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:init" {
		t.Fatalf("effective runtime repeater should reject back to task:init, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("effective runtime repeater should still fail through task:failed, got %+v", tr.FailureArcs)
	}
}

func TestConfigMapping_AuthoredWorkstationTransitionsUseMatchingIDAndName(t *testing.T) {
	t.Parallel()

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
		Workers: []interfaces.WorkerConfig{
			{Name: "reviewer"},
			{Name: "publisher"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "review-task",
				WorkerTypeName: "reviewer",
				Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}},
			},
			{
				Name:           "publish-task",
				WorkerTypeName: "publisher",
				Inputs:         []interfaces.IOConfig{{StateName: "review", WorkTypeName: "task"}},
				Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
				OnFailure:      []interfaces.IOConfig{{StateName: "failed", WorkTypeName: "task"}},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, workstationName := range []string{"review-task", "publish-task"} {
		transition := net.Transitions[workstationName]
		if transition == nil {
			t.Fatalf("expected transition %q to exist", workstationName)
		}
		if transition.ID != workstationName {
			t.Fatalf("transition %q ID = %q, want %q", workstationName, transition.ID, workstationName)
		}
		if transition.Name != workstationName {
			t.Fatalf("transition %q Name = %q, want %q", workstationName, transition.Name, workstationName)
		}
		if transition.ID != transition.Name {
			t.Fatalf("transition %q invariant failed: ID %q != Name %q", workstationName, transition.ID, transition.Name)
		}
	}
}

func TestConfigMapping_DefaultNonRepeaterFanInRejectionUsesFailureDestinations(t *testing.T) {
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
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "fan-in",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
					{StateName: "ready", WorkTypeName: "page"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
					{StateName: "complete", WorkTypeName: "page"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["fan-in"]
	if tr == nil {
		t.Fatal("expected mapped transition for fan-in")
	}
	if len(tr.FailureArcs) != 2 {
		t.Fatalf("expected default failure routing to both failed states, got %+v", tr.FailureArcs)
	}
	assertTransitionArcPlaces(t, tr.FailureArcs, "task:failed", "page:failed")
	if len(tr.RejectionArcs) != 2 {
		t.Fatalf("expected default rejection routing to clone failure destinations, got %+v", tr.RejectionArcs)
	}
	assertTransitionArcPlaces(t, tr.RejectionArcs, "task:failed", "page:failed")
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
					{Name: "failed", Type: interfaces.StateTypeFailed},
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
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("expected cron rejection to follow failure routing, got %+v", tr.RejectionArcs)
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("expected cron failure to route to task:failed, got %+v", tr.FailureArcs)
	}
}

func assertTransitionArcPlaces(t *testing.T, arcs []petri.Arc, wantPlaces ...string) {
	t.Helper()

	places := make(map[string]struct{}, len(arcs))
	for _, arc := range arcs {
		places[arc.PlaceID] = struct{}{}
	}
	for _, want := range wantPlaces {
		if _, ok := places[want]; !ok {
			t.Fatalf("arc places = %+v, want %s destination", arcs, want)
		}
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
