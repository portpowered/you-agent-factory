package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// TestHandlerUnavailableBranchesStayOwnedBySessions exercises the retained
// session/current-factory/event routes without constructing a runtime graph.
func TestHandlerUnavailableBranchesStayOwnedBySessions(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{}, zap.NewNop())
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	var sessionID factoryapi.SessionID = "session-alpha"
	var dispatchID factoryapi.DispatchID = "dispatch-alpha"
	var artifactID factoryapi.ArtifactID = "artifact-alpha"

	tests := []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, r *http.Request) { handler.ValidateFactory(w, r) },
		func(w http.ResponseWriter, r *http.Request) { handler.PreviewFactory(w, r) },
		func(w http.ResponseWriter, r *http.Request) { handler.GetFactorySessionResult(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) { handler.GetFactorySessionPartialResult(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionDispatch(w, r, sessionID, dispatchID)
		},
		func(w http.ResponseWriter, r *http.Request) { handler.ListFactorySessionArtifacts(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionArtifact(w, r, sessionID, artifactID)
		},
		func(w http.ResponseWriter, r *http.Request) { handler.GetCurrentFactoryBySessionId(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetCurrentFactoryWorkstationPromptTemplateContractBySessionId(w, r, sessionID, "worker")
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ValidateCurrentFactoryWorkstationPromptTemplateBySessionId(w, r, sessionID, "worker")
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.InvokeFactorySessionBySessionId(w, r, sessionID)
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionResults(w, r, sessionID, factoryapi.GetFactorySessionResultsParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ListFactorySessionDispatches(w, r, sessionID, factoryapi.ListFactorySessionDispatchesParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetEventsBySessionId(w, r, sessionID, factoryapi.GetEventsBySessionIdParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactoryResponseEventsBySessionId(w, r, sessionID, factoryapi.GetFactoryResponseEventsBySessionIdParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionSyncPreflightBySessionId(w, r, sessionID, factoryapi.GetFactorySessionSyncPreflightBySessionIdParams{})
		},
	}
	for _, call := range tests {
		call(httptest.NewRecorder(), request)
	}
	unsupported := httptest.NewRequest(http.MethodPost, "/", nil)
	unsupported.Header.Set("Content-Type", "text/plain")
	handler.OpenFactorySession(httptest.NewRecorder(), unsupported)
}
