package acp

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ResponseBridge starts subscribing to one Factory Session's response-event
// stream through Factory Sessions' owner-published target-execution
// capability. Its injected implementation sequences observed events onto the
// canonical Chat Session aggregate concurrently with invoke, then drains the
// retained terminal tail before a successful prompt can return. An invocation
// error remains authoritative; a bridge failure after a successful invocation
// is returned so the prompt fails safely rather than omitting canonical output.
// It also runs liveDrain concurrently with the same invoke call and joins it
// the same way: liveDrain is this transport's own genuine mid-generation
// consumer loop (see
// internal/stdio.Server.liveDrainTurnUpdates), a plain function value with no
// concurrency primitive of its own, so this package still never holds a raw
// goroutine/channel/cancel -- it only ever supplies a callback for the
// injected collaborator to run.
//
// pkg/wire constructs the concrete collaborator and injects it. The concrete
// implementation is owned by Chat Sessions, not this transport: this package never implements
// the Factory-response-event translation, drain loop, or the concurrency
// that runs it alongside invoke -- it only ever holds and calls this one
// plain function value (see dispatchFactoryTurn's two Factory dispatch
// branches in internal/stdio/session_prompt.go).
type ResponseBridge func(
	ctx context.Context,
	subscriber factorysessions.TargetExecutionService,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error)
