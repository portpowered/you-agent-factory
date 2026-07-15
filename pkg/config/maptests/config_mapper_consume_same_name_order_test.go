package maptests

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
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

	var ideaArc *petri.Arc
	var taskArc *petri.Arc
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
	guard, ok := ideaArc.Guard.(*petri.SameNameGuard)
	if !ok {
		t.Fatalf("idea arc guard = %T, want *petri.SameNameGuard", ideaArc.Guard)
	}
	if guard.MatchBinding != taskArc.Name {
		t.Fatalf("same-name guard binding = %q, want %q", guard.MatchBinding, taskArc.Name)
	}
}
