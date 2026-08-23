package mappingtests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactoryConfigMapper_RoundTripsFactoryAndWorkstationRunners(t *testing.T) {
	mapper := NewFactoryConfigMapper()
	original := &interfaces.FactoryConfig{
		Name:   "runner-selection-round-trip",
		Runner: "antigravity",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			Type:           interfaces.WorkstationTypeAgent,
			WorkerTypeName: "executor",
			Runner:         "claude",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}

	flattened, err := mapper.Flatten(original)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}
	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	if expanded.Runner != original.Runner {
		t.Fatalf("expanded factory runner = %q, want %q", expanded.Runner, original.Runner)
	}
	if len(expanded.Workstations) != 1 || expanded.Workstations[0].Runner != original.Workstations[0].Runner {
		t.Fatalf("expanded workstation runners = %#v, want %q", expanded.Workstations, original.Workstations[0].Runner)
	}
}
