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
// concrete collaborator and injects it as this contract. Its shape matches
// chat_sessions/internal/factorysessionsshim.FactoryTargetService exactly
// (StartFactoryTarget forwards to the shared factorysessions.Service.StartAsync,
// InvokeFactoryTarget to InvokeFactorySession, and so on), because the
// concrete collaborator pkg/wire injects is that existing, consumer-owned
// shim -- see pkg/wire's provideACPServerFactoryTargetService.
//
// StartFactoryTarget returns the shared, genuinely asynchronous
// factorysessions.AsyncStartResult: opening a Factory target runtime is not
// itself dispatching content, so it carries only the opened identity and a
// non-terminal status, never fabricated text or a terminal outcome. A
// caller that needs the first turn's actual published outcome makes an
// immediate, separate InvokeFactoryTarget call against the returned
// identity -- the exact same operation a later turn's own invoke uses.
type FactoryTargetService interface {
	StartFactoryTarget(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	InvokeFactoryTarget(
		context.Context,
		string,
		factorysessions.InvocationRequest,
	) (factorysessions.InvocationResult, error)
	CancelFactoryTarget(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error)
	CloseFactoryTarget(context.Context, string) error
}
