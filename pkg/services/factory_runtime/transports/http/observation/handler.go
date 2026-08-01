// Package observation owns Factory Runtime HTTP observation operations.
package observation

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	common "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	transporterrors "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/errors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// SessionObserver routes session-scoped Runtime observation through the
// Factory Sessions peer surface without importing Sessions internals.
type SessionObserver interface {
	ObserveForSession(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
}

// Handler adapts Runtime observation operations while keeping their protocol
// mechanics separate from lifecycle, dispatch, and checkpoint handlers.
type Handler struct {
	root     factoryruntime.Service
	sessions SessionObserver
}

// NewHandler binds observation to the already-constructed Runtime root.
func NewHandler(root factoryruntime.Service) *Handler {
	if root == nil {
		return nil
	}
	return &Handler{root: root}
}

// BindSessionObserver attaches the peer used for session-scoped status reads.
func (h *Handler) BindSessionObserver(sessions SessionObserver) {
	if h != nil {
		h.sessions = sessions
	}
}

// GetStatus handles GET /status through the Runtime observation root.
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	h.getStatus(w, r, "")
}

// GetStatusBySessionId handles a status read scoped to one live Factory
// Session. The generated route keeps the public SessionID parameter here.
func (h *Handler) GetStatusBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	h.getStatus(w, r, string(sessionID))
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request, sessionID string) {
	if common.GuardRequestContext(w, r) {
		return
	}
	result, err := h.observeStatus(r.Context(), sessionID)
	if err != nil {
		transporterrors.WriteRootOrInternalError(
			w,
			r.Context(),
			transporterrors.OperationObserve,
			"failed to observe factory runtime status",
			err,
		)
		return
	}
	common.WriteJSON(w, http.StatusOK, statusResponseFromObservation(result.Observation))
}

func (h *Handler) observeStatus(ctx context.Context, sessionID string) (factoryruntime.ObserveResult, error) {
	var configuredRoot factoryruntime.Service
	if h != nil {
		configuredRoot = h.root
	}
	root, err := common.RequireRuntimeRoot(configuredRoot)
	if err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	request := factoryruntime.ObserveRequest{Scope: statusObserveScope}
	if sessionID == "" {
		return root.Observe(ctx, request)
	}
	if h == nil || h.sessions == nil {
		return factoryruntime.ObserveResult{}, transporterrors.ErrSessionObserverRequired
	}
	return h.sessions.ObserveForSession(ctx, sessionID, request)
}
