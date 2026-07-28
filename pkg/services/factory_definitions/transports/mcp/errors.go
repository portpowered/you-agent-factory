package factorydefinition

import (
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	errorCodeBadRequest             = "BAD_REQUEST"
	errorCodeInvalidFactory         = "INVALID_FACTORY"
	invalidFactoryDefinitionMessage = "Factory payload is not a valid Agent Factory definition."
	invalidRequestPayloadMessage    = "invalid request payload"
)

func decodeInputErrorEnvelope(operation string, err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   fmt.Sprintf("%s: %v", operation, err),
		Retryable: false,
	}
}

func unavailableValidationErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_definition.service.unavailable",
		Message:   "factory definition validation is unavailable",
		Retryable: false,
	}
}

func validationErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	if err == nil {
		return ToolErrorEnvelope{}, false
	}
	if envelope, ok := invalidFactoryDefinitionPayloadErrorEnvelope(err); ok {
		return envelope, true
	}
	if envelope, ok := validationFailedErrorEnvelope(err); ok {
		return envelope, true
	}

	var topologyErr *apisurface.TopologyValidationError
	var domainTopologyErr *factorydefinitions.ValidationTopologyError
	switch {
	case errors.As(err, &topologyErr):
		return invalidFactoryErrorEnvelope(topologyErr.Targets), true
	case errors.As(err, &domainTopologyErr):
		return invalidFactoryErrorEnvelope(
			apisurface.FactoryValidationTargetsToAPI(domainTopologyErr.Targets),
		), true
	case errors.Is(err, apisurface.ErrInvalidNamedFactory):
		return invalidFactoryErrorEnvelope([]factoryapi.FactoryValidationTarget{
			apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
		}), true
	default:
		return ToolErrorEnvelope{}, false
	}
}

func invalidFactoryDefinitionPayloadErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	if !errors.Is(err, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		return ToolErrorEnvelope{}, false
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   invalidRequestPayloadMessage,
		Retryable: false,
	}, true
}

func validationFailedErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	var validationFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if errors.As(err, &validationFailure) {
		return invalidFactoryErrorEnvelope(
			apisurface.FactoryValidationTargetsToAPI(validationFailure.Validation.Targets),
		), true
	}
	if errors.Is(err, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		return invalidFactoryErrorEnvelope(nil), true
	}
	return ToolErrorEnvelope{}, false
}

func invalidFactoryErrorEnvelope(targets []factoryapi.FactoryValidationTarget) ToolErrorEnvelope {
	if len(targets) == 0 {
		targets = []factoryapi.FactoryValidationTarget{
			apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
		}
	}
	return ToolErrorEnvelope{
		Code:      errorCodeInvalidFactory,
		Message:   invalidFactoryDefinitionMessage,
		Retryable: false,
		Details: map[string]any{
			"targets": targets,
		},
	}
}

func opaqueValidationErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   invalidRequestPayloadMessage,
		Retryable: false,
	}
}
