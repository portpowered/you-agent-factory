package factorysessionsshim

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Shim adapts a FactoryTargetExecutionService (the narrow start/invoke/
// cancel/close subset of the public factory_sessions.Service root this shim
// actually calls) to this package's own FactoryTargetService contract. It is
// stateless: it holds only the injected service and forwards each call
// exactly once, with the original context, identifiers, request, result, and
// error intact.
type Shim struct {
	service FactoryTargetExecutionService
}

var _ FactoryTargetService = (*Shim)(nil)

// New constructs the Factory Sessions shim over the given execution service.
// Any concrete factorysessions.Service (including the CLI daemon's full
// singleton) already satisfies FactoryTargetExecutionService structurally.
func New(service FactoryTargetExecutionService) *Shim {
	return &Shim{service: service}
}

// StartFactoryTarget delegates exactly once to the existing asynchronous
// Factory Sessions start operation.
func (shim *Shim) StartFactoryTarget(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	return shim.service.StartAsync(ctx, request)
}

// InvokeFactoryTarget delegates exactly once to the existing Factory Session
// invocation operation.
func (shim *Shim) InvokeFactoryTarget(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	return shim.service.InvokeFactorySession(ctx, sessionID, request)
}

// CancelFactoryTarget delegates exactly once to the existing public Factory
// Sessions cancel operation.
func (shim *Shim) CancelFactoryTarget(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return shim.service.Cancel(ctx, sessionID, request)
}

// CloseFactoryTarget delegates exactly once to the existing public Factory
// Session close operation.
func (shim *Shim) CloseFactoryTarget(ctx context.Context, sessionID string) error {
	return shim.service.CloseFactorySession(ctx, sessionID)
}

// SubscribeFactoryResponseEvents delegates exactly once to the existing
// Factory Sessions response-event subscription operation.
func (shim *Shim) SubscribeFactoryResponseEvents(
	ctx context.Context,
	req factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	return shim.service.SubscribeFactoryResponseEvents(ctx, req)
}
