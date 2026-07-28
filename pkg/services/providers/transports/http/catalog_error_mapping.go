package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	catalogInvalidProviderIDMessage     = "invalid provider id"
	catalogProviderNotFoundMessage      = "provider not found"
	catalogListFailedMessage            = "failed to list providers"
	catalogGetFailedMessage             = "failed to get provider"
	catalogErrorCodeProviderUnavailable = "PROVIDER_UNAVAILABLE"
)

// CatalogRootErrorResponse maps typed Providers catalog root failures to HTTP
// status and the public ErrorResponse shape. It returns false when err is not a
// known mapped typed failure.
func CatalogRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch {
	case errors.Is(err, providers.ErrInvalidID):
		return badRequestErrorResponse(catalogInvalidProviderIDMessage)
	case errors.Is(err, providers.ErrUnknownProvider):
		return notFoundErrorResponse(catalogProviderNotFoundMessage)
	case errors.Is(err, providers.ErrProviderUnavailable):
		return providerUnavailableErrorResponse(strings.TrimSpace(err.Error()))
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (a *Adapter) writeCatalogError(w http.ResponseWriter, err error) bool {
	if status, response, ok := CatalogRootErrorResponse(err); ok {
		a.writeJSON(w, status, response)
		return true
	}
	return false
}

func (a *Adapter) writeCatalogOrInternalError(w http.ResponseWriter, err error, fallbackMessage string) {
	if a.writeCatalogError(w, err) {
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

func providerUnavailableErrorResponse(message string) (int, factoryapi.ErrorResponse, bool) {
	if message == "" {
		message = providers.ErrProviderUnavailable.Error()
	}
	return http.StatusNotFound, factoryapi.ErrorResponse{
		Message: message,
		Family:  factoryapi.ErrorFamilyNotFound,
		Code:    factoryapi.ErrorResponseCode(catalogErrorCodeProviderUnavailable),
	}, true
}
