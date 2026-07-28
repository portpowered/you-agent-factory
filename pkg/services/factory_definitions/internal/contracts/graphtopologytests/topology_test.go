package graphtopologytests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

func TestBuildPendingFactoryGraphTopology_UsesCanonicalNodeAndEdgeIDs(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				{Type: interfaces.BundledFileTypeDoc, TargetPath: "factory/docs/guide.md"},
				{Type: interfaces.BundledFileTypeScript, TargetPath: "factory/scripts/setup.py"},
			},
		},
		Resources: []catalogresource.Config{{
			ID:   "resource-slot",
			Name: "executor-slot",
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			ID:   "work-type-story",
			Name: "story",
			States: []interfaces.StateConfig{
				{ID: "state-init", Name: "init", Type: interfaces.StateTypeInitial},
				{ID: "state-done", Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []workerconfig.Config{{
			ID:   "worker-executor",
			Name: "executor",
			Resources: []catalogresource.Config{{
				Name: "executor-slot",
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "workstation-plan",
			Name:           "plan",
			WorkerTypeName: "executor",
			Resources: []catalogresource.Config{{
				ID:   "resource-slot",
				Name: "executor-slot",
			}},
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "done"}},
		}},
	}

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)

	wantNodes := []string{
		"doc:factory/docs/guide.md",
		"doc:factory/scripts/setup.py",
		"script:factory/scripts/setup.py",
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
