package maptests

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestConfigMapping_PerInputGuard_SameNameGuardedIdeaBeforeTaskPeer(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "idea",
				States: []interfaces.StateConfig{
					{Name: "to-complete", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "to-complete", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "consume",
				Type: interfaces.WorkstationTypeLogical,
				Inputs: []interfaces.IOConfig{
					{
						StateName:    "to-complete",
						WorkTypeName: "idea",
						Guard: &interfaces.InputGuardConfig{
							Type:       interfaces.GuardTypeSameName,
							MatchInput: "task",
						},
					},
					{StateName: "to-complete", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "idea"},
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	net, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	transition := net.Transitions["consume"]
	if transition == nil {
		t.Fatal("expected consume transition")
	}

	var ideaArc *factoryruntime.PetriArc
	var taskArc *factoryruntime.PetriArc
	for i := range transition.InputArcs {
		arc := &transition.InputArcs[i]
		switch arc.PlaceID {
		case "idea:to-complete":
			ideaArc = arc
		case "task:to-complete":
			taskArc = arc
		}
	}

	if ideaArc == nil || taskArc == nil {
		t.Fatalf("expected idea/task input arcs, got %#v", transition.InputArcs)
	}
	if ideaArc.Guard == nil {
		t.Fatal("expected same-name guard on idea input arc")
	}
	guard, ok := ideaArc.Guard.(*factoryruntime.PetriSameNameGuard)
	if !ok {
		t.Fatalf("idea arc guard = %T, want *factoryruntime.PetriSameNameGuard", ideaArc.Guard)
	}
	if guard.MatchBinding != taskArc.Name {
		t.Fatalf("same-name guard binding = %q, want %q", guard.MatchBinding, taskArc.Name)
	}
}
