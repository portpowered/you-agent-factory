package http

import (
	"errors"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	automationsInvalidRequestMessage      = "invalid automations request"
	automationsNotFoundMessage            = "automations resource not found"
	automationsConflictMessage            = "automations operation conflicted with observed state"
	automationsNotReadyMessage            = "automations service is not ready"
	automationsSupervisionFailedMessage   = "automation source supervision failed"
	automationsInternalFailureMessage     = "automations request failed"
	automationsErrorCodeConflict          = "CONFLICT"
	automationsErrorCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// RootErrorResponse maps typed Automations root failures and adapter decode
// validation failures to HTTP status and the public ErrorResponse shape. It
// returns false when err is not a known mapped typed failure.
func RootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	if status, response, ok := automationsRequestContextErrorResponse(err); ok {
		if response == nil {
			return 0, factoryapi.ErrorResponse{}, true
		}
		if errResp, ok := response.(factoryapi.ErrorResponse); ok {
			return status, errResp, true
		}
	}

	if IsLifecycleBadRequest(err) || IsConvergenceBadRequest(err) {
		return badRequestErrorResponse(automationsInvalidRequestMessage)
	}

	switch {
	case errors.Is(err, automations.ErrInvalidRequest):
		return badRequestErrorResponse(automationsInvalidRequestMessage)
	case errors.Is(err, automations.ErrNotFound):
		return notFoundErrorResponse(automationsNotFoundMessage)
	case errors.Is(err, automations.ErrConflict):
		return conflictErrorResponse(automationsConflictMessage)
	case errors.Is(err, automations.ErrNotReady):
		return serviceUnavailableErrorResponse(automationsNotReadyMessage)
	case errors.Is(err, automations.ErrSupervisionFailed):
		return internalErrorResponse(automationsSupervisionFailedMessage)
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (a *Adapter) writeRootError(w http.ResponseWriter, err error) bool {
	if a.writeAutomationsRequestContextOutcome(w, err) {
		return true
	}
	if status, response, ok := RootErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeRootOrInternalError(w http.ResponseWriter, err error) {
	if a.writeRootError(w, err) {
		return
	}
	a.writeError(
		w,
		http.StatusInternalServerError,
		automationsInternalFailureMessage,
		string(factoryapi.ErrorResponseCodeINTERNALERROR),
	)
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

func conflictErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    factoryapi.ErrorResponseCode(automationsErrorCodeConflict),
	}, true
}

func serviceUnavailableErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode(automationsErrorCodeServiceUnavailable),
	}, true
}

func internalErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusInternalServerError, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCodeINTERNALERROR,
	}, true
}
