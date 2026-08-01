package errors

import (
	"errors"
	"net/http"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const visualizationRequestFailedMessage = "factory visualization request failed"

// RootErrorResponse maps typed Visualization root failures to HTTP outcomes.
func RootErrorResponse(err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	if status, response, ok := common.RequestContextErrorResponse(err); ok {
		return status, response, true
	}

	var lifeErr *factoryvisualization.LifecycleError
	if errors.As(err, &lifeErr) && lifeErr != nil {
		return lifecycleErrorResponse(lifeErr)
	}

	var projErr *factoryvisualization.ProjectionError
	if errors.As(err, &projErr) && projErr != nil {
		return projectionErrorResponse(projErr)
	}

	var presErr *factoryvisualization.PresentationError
	if errors.As(err, &presErr) && presErr != nil {
		return presentationErrorResponse(presErr)
	}

	return 0, nil, false
}

func lifecycleErrorResponse(err *factoryvisualization.LifecycleError) (int, factoryapi.ErrorResponse, bool) {
	message := visualizationErrorMessage(err.Message, string(err.Kind))
	switch err.Kind {
	case factoryvisualization.LifecycleErrorMissingParameters:
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.LifecycleErrorMissingParameters),
		}, true
	case factoryvisualization.LifecycleErrorAlreadyActivated,
		factoryvisualization.LifecycleErrorNotActivated:
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode(err.Kind),
		}, true
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func projectionErrorResponse(err *factoryvisualization.ProjectionError) (int, factoryapi.ErrorResponse, bool) {
	message := visualizationErrorMessage(err.Message, string(err.Kind))
	switch err.Kind {
	case factoryvisualization.ProjectionErrorInvalidInput:
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.ProjectionErrorInvalidInput),
		}, true
	case factoryvisualization.ProjectionErrorSnapshotUnavailable:
		return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.ProjectionErrorSnapshotUnavailable),
		}, true
	case factoryvisualization.ProjectionErrorReconstructionFailed:
		return http.StatusInternalServerError, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.ProjectionErrorReconstructionFailed),
		}, true
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func presentationErrorResponse(err *factoryvisualization.PresentationError) (int, factoryapi.ErrorResponse, bool) {
	message := visualizationErrorMessage(err.Message, string(err.Kind))
	switch err.Kind {
	case factoryvisualization.PresentationErrorInvalidInput:
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.PresentationErrorInvalidInput),
		}, true
	case factoryvisualization.PresentationErrorEnqueueAfterClose,
		factoryvisualization.PresentationErrorFinalizeWithoutWriter:
		return http.StatusConflict, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode(err.Kind),
		}, true
	case factoryvisualization.PresentationErrorBackpressureRejected:
		return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
			Message: message,
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Code:    factoryapi.ErrorResponseCode(factoryvisualization.PresentationErrorBackpressureRejected),
		}, true
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func visualizationErrorMessage(message, fallback string) string {
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		return trimmed
	}
	return fallback
}

// WriteRequestError writes the stable request failure envelope used by all
// Visualization HTTP operation adapters.
func WriteRequestError(w http.ResponseWriter, logger *zap.Logger, err error, logMessage string) {
	if message, ok := common.RequestFieldValidationMessage(err); ok {
		common.WriteError(w, http.StatusBadRequest, message, "BAD_REQUEST", logger)
		return
	}
	if common.IsDecodeError(err) {
		common.WriteError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST", logger)
		return
	}
	if status, response, ok := RootErrorResponse(err); ok {
		if response == nil {
			return
		}
		common.WriteJSON(w, status, response, logger)
		return
	}
	if logger != nil {
		logger.Error(logMessage, zap.Error(err))
	}
	common.WriteError(
		w,
		http.StatusInternalServerError,
		visualizationRequestFailedMessage,
		string(factoryapi.ErrorResponseCodeINTERNALERROR),
		logger,
	)
}
