package chatsessions

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// FactoryTargetService is the narrow Factory-target execution dependency some
// L1 Chat Sessions flows need ahead of L3 Factory Sessions sealing: start one
// Factory Session, invoke it, cancel the captured turn, and close the
// session. Requests, results, and errors are the published factory_sessions
// root vocabulary, unmodified; peers do not import factory_sessions directly
// for these operations, and FactoryTargetService does not reinterpret
// results or reach into Factory Sessions internals. It is a separate root
// from Service (contracts.go), which is the L1 V0 contract slice's own
// transport-independent vocabulary and does not depend on Factory Sessions.
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
