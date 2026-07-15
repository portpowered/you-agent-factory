package maptests

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestConfigMapping_CronWithoutRequiredInputsUsesOutputWorkTypeForImplicitFailureRouting(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{
			Name: "cron-worker",
			Type: interfaces.WorkerTypeModel,
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "poll-for-work",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "cron-worker",
			Cron:           &interfaces.CronConfig{Schedule: "* * * * *", TriggerAtStart: true},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
		}},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["poll-for-work"]
	if tr == nil {
		t.Fatal("expected mapped transition for poll-for-work")
	}
	if len(tr.FailureArcs) != 1 || tr.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("cron failure arcs = %+v, want output-derived failed-state routing", tr.FailureArcs)
	}
	if len(tr.RejectionArcs) != 1 || tr.RejectionArcs[0].PlaceID != "task:failed" {
		t.Fatalf("cron rejection arcs = %+v, want cloned failed-state routing", tr.RejectionArcs)
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-mapping-fixture review=2026-07-18 removal=split-cron-fixture-before-next-cron-topology-change
func TestConfigMapping_LogicalMoveCronWorkstationWithoutWorker(t *testing.T) {
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
				Name: "scheduled-route",
				Type: interfaces.WorkstationTypeLogical,
				Kind: interfaces.WorkstationKindCron,
				Cron: &interfaces.CronConfig{Schedule: "0 * * * *"},
				Outputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := net.Transitions["scheduled-route"]
	if tr == nil {
		t.Fatal("expected cron logical move transition")
	}
	if tr.WorkerType != "" {
		t.Fatalf("logical move cron worker type = %q, want empty", tr.WorkerType)
	}
	if len(tr.InputArcs) != 1 {
		t.Fatalf("expected cron time input only, got %+v", tr.InputArcs)
	}
	timeArc := tr.InputArcs[0]
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
	if len(tr.OutputArcs) != 1 || tr.OutputArcs[0].PlaceID != "task:init" {
		t.Fatalf("expected cron output to be preserved, got %+v", tr.OutputArcs)
	}
}

func TestConfigMapping_WorkstationTypeCron(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "ready", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []workerconfig.Config{{Name: "cron-worker"}},
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
				OnContinue: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertMappedCronTransition(t, net)
	assertMappedSystemTimeExpiryTransition(t, net)
}

func assertMappedCronTransition(t *testing.T, net *state.Net) {
	t.Helper()

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

func assertMappedSystemTimeExpiryTransition(t *testing.T, net *state.Net) {
	t.Helper()

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
}

func TestConfigMapping_CronTimeArcDoesNotReceiveDependencyGuard(t *testing.T) {
	input := cronRequiredInputFactoryConfig()

	mapper := testConfigMapper{}
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

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		tokens   []*factorytoken.Token
		want     bool
		wantBind []string
	}{
		{
			name: "ready input and due time token enables cron",
			tokens: []*factorytoken.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want:     true,
			wantBind: []string{"task:ready:to:daily-refresh", interfaces.SystemTimePendingPlaceID + ":to:daily-refresh"},
		},
		{
			name: "missing configured input disables cron",
			tokens: []*factorytoken.Token{
				configMapperCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want: false,
		},
		{
			name: "not-yet-due time token disables cron",
			tokens: []*factorytoken.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-early", "daily-refresh", now.Add(time.Second), now.Add(time.Minute)),
			},
			want: false,
		},
		{
			name: "expired time token disables cron",
			tokens: []*factorytoken.Token{
				configMapperWorkToken("task-ready", "task", "ready"),
				configMapperCronTimeToken("time-expired", "daily-refresh", now.Add(-time.Minute), now),
			},
			want: false,
		},
		{
			name: "wrong workstation time token disables cron",
			tokens: []*factorytoken.Token{
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

	mapper := testConfigMapper{}
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
