package factorydefinition

import (
	"context"
	"encoding/json"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// DefinitionsRoot is the accepted Factory Definitions root contract used by the
// MCP adapter. Adapter-owned operations invoke this surface rather than
// Definitions internal packages.
type DefinitionsRoot = factorydefinitions.Service

// RootBinding binds the MCP adapter to one injected Definitions root and
// optional root-shaped validation and distribute roles already used by
// HTTP-DEF and CLI-DEF.
type RootBinding struct {
	Definitions DefinitionsRoot
	Validation  factorydefinitions.SubmittedDefinitionValidationOperation
	Install     factorydefinitions.InstallPackagedFactoryOperation
}

// NewFromRoot constructs an MCP tool operation that calls through the supplied
// Definitions root binding. Tests inject focused fakes without constructing
// real catalog, authoring, compile, validate, snapshot, distribute, or
// service-local Wire graphs.
func NewFromRoot(binding RootBinding) ToolOperation {
	return Bind(binding)
}

func resolveSubmittedDefinitionValidation(binding RootBinding) (factorydefinitions.SubmittedDefinitionValidationOperation, error) {
	if binding.Validation != nil {
		return binding.Validation, nil
	}
	if binding.Definitions == nil {
		return nil, errValidationUnavailable
	}
	return submittedDefinitionValidationFromRoot{binding.Definitions}, nil
}

var errValidationUnavailable = fmt.Errorf("factory definition validation is unavailable")

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
