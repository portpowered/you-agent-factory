package maptests

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// --- per-input guard tests ---

func TestConfigMapping_PerInputGuard_StaticAllChildrenComplete(t *testing.T) {
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), staticAllChildrenCompleteFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	assertStaticAllChildrenCompleteCollector(t, outputNet)
}

func TestConfigMapping_PerInputGuard_DynamicFanout(t *testing.T) {
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), dynamicFanoutFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	assertDynamicFanoutTransition(t, outputNet)
}

func TestConfigMapping_PerInputGuard_AnyChildFailed(t *testing.T) {
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), anyChildFailedFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	assertAnyChildFailedFailureChecker(t, outputNet)
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

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected match-items transition")
	}

	var planArc *factoryruntime.PetriArc
	var taskArc *factoryruntime.PetriArc
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
	if taskArc.Cardinality.Mode != factoryruntime.PetriCardinalityOne {
		t.Fatalf("same-name guarded arc cardinality = %v, want one", taskArc.Cardinality.Mode)
	}
	guard, ok := taskArc.Guard.(*factoryruntime.PetriSameNameGuard)
	if !ok {
		t.Fatalf("same-name guarded arc guard = %T, want *factoryruntime.PetriSameNameGuard", taskArc.Guard)
	}
	if guard.MatchBinding != planArc.Name {
		t.Fatalf("same-name guard binding = %q, want %q", guard.MatchBinding, planArc.Name)
	}
}

func TestConfigMapping_PerInputGuard_SameTraceIDBuildsConsumeGuardAgainstPeerInput(t *testing.T) {
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
							Type:       interfaces.GuardTypeSameTraceID,
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

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["match-items"]
	if transition == nil {
		t.Fatal("expected match-items transition")
	}

	var planArc *factoryruntime.PetriArc
	var taskArc *factoryruntime.PetriArc
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
		t.Fatalf("same-trace guarded arc mode = %v, want consume", taskArc.Mode)
	}
	if taskArc.Cardinality.Mode != factoryruntime.PetriCardinalityOne {
		t.Fatalf("same-trace guarded arc cardinality = %v, want one", taskArc.Cardinality.Mode)
	}
	guard, ok := taskArc.Guard.(*factoryruntime.PetriSameTraceIDGuard)
	if !ok {
		t.Fatalf("same-trace guarded arc guard = %T, want *factoryruntime.PetriSameTraceIDGuard", taskArc.Guard)
	}
	if guard.MatchBinding != planArc.Name {
		t.Fatalf("same-trace guard binding = %q, want %q", guard.MatchBinding, planArc.Name)
	}
}
