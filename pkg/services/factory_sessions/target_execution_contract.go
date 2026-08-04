package factorysessions

import "context"

// TargetExecutionService is the narrow, owner-published Factory Sessions
// capability for asynchronous target start, captured-session invocation,
// captured-turn cancellation, and target close. It uses the exact request,
// result, and error vocabulary Service already publishes for these four
// operations (StartAsync, InvokeFactorySession, Cancel, CloseFactorySession),
// so Service itself satisfies this interface structurally -- see the var _
// assertion below. It is declared in this root package, not a wire or
// internal subpackage, so a peer service or transport that only needs to
// start, invoke, cancel, and close a selected target can depend directly on
// the owning service's published contract instead of a consumer-declared
// lookalike: docs/architecture/packaged-structure.md's service package
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
	CloseFactorySession(context.Context, string) error
	SubscribeFactoryResponseEvents(context.Context, ResponseEventSubscriptionRequest) (*ResponseEventCursor, error)
}

// Service satisfies TargetExecutionService structurally; this assertion
// keeps the two interfaces from silently drifting apart.
var _ TargetExecutionService = (Service)(nil)
