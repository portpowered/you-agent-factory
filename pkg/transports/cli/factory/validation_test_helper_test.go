package factory

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type testFactoryDefinitionValidator struct {
	validate             func() factorydefinitions.ValidationResult
	validateBlockingLoad func() factorydefinitions.ValidationResult
	pruneLayout          func() factorydefinitions.ValidationResult
}

func testTopologyFactoryDefinitionValidator(
	result factorydefinitions.ValidationResult,
) testFactoryDefinitionValidator {
	return testFactoryDefinitionValidator{
		validate: func() factorydefinitions.ValidationResult { return result },
	}
}

func (v testFactoryDefinitionValidator) ValidateSubmittedDefinition(
	_ context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if v.validate == nil {
		panic("unexpected Factory Definition ValidateSubmittedDefinition call")
	}
	result := v.validate()
	return result, nil
}

func testPersistableFactoryDefinitionValidator(
	blockingLoadResult factorydefinitions.ValidationResult,
) factorydefinitions.Validator {
	return testFactoryDefinitionValidator{
		validate: func() factorydefinitions.ValidationResult {
			return factorydefinitions.ValidationResult{}
		},
		validateBlockingLoad: func() factorydefinitions.ValidationResult {
			return blockingLoadResult
		},
		pruneLayout: func() factorydefinitions.ValidationResult {
			return factorydefinitions.ValidationResult{}
		},
	}
}

// testValidPersistableFactoryDefinitionValidator scripts the validation edge
// for CLI tests whose subject is persistence policy, filesystem effects, or
// output rendering. Structural acceptance is covered by the owner-local
// Factory Definitions validation suites.
func testValidPersistableFactoryDefinitionValidator() factorydefinitions.Validator {
	return testPersistableFactoryDefinitionValidator(factorydefinitions.ValidationResult{})
}

func (v testFactoryDefinitionValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	if v.validate == nil {
		panic("unexpected Factory Definition Validate call")
	}
	return v.validate()
}

func (v testFactoryDefinitionValidator) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	if v.validateBlockingLoad == nil {
		panic("unexpected Factory Definition ValidateBlockingLoad call")
	}
	return v.validateBlockingLoad()
}

func (testFactoryDefinitionValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	panic("unexpected Factory Definition ValidateTopology call")
}

func (testFactoryDefinitionValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	panic("unexpected Factory Definition WorkerWorkstationBehaviorCompatibility call")
}

func (testFactoryDefinitionValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	panic("unexpected Factory Definition WorkTypeHandlingBehavior call")
}

func (v testFactoryDefinitionValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	if v.pruneLayout == nil {
		panic("unexpected Factory Definition PruneLayout call")
	}
	return v.pruneLayout()
}

func testFactoryDefinitionValidationFailure(
	code string,
	message string,
	subjectID string,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{
			Code:     code,
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  message,
			Subject: factorydefinitions.ValidationSubject{
				Type: factorydefinitions.ValidationSubjectTypeWorkstation,
				ID:   subjectID,
			},
		}},
	}
}
