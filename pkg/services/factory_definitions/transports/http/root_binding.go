package http

import (
	"context"
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// DefinitionsRoot is the accepted Factory Definitions root contract used by the
// HTTP adapter. Adapter-owned operations invoke this surface rather than
// Definitions internal packages.
type DefinitionsRoot = factorydefinitions.Service

// NewTopologyValidation explicitly adapts the existing structural/topology
// policy for HTTP callers that previously omitted Validation.
func NewTopologyValidation(
	definitions factorydefinitions.Service,
) factorydefinitions.SubmittedDefinitionValidationOperation {
	return submittedDefinitionValidationFromRoot{root: definitions}
}

type submittedDefinitionValidationFromRoot struct {
	root DefinitionsRoot
}

func (adapter submittedDefinitionValidationFromRoot) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if request.Config == nil {
		return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	payload, err := json.Marshal(request.Config)
	if err != nil {
		return factorydefinitions.ValidationResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	result, err := adapter.root.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	return result.Validation, nil
}
