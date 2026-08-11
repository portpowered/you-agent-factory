package definitionmapping

import (
	"context"
	"fmt"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResourceMappingUsesStableIDForMutableDisplayName(t *testing.T) {
	var nextID int
	mapper, err := New(func() string {
		nextID++
		return fmt.Sprintf("arc-%d", nextID)
	})
	if err != nil {
		t.Fatalf("New mapper: %v", err)
	}
	config := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Resources: []interfaces.ResourceConfig{{ID: "gpu-1", Name: "GPU pool", Capacity: 2}},
		Workers:   []interfaces.FactoryWorkerConfig{{Name: "worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "run", WorkerTypeName: "worker",
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			Resources: []interfaces.ResourceConfig{{Name: "GPU pool", Capacity: 1}},
		}},
	}
	net, err := mapper.Map(context.Background(), config)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if net.Resources["gpu-1"] == nil || net.Resources["gpu-1"].Name != "GPU pool" {
		t.Fatalf("resources = %#v, want stable gpu-1 with display name", net.Resources)
	}
	if _, ok := net.Resources["GPU pool"]; ok {
		t.Fatal("display name was used as the resource map key")
	}
	if net.Places["gpu-1:available"] == nil {
		t.Fatal("stable resource availability place is missing")
	}
	transition := net.Transitions["run"]
	if transition == nil || len(transition.InputArcs) < 2 || transition.InputArcs[1].PlaceID != "gpu-1:available" {
		t.Fatalf("resource input arcs = %#v, want gpu-1:available", transition)
	}
}

func TestCombinedTransitionResourceUsageDeduplicatesWorkerAndWorkstationRequirement(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "executor",
			Resources: []interfaces.ResourceConfig{{
				Name: "agent-slot", Capacity: 1,
			}},
		}},
	}
	got := combinedTransitionResourceUsage(cfg, interfaces.FactoryWorkstationConfig{
		WorkerTypeName: "executor",
		Resources: []interfaces.ResourceConfig{{
			Name: "agent-slot", Capacity: 1,
		}},
	})
	if len(got) != 1 || got[0].Name != "agent-slot" || got[0].Capacity != 1 {
		t.Fatalf("combined resources = %#v, want one aligned agent-slot requirement", got)
	}
}

func TestCombinedTransitionResourceUsageUsesStricterRepeatedRequirement(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "executor",
			Resources: []interfaces.ResourceConfig{{
				Name: "gpu", Capacity: 1,
			}},
		}},
	}
	got := combinedTransitionResourceUsage(cfg, interfaces.FactoryWorkstationConfig{
		WorkerTypeName: "executor",
		Resources: []interfaces.ResourceConfig{{
			Name: "gpu", Capacity: 2,
		}},
	})
	if len(got) != 1 || got[0].Capacity != 2 {
		t.Fatalf("combined resources = %#v, want stricter capacity 2", got)
	}
}
