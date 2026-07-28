package factorydefinition

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
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
	if validation == nil {
		envelope := unavailableValidationErrorEnvelope()
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}

	result, err := validationentry.ValidateFactoryAPI(ctx, factory, validation)
	if err != nil {
		envelope := decodeInputErrorEnvelope("validate factory definition", err)
		return ToolResponse[factoryapi.FactoryValidationResult]{Error: &envelope}
	}
	apiResult := apisurface.FactoryValidationResultToAPI(result)
	return ToolResponse[factoryapi.FactoryValidationResult]{Result: &apiResult}
}
