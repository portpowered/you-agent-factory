package structural_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/structural"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func validPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "structural-validation",
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
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorker {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want duplicate worker structural target", result.Targets)
	}
}
