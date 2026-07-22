package maptests

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
		Workers: []interfaces.FactoryWorkerConfig{{
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
	if _, ok := timeArc.Guard.(*factoryruntime.PetriCronTimeWindowGuard); !ok {
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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "cron-worker"}},
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

func assertMappedCronTransition(t *testing.T, net *factoryruntime.Net) {
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
	if _, ok := timeArc.Guard.(*factoryruntime.PetriCronTimeWindowGuard); !ok {
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

func assertMappedSystemTimeExpiryTransition(t *testing.T, net *factoryruntime.Net) {
	t.Helper()

	expiry := net.Transitions[interfaces.SystemTimeExpiryTransitionID]
	if expiry == nil {
		t.Fatalf("expected system time expiry transition")
	}
	if expiry.Type != factoryruntime.PetriTransitionExhaustion {
		t.Fatalf("expected expiry transition type %s, got %s", factoryruntime.PetriTransitionExhaustion, expiry.Type)
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
	if _, ok := expiryArc.Guard.(*factoryruntime.PetriExpiredTimeWorkGuard); !ok {
		t.Fatalf("expected expiry guard, got %T", expiryArc.Guard)
	}
	if expiryArc.Mode != interfaces.ArcModeConsume || expiryArc.Cardinality.Mode != factoryruntime.PetriCardinalityAll {
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
		if _, ok := arc.Guard.(*factoryruntime.PetriCronTimeWindowGuard); !ok {
			t.Fatalf("expected cron time guard to survive dependency injection, got %T", arc.Guard)
		}
	}
	if !foundTimeArc {
		t.Fatal("expected cron time input arc")
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-enableability-fixture review=2026-07-18 removal=split-cron-enableability-fixture-before-next-cron-topology-change
func TestConfigMapping_CronTimeEnablementUsesSharedTimePlace(t *testing.T) {
	input := cronRequiredInputFactoryConfig()

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertMappedCronTransition(t, net)
	if net.Places[interfaces.SystemTimePendingPlaceID] == nil {
		t.Fatalf("missing shared time place %q", interfaces.SystemTimePendingPlaceID)
	}
}

func TestConfigMapping_DefaultExpiryTargetsExpiredTokenCronCannotUse(t *testing.T) {
	input := cronRequiredInputFactoryConfig()

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertMappedCronTransition(t, net)
	assertMappedSystemTimeExpiryTransition(t, net)
}
