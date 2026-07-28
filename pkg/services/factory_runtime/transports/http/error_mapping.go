package http

import (
	"errors"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

var errSessionObserverRequired = errors.New("factory session observation is required for session-scoped status reads")

func observeErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}
	switch {
	case errors.Is(err, errSessionObserverRequired):
		return serviceUnavailableErrorResponse("factory status is unavailable")
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return notFoundErrorResponse("factory session not found")
	case errors.Is(err, factoryruntime.ErrInvalidObservationScope):
		return badRequestErrorResponse("invalid observation scope")
	case errors.Is(err, factoryruntime.ErrNotFound):
		return notFoundErrorResponse("factory runtime target not found")
	case errors.Is(err, factoryruntime.ErrNotRunning):
		return serviceUnavailableErrorResponse("factory runtime is not running")
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (a *Adapter) writeObserveError(w http.ResponseWriter, err error) bool {
	if status, response, ok := observeErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
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

func serviceUnavailableErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	return http.StatusServiceUnavailable, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyInternalServerError,
		Code:    factoryapi.ErrorResponseCode("SERVICE_UNAVAILABLE"),
	}, true
}
