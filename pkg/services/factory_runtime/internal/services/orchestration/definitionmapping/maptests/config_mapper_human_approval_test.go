package maptests

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestConfigMapping_HumanApprovalIsWorkerlessWithDistinctRoutes(t *testing.T) {
	net, err := (testConfigMapper{}).Map(context.Background(), &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "review",
				States: []interfaces.StateConfig{
					{Name: "pending", Type: interfaces.StateTypeInitial},
					{Name: "approved", Type: interfaces.StateTypeTerminal},
					{Name: "rejected", Type: interfaces.StateTypeProcessing},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:        "approval",
				Type:        interfaces.WorkstationTypeHumanApproval,
				Inputs:      []interfaces.IOConfig{{WorkTypeName: "review", StateName: "pending"}},
				Outputs:     []interfaces.IOConfig{{WorkTypeName: "review", StateName: "approved"}},
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "review", StateName: "rejected"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("map HUMAN_APPROVAL factory: %v", err)
	}

	transition := net.Transitions["approval"]
	if transition == nil {
		t.Fatal("expected approval transition")
	}
	if transition.Type != factoryruntime.PetriTransitionHumanApproval {
		t.Fatalf("transition type = %q, want %q", transition.Type, factoryruntime.PetriTransitionHumanApproval)
	}
	if transition.WorkerType != "" {
		t.Fatalf("worker type = %q, want empty", transition.WorkerType)
	}
	if len(transition.InputArcs) != 1 || transition.InputArcs[0].PlaceID != "review:pending" {
		t.Fatalf("input arcs = %+v", transition.InputArcs)
	}
	if len(transition.OutputArcs) != 1 || transition.OutputArcs[0].PlaceID != "review:approved" {
		t.Fatalf("approval output arcs = %+v", transition.OutputArcs)
	}
	if len(transition.RejectionArcs) != 1 || transition.RejectionArcs[0].PlaceID != "review:rejected" {
		t.Fatalf("rejection arcs = %+v", transition.RejectionArcs)
	}
	if len(transition.FailureArcs) != 0 {
		t.Fatalf("human approval should not gain an implicit failure route: %+v", transition.FailureArcs)
	}
}
