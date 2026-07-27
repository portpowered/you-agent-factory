package structural_test

import (
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/structural"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

func validPetriFactoryConfig() *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "structural-validation",
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

func TestValidate_ValidPetriFactoryHasNoBlockingStructuralTargets(t *testing.T) {
	t.Parallel()

	result := structural.Validate(validPetriFactoryConfig())
	if result.HasBlockingTargets() {
		t.Fatalf("structural targets = %#v, want none", result.Targets)
	}
}

func TestValidate_DuplicateWorkerReturnsTypedStructuralTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workers = append(cfg.Workers, workerconfig.Config{Name: "worker-a"})

	result := structural.Validate(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking structural targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDuplicateIdentifier &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeWorker {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want duplicate worker structural target", result.Targets)
	}
}
