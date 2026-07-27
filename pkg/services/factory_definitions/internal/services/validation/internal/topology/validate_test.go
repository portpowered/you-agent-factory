package topology_test

import (
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/topology"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

func validPetriFactoryConfig() *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "topology-validation",
		WorkTypes: []factorycontracts.WorkTypeConfig{{
			Name: "task",
			States: []factorycontracts.StateConfig{
				{Name: "init", Type: factorycontracts.StateTypeInitial},
				{Name: "done", Type: factorycontracts.StateTypeTerminal},
				{Name: "failed", Type: factorycontracts.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorycontracts.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

func TestValidate_ValidPetriFactoryHasNoBlockingTopologyTargets(t *testing.T) {
	t.Parallel()

	result := topology.Validate(validPetriFactoryConfig())
	if result.HasBlockingTargets() {
		t.Fatalf("topology targets = %#v, want none", result.Targets)
	}
}

func TestValidate_DanglingPlaceReferenceReturnsTypedTopologyTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workstations[0].Outputs = []factorycontracts.IOConfig{{
		WorkTypeName: "task",
		StateName:    "bogus",
	}}

	result := topology.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking topology targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDanglingPlaceReference &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeRoute {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want dangling place topology target", result.Targets)
	}
}

func TestValidate_DanglingWorkerReferenceReturnsTypedTopologyTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workstations[0].WorkerTypeName = "missing-worker"

	result := topology.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking topology targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDanglingWorkerReference &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeWorkstation {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want dangling worker topology target", result.Targets)
	}
}
