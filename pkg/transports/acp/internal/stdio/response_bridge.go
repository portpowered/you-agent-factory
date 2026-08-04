package stdio

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// dispatchFactoryInvocation calls invoke -- one of the two
// factorysessions.Service.InvokeFactorySession-forwarding calls
// startFactorySessionForEpisode/invokeFactorySessionForEpisode make -- and
// returns its result, error, and liveDelivered=false unchanged. When
// s.responseBridge, s.chatSessions, and s.factoryTarget are all configured, it
// instead calls s.responseBridge with invoke and a liveDrain closure over
// s.liveDrainTurnUpdates: the injected collaborator (see acp.ResponseBridge's
// own doc comment) starts the Chat Sessions-owned Factory response-event
// bridge AND this transport's own genuine mid-generation consumer loop both
// concurrently with invoke, then drains the bridge's terminal retained tail
// before a successful prompt can return. An invocation error remains
// authoritative; a bridge failure after a successful invocation returns a
// bounded prompt failure. This method itself starts no goroutine and owns no
// concurrency primitive: it only ever holds and calls the one plain function
// value pkg/wire injected, and only ever supplies liveDrain as a plain
// callback for that collaborator to run.
//
// liveDelivered reports whether the liveDrain callback itself observed and
// delivered at least one canonical agent_message_chunk before invoke
// returned (see dispatchOutcome's own doc comment for why this must be
// threaded through to deliverPromptUpdates' V1 suppression decision). Writing
// to it from inside the liveDrain closure -- which s.responseBridge runs on a
// goroutine it owns, never one this method spawns itself -- is race-free
// without its own synchronization: s.responseBridge only returns once that
// goroutine has fully joined (see responsebridge.Service.Run's own doc
// comment), and that join's happens-before guarantee is what makes reading
// liveDelivered after s.responseBridge returns safe.
//
// connectionID is threaded through from dispatchFactoryTurn's own
// reqIdentity so liveDrainTurnUpdates can call ensureAttachment exactly the
// way the post-invocation streamTurnUpdates call already does; notify is read
// from ctx (promptNotifierFromContext), matching deliverPromptUpdates' own
// convention, since liveDrain runs against the bridge-derived context the
// response bridge passes it, not necessarily this ctx directly.
func (s *Server) dispatchFactoryInvocation(
	ctx context.Context,
	connectionID, chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (result factorysessions.InvocationResult, liveDelivered bool, err error) {
	if s.responseBridge == nil || s.chatSessions == nil || s.factoryTarget == nil {
		result, err = invoke(ctx)
		return result, false, err
	}
	notify := promptNotifierFromContext(ctx)
	liveDrain := func(drainCtx context.Context) {
		liveDelivered = s.liveDrainTurnUpdates(drainCtx, connectionID, chatSessionID, sessionVersion, notify)
	}
	result, err = s.responseBridge(ctx, chatSessionID, sessionVersion, factorySessionID, liveDrain, invoke)
	return result, liveDelivered, err
}
