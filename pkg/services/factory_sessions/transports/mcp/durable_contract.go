package factorysession

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// DurableExecution is the Factory Sessions-owned MCP capability for lifecycle,
// start, result, and session reads. Canonical history inspection is deliberately
// absent: Recordings owns event, dispatch, and artifact reads at the transport
// boundary.
type DurableExecution interface {
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error)
	ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error)
	GetSession(context.Context, string) (factorysessions.SessionReadResult, error)
	Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error)
	RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error)
	InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error)
	GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

// LiveDispatchReader is the narrow compatibility capability used when a live
// Factory Session has no finalized Recordings artifact yet. It is asserted
// from the already-bound durable execution owner; no second execution graph is
// constructed by the MCP transport.
type LiveDispatchReader interface {
	ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error)
}
