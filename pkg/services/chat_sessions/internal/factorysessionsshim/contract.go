package factorysessionsshim

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// FactoryTargetService is the narrow Factory-target execution dependency some
// L1 Chat Sessions flows need ahead of L3 Factory Sessions sealing: start one
// Factory Session, invoke it, cancel the captured turn, and close the
// session. Requests, results, and errors are the published factory_sessions
// root vocabulary, unmodified; callers do not import factory_sessions
// directly for these operations, and FactoryTargetService does not
// reinterpret results or reach into Factory Sessions internals. It is owned
// by this internal shim package, not the chat_sessions public root, so the
// L1 V0 chatsessions.Service contract slice stays free of a Factory Sessions
// dependency and of a second public peer-facing interface.
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

// FactoryTargetExecutionService is the narrow subset of the public
// factorysessions.Service root this shim actually forwards to: start, invoke,
// cancel, and close. Shim depends on this interface rather than the full
// 30+-method Service so a caller-owned Factory Sessions execution authority
// (for example an on-demand, per-target activation with no fixed pre-opened
// project runtime to serve every other Service operation) can be a complete,
// non-panicking implementation of exactly what this shim needs, instead of
// having to embed the full Service as a permanently-nil value and panic on
// every method beyond these four just to satisfy a parameter type it never
// fully uses. The CLI daemon's own full factorysessions.Service singleton
// still satisfies this interface unmodified, since Go interface satisfaction
// is structural.
type FactoryTargetExecutionService interface {
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error)
	Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
}
