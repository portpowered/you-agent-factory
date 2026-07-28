package http

import (
	"errors"
	"net/http"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

const (
	invalidFactoryDefinitionMessage = "Factory payload is not a valid Agent Factory definition."
	invalidRequestPayloadMessage    = "invalid request payload"
)

// definitionsRootErrorResponse maps typed Definitions root failures to HTTP status
// and a public ErrorResponse body.
func definitionsRootErrorResponse(err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	if status, response, ok := definitionsInvalidFactoryDefinitionPayloadErrorResponse(err); ok {
		return status, response, true
	}
	if status, response, ok := definitionsValidationFailedErrorResponse(err); ok {
		return status, response, true
	}

	var topologyErr *apisurface.TopologyValidationError
	var domainTopologyErr *factorydefinitions.ValidationTopologyError
	switch {
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "factory session not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	case errors.Is(err, apisurface.ErrCurrentFactoryNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "Current factory not found.",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	case errors.Is(err, apisurface.ErrInvalidNamedFactoryName):
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.",
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCode("INVALID_FACTORY_NAME"),
			Targets: validationTargetsPtr([]factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
			}),
		}, true
	case errors.Is(err, apisurface.ErrFactoryVersionStale):
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: "Current factory definition is stale. Refresh the graph before saving.",
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode("STALE_FACTORY_VERSION"),
			Targets: validationTargetsPtr([]factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(factorydefinitions.StaleFactoryVersionValidationTarget()),
			}),
		}, true
	case errors.As(err, &topologyErr):
		return invalidFactoryErrorResponse(topologyErr.Targets)
	case errors.As(err, &domainTopologyErr):
		return invalidFactoryErrorResponse(apisurface.FactoryValidationTargetsToAPI(domainTopologyErr.Targets))
	case errors.Is(err, apisurface.ErrInvalidNamedFactory):
		return invalidFactoryErrorResponse([]factoryapi.FactoryValidationTarget{
			apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
		})
	case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: "Current factory runtime must be idle before activation.",
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode("FACTORY_NOT_IDLE"),
			Targets: validationTargetsPtr([]factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(factorydefinitions.FactoryRuntimeNotIdleValidationTarget()),
			}),
		}, true
	case errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists):
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: "Named factory already exists.",
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode("FACTORY_ALREADY_EXISTS"),
		}, true
	default:
		return 0, nil, false
	}
}

func definitionsInvalidFactoryDefinitionPayloadErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if !errors.Is(err, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		return 0, factoryapi.ErrorResponse{}, false
	}
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: invalidRequestPayloadMessage,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCodeBADREQUEST,
	}, true
}

func definitionsValidationFailedErrorResponse(err error) (int, any, bool) {
	var validationFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if errors.As(err, &validationFailure) {
		return invalidFactoryErrorResponse(
			apisurface.FactoryValidationTargetsToAPI(validationFailure.Validation.Targets),
		)
	}
	if errors.Is(err, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		return invalidFactoryErrorResponse(nil)
	}
	return 0, nil, false
}

func invalidFactoryErrorResponse(targets []factoryapi.FactoryValidationTarget) (int, factoryapi.ErrorResponse, bool) {
	if len(targets) == 0 {
		targets = []factoryapi.FactoryValidationTarget{
			apisurface.FactoryValidationTargetToAPI(factorydefinitions.FormFactoryPayloadValidationTarget()),
		}
	}
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: invalidFactoryDefinitionMessage,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCode("INVALID_FACTORY"),
		Targets: validationTargetsPtr(targets),
	}, true
}

func validationTargetsPtr(targets []factoryapi.FactoryValidationTarget) *[]factoryapi.FactoryValidationTarget {
	if len(targets) == 0 {
		return nil
	}
	return &targets
}

func (s *Server) writeDefinitionsRootError(w http.ResponseWriter, err error) bool {
	if status, response, ok := definitionsRootErrorResponse(err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) writeDefinitionsRootErrorOrInternal(
	w http.ResponseWriter,
	err error,
	fallbackMessage string,
	fields ...zap.Field,
) {
	if s.writeDefinitionsRootError(w, err) {
		return
	}
	logFields := append([]zap.Field(nil), fields...)
	logFields = append(logFields, zap.Error(err))
	if errors.Is(err, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		s.logger.Error("definitions root write failed", logFields...)
	} else {
		s.logger.Error("definitions root request failed", logFields...)
	}
	message := strings.TrimSpace(fallbackMessage)
	if message == "" {
		message = "factory definitions request failed"
	}
	s.writeError(w, http.StatusInternalServerError, message, string(factoryapi.ErrorResponseCodeINTERNALERROR))
}
