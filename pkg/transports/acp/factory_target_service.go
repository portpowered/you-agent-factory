package acp

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// FactoryTargetService is the narrow Factory Session start/invoke/cancel/
// close dependency the production ACP prompt-delegation consumer needs. It
// is declared here, in this transport's own public root, rather than
// imported from another service's internal or wire subpackage, so this
// package states the exact shape it consumes; pkg/wire constructs the
// concrete collaborator and injects it as this contract. Its shape is an
// exact structural match for factory_sessions/wire.TargetExecutionService,
// the Factory Sessions-owned target-execution capability, because the
// concrete collaborator pkg/wire injects is that capability's own on-demand
// activation directly -- see pkg/wire's provideACPServerFactoryTargetService.
//
// StartAsync returns the shared, genuinely asynchronous
// factorysessions.AsyncStartResult: opening a Factory target runtime is not
// itself dispatching content, so it carries only the opened identity and a
// non-terminal status, never fabricated text or a terminal outcome. A
// caller that needs the first turn's actual published outcome makes an
// immediate, separate InvokeFactorySession call against the returned
// identity -- the exact same operation a later turn's own invoke uses.
type FactoryTargetService interface {
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	InvokeFactorySession(
		context.Context,
		string,
		factorysessions.InvocationRequest,
	) (factorysessions.InvocationResult, error)
	Cancel(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
}
