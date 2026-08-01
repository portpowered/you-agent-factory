package topology_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/authoredmodel/workers"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/topology"
)

func validPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "topology-validation",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
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
	cfg.Workstations[0].Outputs = []factorydefinitions.IOConfig{{
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
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeRoute {
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
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorkstation {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want dangling worker topology target", result.Targets)
	}
}
