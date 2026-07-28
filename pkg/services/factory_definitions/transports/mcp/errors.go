package factorydefinition

import (
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	errorCodeBadRequest                 = "BAD_REQUEST"
	errorCodeInvalidFactory             = "INVALID_FACTORY"
	errorCodeSessionNotFound            = "factory_definition.session.not_found"
	errorCodeCurrentFactoryNotFound     = "factory_definition.current_factory.not_found"
	errorCodeStaleFactoryVersion        = "STALE_FACTORY_VERSION"
	errorCodeInternalCurrentFactory     = "factory_definition.current_factory.internal"
	errorCodeUnknownPackagedFactory     = "factory_definition.packaged.unknown_identity"
	errorCodeFactoryAlreadyExists       = "FACTORY_ALREADY_EXISTS"
	errorCodePackagedDistributeFailed   = "factory_definition.packaged.distribute_failed"
	errorMessageSessionNotFound         = "factory session not found"
	errorMessageCurrentFactoryNotFound  = "Current factory not found."
	errorMessageStaleFactoryVersion     = "Current factory definition is stale. Refresh the graph before saving."
	errorMessageInternalCurrentFactory  = "failed to load current factory"
	errorMessageMissingPackageIdentity  = "package name is required"
	errorMessageNamedFactoryExists      = "Named factory already exists."
	errorMessagePackagedDistributeFailed = "factory distribute failed"
	invalidFactoryDefinitionMessage   = "Factory payload is not a valid Agent Factory definition."
	invalidRequestPayloadMessage      = "invalid request payload"
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

func unavailableDefinitionsErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_definition.service.unavailable",
		Message:   "factory definition service is unavailable",
		Retryable: false,
	}
}

func currentFactoryErrorEnvelope(sessionID string, action string, err error) ToolErrorEnvelope {
	if err == nil {
		return opaqueCurrentFactoryErrorEnvelope(action)
	}
	if envelope, ok := validationErrorEnvelope(err); ok {
		return envelope
	}
	if envelope, ok := invalidFactoryDefinitionPayloadErrorEnvelope(err); ok {
		return envelope
	}
	switch {
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return sessionNotFoundErrorEnvelope(sessionID)
	case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
		return currentFactoryNotFoundErrorEnvelope()
	case errors.Is(err, apisurface.ErrFactoryVersionStale):
		return staleFactoryVersionErrorEnvelope()
	default:
		return opaqueCurrentFactoryErrorEnvelope(action)
	}
}

func sessionNotFoundErrorEnvelope(sessionID string) ToolErrorEnvelope {
	details := map[string]any{}
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		details["sessionId"] = trimmed
	}
	return ToolErrorEnvelope{
		Code:      errorCodeSessionNotFound,
		Message:   errorMessageSessionNotFound,
		Retryable: false,
		Details:   details,
	}
}

func currentFactoryNotFoundErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeCurrentFactoryNotFound,
		Message:   errorMessageCurrentFactoryNotFound,
		Retryable: false,
	}
}

func staleFactoryVersionErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeStaleFactoryVersion,
		Message:   errorMessageStaleFactoryVersion,
		Retryable: false,
		Details: map[string]any{
			"targets": []factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(factorydefinitions.StaleFactoryVersionValidationTarget()),
			},
		},
	}
}

func opaqueCurrentFactoryErrorEnvelope(action string) ToolErrorEnvelope {
	message := errorMessageInternalCurrentFactory
	if action == "save" {
		message = "failed to save current factory"
	}
	return ToolErrorEnvelope{
		Code:      errorCodeInternalCurrentFactory,
		Message:   message,
		Retryable: false,
	}
}

func unavailableInstallErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      "factory_definition.service.unavailable",
		Message:   "packaged factory installation is unavailable",
		Retryable: false,
	}
}

func missingPackageIdentityErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   errorMessageMissingPackageIdentity,
		Retryable: false,
	}
}

func installPackagedErrorEnvelope(err error) ToolErrorEnvelope {
	if err == nil {
		return opaqueInstallPackagedErrorEnvelope()
	}
	if envelope, ok := unknownPackagedFactoryErrorEnvelope(err); ok {
		return envelope
	}
	if errors.Is(err, factorydefinitions.ErrIncompatibleFactoryDistributeOptions) {
		return ToolErrorEnvelope{
			Code:      errorCodeBadRequest,
			Message:   err.Error(),
			Retryable: false,
		}
	}
	if errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists) {
		return ToolErrorEnvelope{
			Code:      errorCodeFactoryAlreadyExists,
			Message:   errorMessageNamedFactoryExists,
			Retryable: false,
		}
	}
	if errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		return ToolErrorEnvelope{
			Code:      errorCodePackagedDistributeFailed,
			Message:   errorMessagePackagedDistributeFailed,
			Retryable: false,
		}
	}
	return opaqueInstallPackagedErrorEnvelope()
}

func unknownPackagedFactoryErrorEnvelope(err error) (ToolErrorEnvelope, bool) {
	var unknown *factorydefinitions.UnknownPackagedFactoryError
	if !errors.As(err, &unknown) && !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		return ToolErrorEnvelope{}, false
	}
	details := map[string]any{}
	if unknown != nil {
		if trimmed := strings.TrimSpace(unknown.Name); trimmed != "" {
			details["package"] = trimmed
		}
		if len(unknown.Available) > 0 {
			details["available"] = append([]string(nil), unknown.Available...)
		}
	}
	message := factorydefinitions.ErrUnknownPackagedFactoryIdentity.Error()
	if unknown != nil && unknown.Error() != "" {
		message = unknown.Error()
	}
	return ToolErrorEnvelope{
		Code:      errorCodeUnknownPackagedFactory,
		Message:   message,
		Retryable: false,
		Details:   details,
	}, true
}

func opaqueInstallPackagedErrorEnvelope() ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodePackagedDistributeFailed,
		Message:   errorMessagePackagedDistributeFailed,
		Retryable: false,
	}
}
