package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const (
	catalogNotFoundMessage            = "model not found"
	catalogListFailedMessage          = "failed to list models"
	catalogGetFailedMessage           = "failed to load model"
	catalogErrorCodeModelNotAvailable = "MODEL_NOT_AVAILABLE"
)

// CatalogRootErrorResponse maps typed Models catalog root failures to HTTP
// status and the public ErrorResponse shape. It returns false when err is not a
// known mapped typed failure.
func CatalogRootErrorResponse(err error) (int, factoryapi.ErrorResponse, bool) {
	if err == nil {
		return 0, factoryapi.ErrorResponse{}, false
	}

	switch {
	case errors.Is(err, models.ErrNotFound):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: catalogNotFoundMessage,
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	case errors.Is(err, models.ErrUnavailable):
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: strings.TrimSpace(err.Error()),
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCode(catalogErrorCodeModelNotAvailable),
		}, true
	default:
		return 0, factoryapi.ErrorResponse{}, false
	}
}

func (h *Handler) writeCatalogError(w http.ResponseWriter, err error, fallbackMessage string) {
	if status, response, ok := CatalogRootErrorResponse(err); ok {
		h.writeJSON(w, status, response)
		return
	}
	h.logger.Error(fallbackMessage, zap.Error(err))
	h.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
}
