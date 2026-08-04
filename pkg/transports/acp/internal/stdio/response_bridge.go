package stdio

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// dispatchFactoryInvocation calls invoke -- one of the two
// factorysessions.Service.InvokeFactorySession-forwarding calls
// startFactorySessionForEpisode/invokeFactorySessionForEpisode make -- and
// returns its result and error unchanged. When s.responseBridge,
// s.chatSessions, and s.factoryTarget are all configured, it instead calls
// s.responseBridge with invoke: the injected collaborator (see
// acp.ResponseBridge's own doc comment) starts the Chat Sessions-owned
// Factory response-event bridge concurrently with invoke and still returns
// invoke's own result and error unchanged, streaming being purely additive.
// This method itself starts no goroutine and owns no concurrency primitive:
// it only ever holds and calls the one plain function value pkg/wire
// injected.
func (s *Server) dispatchFactoryInvocation(
	ctx context.Context,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error) {
	if s.responseBridge == nil || s.chatSessions == nil || s.factoryTarget == nil {
		return invoke(ctx)
	}
	return s.responseBridge(ctx, s.chatSessions, s.factoryTarget, chatSessionID, sessionVersion, factorySessionID, invoke)
}
