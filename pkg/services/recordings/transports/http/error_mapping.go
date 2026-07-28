package http

import (
	"errors"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type recordingsHTTPOperation int

const (
	recordingsHTTPOperationEventSubscribe recordingsHTTPOperation = iota
	recordingsHTTPOperationArtifactRead
)

// RootErrorResponse maps typed Recordings root failures to HTTP status and the
// public ErrorResponse shape. It returns false when err is not a known mapped
// typed failure.
func RootErrorResponse(err error, operation recordingsHTTPOperation) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch operation {
	case recordingsHTTPOperationEventSubscribe:
		if isEventReconnectValidationError(err) {
			return badRequestErrorResponse("invalid event reconnect cursor")
		}
	case recordingsHTTPOperationArtifactRead:
		if isArtifactReadValidationError(err) {
			return badRequestErrorResponse("invalid artifact read request")
		}
		if isRecordingMissingTargetError(err) {
			return notFoundErrorResponse("factory session artifact not found")
		}
	}

	return 0, factoryapi.ErrorResponse{}, false
}

func (a *Adapter) writeRootError(
	w http.ResponseWriter,
	operation recordingsHTTPOperation,
	err error,
) bool {
	if status, response, ok := RootErrorResponse(err, operation); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeRootOrInternalError(
	w http.ResponseWriter,
	operation recordingsHTTPOperation,
	err error,
) {
	if a.writeRootError(w, operation, err) {
		return
	}
	a.writeError(
		w,
		http.StatusInternalServerError,
		internalErrorMessage(operation),
		"INTERNAL_ERROR",
	)
}

func isEventReconnectValidationError(err error) bool {
	return errors.Is(err, recordings.ErrInvalidSubscribeScope) ||
		errors.Is(err, recordings.ErrInvalidReconnectCursor) ||
		errors.Is(err, recordings.ErrReconnectCursorNotFound) ||
		errors.Is(err, recordings.ErrReconnectCursorExpired) ||
		errors.Is(err, recordings.ErrReconnectCursorUnavailable)
}

func isArtifactReadValidationError(err error) bool {
	return errors.Is(err, errInvalidArtifactReadScope) ||
		errors.Is(err, errInvalidArtifactReadID) ||
		errors.Is(err, recordings.ErrInvalidRecordingScope) ||
		errors.Is(err, recordings.ErrInvalidProjectionInput) ||
		errors.Is(err, recordings.ErrInvalidProjectionScope) ||
		errors.Is(err, recordings.ErrMalformedProjectionOrder)
}

func isRecordingMissingTargetError(err error) bool {
	return errors.Is(err, recordings.ErrMissingRecordingTarget) ||
		errors.Is(err, recordings.ErrPortableArtifactUnavailable) ||
		errors.Is(err, recordings.ErrForeignPortableArtifact)
}

func badRequestErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusBadRequest, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Code:    factoryapi.ErrorResponseCodeBADREQUEST,
	}, true
}

func notFoundErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
	}, true
}

func internalErrorMessage(operation recordingsHTTPOperation) string {
	switch operation {
	case recordingsHTTPOperationEventSubscribe:
		return "failed to subscribe to factory events"
	default:
		return "failed to read factory session artifacts"
	}
}
