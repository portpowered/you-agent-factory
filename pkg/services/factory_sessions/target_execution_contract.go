package factorysessions

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// TargetExecutionService is the narrow, owner-published Factory Sessions
// capability for asynchronous target start, captured-session invocation,
// captured-turn cancellation and termination, and target close. StartAsync,
// InvokeFactorySession, Cancel, and CloseFactorySession use the exact request,
// result, and error vocabulary Service already publishes. The target-specific
// TerminateFactorySession operation adds a captured ControlRequest because
// generic Factory Session cleanup has no Chat-turn ownership to forward.
// It is declared in this root package, not a wire or internal subpackage, so
// a peer service or transport that needs to start, invoke, control, and close
// a selected target can depend directly on the owning service's published
// contract instead of a consumer-declared lookalike. The service package
// convention states root files expose the contracts peer services and
// transports are allowed to consume, and explicitly notes the convention is
// "not a demand that every service root have the same number of interfaces,
// files, or subservices" as any other root.
//
// A peer that only needs this narrower interface can never reach an
// unrelated Service operation through it.
type TargetExecutionService interface {
	StartAsync(context.Context, StartRequest) (AsyncStartResult, error)
	InvokeFactorySession(context.Context, string, InvocationRequest) (InvocationResult, error)
	Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	// TerminateFactorySession terminates the captured target runtime. A
	// committed Chat control supplies its immutable turn and request identities
	// so Factory Runtime can fan termination to the associated Worker Sessions
	// before target cleanup begins.
	TerminateFactorySession(context.Context, string, ControlRequest) error
	CloseFactorySession(context.Context, string) error
	SubscribeFactoryResponseEvents(context.Context, ResponseEventSubscriptionRequest) (*ResponseEventCursor, error)
	// SubscribeFactoryEventsForSession exposes the canonical Factory Event
	// stream for one target-execution session. The stream includes the
	// dispatch-to-Worker-Session association records that downstream Chat
	// projection uses to establish child ownership before reading a Worker
	// Session topic.
	SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
}
