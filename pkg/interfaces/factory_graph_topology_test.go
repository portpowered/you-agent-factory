package interfaces

import "testing"

func TestBuildPendingFactoryGraphTopology_UsesCanonicalNodeAndEdgeIDs(t *testing.T) {
	t.Parallel()

	cfg := &FactoryConfig{
		Resources: []ResourceConfig{{
			ID:   "resource-slot",
			Name: "executor-slot",
		}},
		WorkTypes: []WorkTypeConfig{{
			ID:   "work-type-story",
			Name: "story",
			States: []StateConfig{
				{ID: "state-init", Name: "init", Type: StateTypeInitial},
				{ID: "state-done", Name: "done", Type: StateTypeTerminal},
			},
		}},
		Workers: []WorkerConfig{{
			ID:   "worker-executor",
			Name: "executor",
			Resources: []ResourceConfig{{
				Name: "executor-slot",
			}},
		}},
		Workstations: []FactoryWorkstationConfig{{
			ID:             "workstation-plan",
			Name:           "plan",
			WorkerTypeName: "executor",
			Resources: []ResourceConfig{{
				ID:   "resource-slot",
				Name: "executor-slot",
			}},
			Inputs:  []IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs: []IOConfig{{WorkTypeName: "story", StateName: "done"}},
		}},
	}

	topology := BuildPendingFactoryGraphTopology(cfg)

	wantNodes := []string{
		"resource:resource-slot",
		"work-type:work-type-story",
		"work-state:work-type-story:state-init",
		"work-state:work-type-story:state-done",
		"worker:worker-executor",
		"workstation:workstation-plan",
	}
	for _, nodeID := range wantNodes {
		if _, ok := topology.NodeIDs[nodeID]; !ok {
			t.Fatalf("topology missing node %q; nodes = %#v", nodeID, topology.NodeIDs)
		}
	}

	wantEdges := []string{
		"work-type-state:work-type:work-type-story->work-state:work-type-story:state-init",
		"work-type-state:work-type:work-type-story->work-state:work-type-story:state-done",
		"worker-resource:resource:resource-slot->worker:worker-executor",
		"worker-assignment:worker:worker-executor->workstation:workstation-plan",
		"workstation-resource:resource:resource-slot->workstation:workstation-plan",
		"workstation-input:work-state:work-type-story:state-init->workstation:workstation-plan",
		"workstation-output:workstation:workstation-plan->work-state:work-type-story:state-done",
	}
	for _, edgeID := range wantEdges {
		if _, ok := topology.EdgeIDs[edgeID]; !ok {
			t.Fatalf("topology missing edge %q; edges = %#v", edgeID, topology.EdgeIDs)
		}
	}
}
