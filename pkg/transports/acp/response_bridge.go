package acp

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ResponseBridge starts subscribing to one Factory Session's response-event
// stream (through subscriber, a FactoryTargetService) and sequencing every
// event it observes onto one Chat Session's aggregate stream (through
// chatSessions), concurrently with invoke, and returns invoke's own result
// and error unchanged once invoke itself returns. It also runs liveDrain
// concurrently with the same invoke call and joins it the same way: liveDrain
// is this transport's own genuine mid-generation consumer loop (see
// internal/stdio.Server.liveDrainTurnUpdates), a plain function value with no
// concurrency primitive of its own, so this package still never holds a raw
// goroutine/channel/cancel -- it only ever supplies a callback for the
// injected collaborator to run.
//
// It is declared here, in this transport's own public root (matching
// FactoryTargetService's own declared-locally convention), rather than
// imported from another service's internal or wire subpackage, so this
// package states the exact shape it consumes; pkg/wire constructs the
// concrete collaborator and injects it. The concrete implementation --
// chat_sessions/internal/factorysessionsshim.RunWithResponseBridge -- is
// owned by Chat Sessions, not this transport: this package never implements
// the Factory-response-event translation, drain loop, or the concurrency
// that runs it alongside invoke -- it only ever holds and calls this one
// plain function value (see dispatchFactoryTurn's two Factory dispatch
// branches in internal/stdio/session_prompt.go).
type ResponseBridge func(
	ctx context.Context,
	chatSessions chatsessions.Service,
	subscriber FactoryTargetService,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error)
