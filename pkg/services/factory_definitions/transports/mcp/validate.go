package factorydefinition

import (
	"context"
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// Validate runs the canonical Factory validation contract for the
// you.factory_definition.validate MCP tool.
func Validate(
	ctx context.Context,
	validation factorydefinitions.SubmittedDefinitionValidationOperation,
	factory factoryapi.Factory,
) ToolResponse[factoryapi.FactoryValidationResult] {
	if ctx == nil {
		envelope := decodeInputErrorEnvelope("validate factory definition", errMissingRequestContext)
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactoryValidationResult](ctx); done {
		return response
	}
	if validation == nil {
		envelope := unavailableValidationErrorEnvelope()
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}

	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: validationErrorResponse(err)}
	}
	result, err := validation.ValidateSubmittedDefinition(
		ctx,
		factorydefinitions.SubmittedDefinitionValidationRequest{Config: &cfg},
	)
	if err != nil {
		if envelope, ok := contextRequestErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
		}
		if envelope, ok := validationErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
		}
		envelope := opaqueValidationErrorEnvelope()
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}
	apiResult := apisurface.FactoryValidationResultToAPI(result)
	return ToolResponse[factoryapi.FactoryValidationResult]{Result: &apiResult}
}

// ValidateRoot invokes the single Definitions root validation operation. The
// adapter serializes only the generated protocol value; compatibility,
// normalization, and validation policy stay behind the root.
func ValidateRoot(
	ctx context.Context,
	root DefinitionsRoot,
	factory factoryapi.Factory,
) ToolResponse[factoryapi.FactoryValidationResult] {
	if ctx == nil {
		envelope := decodeInputErrorEnvelope("validate factory definition", errMissingRequestContext)
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactoryValidationResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := unavailableDefinitionsErrorEnvelope()
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}
	payload, err := json.Marshal(factory)
	if err != nil {
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: validationErrorResponse(factorydefinitions.ErrInvalidFactoryDefinitionPayload)}
	}
	result, err := root.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	if err != nil {
		if envelope, ok := contextRequestErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
		}
		if envelope, ok := validationErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
		}
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: opaqueValidationErrorEnvelopePtr()}
	}
	apiResult := apisurface.FactoryValidationResultToAPI(result.Validation)
	return ToolResponse[factoryapi.FactoryValidationResult]{Result: &apiResult}
}

func validationErrorResponse(err error) *ToolErrorEnvelope {
	if envelope, ok := validationErrorEnvelope(err); ok {
		return &envelope
	}
	envelope := opaqueValidationErrorEnvelope()
	return &envelope
}

func opaqueValidationErrorEnvelopePtr() *ToolErrorEnvelope {
	envelope := opaqueValidationErrorEnvelope()
	return &envelope
}
