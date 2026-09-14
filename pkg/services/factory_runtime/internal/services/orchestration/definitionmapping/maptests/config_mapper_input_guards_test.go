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

func TestConfigMapping_ExactLineageFailedIdeaRouteMapsAllThreeInputs(t *testing.T) {
	net, err := (testConfigMapper{}).Map(context.Background(), exactLineageFailedIdeaFactoryConfig())
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["complete-reviewed-task-after-failed-idea"]
	if transition == nil {
		t.Fatal("expected exact-lineage completion transition")
	}
	arcs := make(map[string]*factoryruntime.PetriArc, len(transition.InputArcs))
	for index := range transition.InputArcs {
		arc := &transition.InputArcs[index]
		arcs[arc.PlaceID] = arc
	}
	ideaArc, taskArc, reviewArc := arcs["idea:failed"], arcs["task:to-complete"], arcs["review:complete"]
	if ideaArc == nil || taskArc == nil || reviewArc == nil {
		t.Fatalf("expected all exact-lineage inputs, got %#v", transition.InputArcs)
	}
	for name, arc := range map[string]*factoryruntime.PetriArc{"idea": ideaArc, "review": reviewArc} {
		guard, ok := arc.Guard.(*factoryruntime.PetriSameTraceIDGuard)
		if !ok {
			t.Fatalf("%s arc guard = %T, want *factoryruntime.PetriSameTraceIDGuard", name, arc.Guard)
		}
		if guard.MatchBinding != taskArc.Name {
			t.Fatalf("%s arc guard binding = %q, want %q", name, guard.MatchBinding, taskArc.Name)
		}
	}
	if got := len(transition.OutputArcs); got != 3 {
		t.Fatalf("output arc count = %d, want 3", got)
	}
}

func exactLineageFailedIdeaFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "idea",
				States: []interfaces.StateConfig{
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "to-complete", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "review",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "complete-reviewed-task-after-failed-idea",
				Type: interfaces.WorkstationTypeLogical,
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "failed",
						WorkTypeName: "idea",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "task",
						},
					},
					{StateName: "to-complete", WorkTypeName: "task"},
					{
						StateName:    "complete",
						WorkTypeName: "review",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameTraceID,
							MatchInput: "task",
						},
					},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "failed", WorkTypeName: "idea"},
					{StateName: "complete", WorkTypeName: "task"},
					{StateName: "complete", WorkTypeName: "review"},
				},
			},
		},
	}
}
