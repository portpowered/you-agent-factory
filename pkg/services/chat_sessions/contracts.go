package chatsessions

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Service is the singular Chat Sessions root contract. It currently exposes
// only the Factory-target execution capabilities the L1 Chat Sessions flows
// need: start one Factory Session, invoke it, cancel the captured turn, and
// close the session. Requests, results, and errors are the published
// factory_sessions root vocabulary, unmodified; peers do not import
// factory_sessions directly for these operations, and Service does not
// reinterpret results or reach into Factory Sessions internals.
type Service interface {
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
