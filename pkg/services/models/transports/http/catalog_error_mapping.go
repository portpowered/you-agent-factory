package http

import (
	"net/http"
	"strings"

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
	h.writeUnmappedRootError(w, operation, err, fallbackMessage)
}

func (h *Handler) writeUnmappedRootError(
	w http.ResponseWriter,
	operation modelsHTTPOperation,
	err error,
	fallbackMessage string,
) {
	rawMessage := strings.TrimSpace(err.Error())
	leaksInternalDetail := modelsErrorMessageLeaksInternalDetail(rawMessage)

	switch operation {
	case modelsHTTPOperationInvoke:
		if leaksInternalDetail {
			h.logger.Error(fallbackMessage, zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
			return
		}
		h.writeError(w, http.StatusBadRequest, rawMessage, "BAD_REQUEST")
	case modelsHTTPOperationPull:
		message := rawMessage
		if leaksInternalDetail {
			h.logger.Error(fallbackMessage, zap.Error(err))
			message = fallbackMessage
		}
		h.writeError(w, http.StatusInternalServerError, message, "INTERNAL_ERROR")
	default:
		h.logger.Error(fallbackMessage, zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, fallbackMessage, "INTERNAL_ERROR")
	}
}

func (h *Handler) writeCatalogError(w http.ResponseWriter, err error, fallbackMessage string) {
	h.writeRootOrInternalError(w, modelsHTTPOperationCatalog, err, fallbackMessage)
}
