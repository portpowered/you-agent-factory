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
// concrete collaborator (the chat_sessions-owned Factory Sessions shim) and
// injects it as this contract.
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
