package http

import (
	"errors"
	"net/http"
	"strings"

	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	workErrorCodeConflict = "CONFLICT"
)

// RootErrorResponse maps typed Work root failures to HTTP status and the public
// ErrorResponse shape. It returns false when err is not a known mapped typed
// failure.
func RootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch {
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return notFoundErrorResponse("factory session not found")
	case errors.Is(err, work.ErrWorkNotFound), errors.Is(err, state.ErrMoveWorkNotFound):
		return notFoundErrorResponse("work not found")
	case errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied):
		return conflictErrorResponse(
			"Operator move request was already applied.",
			factoryapi.ErrorResponseCodeMOVEWORKREQUESTALREADYAPPLIED,
		)
	case errors.Is(err, state.ErrMoveWorkInvalidState):
		return badRequestErrorResponse("invalid target state for work type")
	case errors.Is(err, state.ErrMoveWorkInFlightDispatch):
		return badRequestErrorResponse("work is in an active dispatch")
	case errors.Is(err, state.ErrMoveWorkEngineTerminated):
		return badRequestErrorResponse("engine has terminated")
	case errors.Is(err, work.ErrWorkRequestConflict):
		return conflictErrorResponse("Work Request admission conflict", workErrorCodeConflict)
	case errors.Is(err, work.ErrWorkRequestRejected):
		return badRequestErrorResponse("Work Request rejected")
	case errors.Is(err, work.ErrInvalidWorkRequest):
		return badRequestErrorResponse("invalid Work Request")
	}

	var stagingErr *work.ContentStagingError
	if errors.As(err, &stagingErr) {
		return badRequestErrorResponse(stagingErr.Message)
	}

	var validation *work.ValidationError
	if errors.As(err, &validation) {
		return badRequestErrorResponse(validation.Message)
	}

	if message, ok := workRequestBadRequestMessage(err); ok {
		return badRequestErrorResponse(message)
	}

	return 0, factoryapi.ErrorResponse{}, false
}

func (a *Adapter) writeRootError(w http.ResponseWriter, err error) bool {
	if status, response, ok := RootErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeRootOrInternalError(
	w http.ResponseWriter,
	err error,
	fallbackMessage string,
) {
	if a.writeRootError(w, err) {
		return
	}
	a.writeError(
		w,
		http.StatusInternalServerError,
		fallbackMessage,
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

func conflictErrorResponse(message string, code factoryapi.ErrorResponseCode) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusConflict, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyConflict,
		Code:    code,
	}, true
}

func workRequestBadRequestMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := err.Error()
	if strings.HasPrefix(message, "work_request:") {
		return workRequestTypeNameMessage(message), true
	}
	if strings.Contains(message, "unknown work type") ||
		strings.Contains(message, "work type") && strings.Contains(message, "not found") {
		return workRequestTypeNameMessage(message), true
	}
	return "", false
}

func workRequestTypeNameMessage(message string) string {
	message = strings.ReplaceAll(message, "work_type_name", "workTypeName")
	message = strings.ReplaceAll(message, "work_type_id", "workTypeName")
	if strings.Contains(message, "work type name") {
		return message
	}
	return strings.ReplaceAll(message, "work type", "work type name")
}
