package http

import (
	"context"
	"errors"
	"net/http"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func (h *Handler) writeProviderSessionError(
	w http.ResponseWriter,
	params factoryapi.GetProviderSessionDetailsParams,
	err error,
) {
	var lookupErr *providersessions.LookupError
	switch {
	case errors.Is(err, providersessions.ErrOperationCanceled),
		errors.Is(err, context.Canceled):
		h.writeError(w, http.StatusInternalServerError, "provider session inspection canceled", "INTERNAL_ERROR")
	case errors.Is(err, context.DeadlineExceeded):
		h.writeError(w, http.StatusInternalServerError, "provider session inspection timed out", "INTERNAL_ERROR")
	case errors.Is(err, providersessions.ErrUnsupportedProvider),
		errors.Is(err, providersessions.ErrUnsupportedKind):
		h.writeError(w, http.StatusBadRequest, "invalid request parameter", "BAD_REQUEST")
	case errors.Is(err, providersessions.ErrInvalidIdentifier):
		_ = errors.As(err, &lookupErr)
		h.writeError(w, http.StatusBadRequest, invalidProviderSessionIdentifierMessage(lookupErr), "BAD_REQUEST")
	case errors.Is(err, providersessions.ErrSessionNotFound):
		if errors.As(err, &lookupErr) && lookupErr.Provider == providersessions.ProviderCursor {
			h.logCursorProviderSessionLookupNotFound(params.Kind, string(params.Id), lookupErr.Root)
		}
		h.writeError(w, http.StatusNotFound, "provider session not found", "NOT_FOUND")
	case errors.Is(err, providersessions.ErrAmbiguousSessionFile):
		h.writeError(w, http.StatusInternalServerError, "multiple provider session files match session identifier", "INTERNAL_ERROR")
	default:
		h.logger.Error("load provider session details failed", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to load provider session details", "INTERNAL_ERROR")
	}
}

func invalidProviderSessionIdentifierMessage(lookupErr *providersessions.LookupError) string {
	if lookupErr != nil && lookupErr.Provider == providersessions.ProviderCursor {
		return "provider session must be a cursor session_id identifier without path separators"
	}
	return "provider session must be a codex session_id identifier without path separators"
}

func (h *Handler) logCursorProviderSessionLookupNotFound(
	kind factoryapi.LoadableProviderSessionKind,
	requestedID string,
	root string,
) {
	fields := []zap.Field{
		zap.String("provider", string(providersessions.ProviderCursor)),
		zap.String("lookup_kind", string(kind)),
		zap.String("requested_id", requestedID),
	}
	if root == "" {
		fields = append(fields, zap.Bool("root_configured", false))
	} else {
		fields = append(fields,
			zap.Bool("root_configured", true),
			zap.String("searched_root", root),
		)
	}
	h.logger.Info("cursor provider session lookup not found", fields...)
}
