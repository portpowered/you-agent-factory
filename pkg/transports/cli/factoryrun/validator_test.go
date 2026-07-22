package factoryrun

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// promptRunValidationFailureValidator scripts the exact Factory Definitions edge so
// these transport tests cover CLI error projection, not concrete validation
// policy. Handling-behavior policy remains covered by Factory Definitions.
type promptRunValidationFailureValidator struct{}

func (promptRunValidationFailureValidator) ValidateEffectiveDefinition(
	context.Context,
	factorydefinitions.EffectiveDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return factorydefinitions.ValidationResult{Targets: []factorydefinitions.ValidationTarget{{
		Code:     "factory.invocationReturn.missingDefaultWorkType",
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  "expected exactly one work type with handlingBehavior DEFAULT",
		Subject: factorydefinitions.ValidationSubject{
			Type: factorydefinitions.ValidationSubjectTypeFactory,
		},
	}}}, nil
}
