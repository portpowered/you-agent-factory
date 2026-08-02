package factorysessionsshim

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Shim adapts the public factory_sessions.Service root to the Chat Sessions
// chatsessions.FactoryTargetService contract. It is stateless: it holds only
// the injected Service and forwards each call exactly once, with the
// original context, identifiers, request, result, and error intact.
type Shim struct {
	service factorysessions.Service
}

var _ chatsessions.FactoryTargetService = (*Shim)(nil)

// New constructs the Factory Sessions shim over the given public Service.
func New(service factorysessions.Service) *Shim {
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
