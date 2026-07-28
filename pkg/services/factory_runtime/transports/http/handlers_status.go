package http

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// GetStatus handles GET /status through the accepted Runtime observation slice.
func (a *Adapter) GetStatus(w http.ResponseWriter, r *http.Request) {
	a.getStatus(w, r, "")
}

// GetStatusBySessionId handles GET /factory-sessions/{session_id}/status through
// the accepted Runtime observation slice for one live session.
func (a *Adapter) GetStatusBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	a.getStatus(w, r, string(sessionID))
}

func (a *Adapter) getStatus(w http.ResponseWriter, r *http.Request, sessionID string) {
	if a.guardRuntimeRequestContext(w, r) {
		return
	}
	result, err := a.observeStatus(r.Context(), sessionID)
	if err != nil {
		a.writeRootOrInternalError(w, r.Context(), runtimeHTTPOperationObserve, "failed to observe factory runtime status", err)
		return
	}
	a.writeJSON(w, http.StatusOK, statusResponseFromObservation(result.Observation))
}

func (a *Adapter) observeStatus(ctx context.Context, sessionID string) (factoryruntime.ObserveResult, error) {
	root, err := a.runtimeRoot()
	if err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	request := factoryruntime.ObserveRequest{Scope: statusObserveScope}
	if sessionID == "" {
		return root.Observe(ctx, request)
	}
	if a == nil || a.sessions == nil {
		return factoryruntime.ObserveResult{}, errSessionObserverRequired
	}
	return a.sessions.ObserveForSession(ctx, sessionID, request)
}
