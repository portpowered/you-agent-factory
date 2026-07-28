package http

import (
	"net/http"

	"go.uber.org/zap"
)

func (h *Handler) writeRootError(w http.ResponseWriter, operation modelsHTTPOperation, err error) bool {
	if status, response, ok := RootErrorResponse(err, operation); ok {
		h.writeJSON(w, status, response)
		return true
	}
	return false
}

func (h *Handler) writeRootOrInternalError(
	w http.ResponseWriter,
	operation modelsHTTPOperation,
	err error,
	fallbackMessage string,
) {
	if h.writeModelsRequestContextOutcome(w, err) {
		return
	}
	if h.writeRootError(w, operation, err) {
		return
	}
	h.logger.Error(fallbackMessage, zap.Error(err))
	h.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
}

func (h *Handler) writeCatalogError(w http.ResponseWriter, err error, fallbackMessage string) {
	h.writeRootOrInternalError(w, modelsHTTPOperationCatalog, err, fallbackMessage)
}
