package http

import (
	"errors"
	"net/http"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const visualizationRequestFailedMessage = "factory visualization request failed"

// visualizationRootErrorResponse maps typed Visualization root failures to HTTP
// status and a public ErrorResponse body.
func visualizationRootErrorResponse(err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	if status, response, ok := visualizationRequestContextErrorResponse(err); ok {
		return status, response, true
	}

	var lifeErr *factoryvisualization.LifecycleError
	if errors.As(err, &lifeErr) {
		return lifecycleErrorResponse(lifeErr)
	}

	var projErr *factoryvisualization.ProjectionError
	if errors.As(err, &projErr) {
		return projectionErrorResponse(projErr)
	}

	var presErr *factoryvisualization.PresentationError
	if errors.As(err, &presErr) {
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

func (a *Adapter) writeVisualizationRootError(w http.ResponseWriter, err error) bool {
	if status, response, ok := visualizationRootErrorResponse(err); ok {
		if response == nil {
			return true
		}
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeVisualizationRequestError(w http.ResponseWriter, err error, logMessage string) {
	if message, ok := requestFieldValidationMessage(err); ok {
		a.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
		return
	}
	if isVisualizationHTTPDecodeError(err) {
		a.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if a.writeVisualizationRootError(w, err) {
		return
	}
	a.logger.Error(logMessage, zap.Error(err))
	a.writeError(
		w,
		http.StatusInternalServerError,
		visualizationRequestFailedMessage,
		string(factoryapi.ErrorResponseCodeINTERNALERROR),
	)
}

func isVisualizationHTTPDecodeError(err error) bool {
	var (
		lifecycleDecodeErr    lifecycleHTTPDecodeError
		observeDecodeErr      observeHTTPDecodeError
		presentationDecodeErr presentationHTTPDecodeError
	)
	return errors.As(err, &lifecycleDecodeErr) ||
		errors.As(err, &observeDecodeErr) ||
		errors.As(err, &presentationDecodeErr)
}
