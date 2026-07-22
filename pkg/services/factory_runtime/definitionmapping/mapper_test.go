package definitionmapping

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

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
