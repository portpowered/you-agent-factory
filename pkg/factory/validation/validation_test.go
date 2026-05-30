package validation_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestValidate_EquivalentTargetsForInvalidFactoryThroughConfigAndPackageValidation(t *testing.T) {
	t.Parallel()

	apiFactory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	cfg, err := config.FactoryConfigFromOpenAPI(apiFactory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	configFindings := config.CanonicalStructuralFindings(&cfg)

	if len(explicit.Targets) == 0 || len(configFindings) == 0 {
		t.Fatalf("explicit targets = %d, config findings = %d, want both non-empty", len(explicit.Targets), len(configFindings))
	}
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, explicit targets = %d, want equivalent coverage", len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("finding[%d].Rule = %q, want explicit target code %q", index, configFindings[index].Rule, target.Code)
		}
	}

	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDuplicateIdentifier)
	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDanglingWorkerReference)
	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDanglingPlaceReference)
	validationassert.HasDomainTargetSubject(t, explicit.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "process",
		Location: factoryvalidation.SubjectLocationReference,
	})
}

func TestValidate_MissingOutcomeRoutesUseCanonicalWorkstationLocations(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "in-review", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "repeater",
			Kind:           interfaces.WorkstationKindRepeater,
			WorkerTypeName: "worker-a",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "repeater",
		Location: factoryvalidation.SubjectLocationOnFailure,
	})
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "repeater",
		Location: factoryvalidation.SubjectLocationOnRejection,
	})
}

func TestValidate_RepresentativeCanonicalSubjects(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			Kind:           interfaces.WorkstationKindRepeater,
			WorkerTypeName: "worker-a",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "process",
		Location: factoryvalidation.SubjectLocationOnRejection,
	})
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkType,
		ID:       "task",
		Location: factoryvalidation.SubjectLocationStates,
	})
}

func TestValidate_MissingWorkTypeOutcomeStates(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeMissingCompletionState)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeMissingFailureState)
}
