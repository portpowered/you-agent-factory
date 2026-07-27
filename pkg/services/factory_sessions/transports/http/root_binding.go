package http

import (
	"net/http"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
)

// SessionsRoot is the accepted Factory Sessions root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Sessions
// internal packages.
type SessionsRoot = factorysessions.Service

// RootBinding binds the HTTP adapter to one injected Sessions root.
type RootBinding struct {
	Sessions SessionsRoot
	Prepare  RequestPreparation
}

// NewHandlerFromRoot constructs an HTTP adapter that calls through the supplied
// Sessions root. Tests inject a focused fake implementing SessionsRoot without
// constructing durable runtime state or importing Sessions internals.
func NewHandlerFromRoot(binding RootBinding, logger *zap.Logger) *Adapter {
	if binding.Prepare == nil {
		binding.Prepare = noopRequestPreparation{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return NewHandler(Dependencies{
		SessionsRoot:    binding.Sessions,
		SessionRequests: binding.Prepare,
	}, logger)
}

type noopRequestPreparation struct{}

func (noopRequestPreparation) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	if request.Scope == "" {
		request.Scope = factorysessions.DefaultSessionListScope
	}
	return request, nil
}
func (noopRequestPreparation) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	return request, nil
}
func (noopRequestPreparation) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	return request, nil
}

func (s *Server) requireSessionsRoot(w http.ResponseWriter) (SessionsRoot, bool) {
	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionsRoot, true
}
