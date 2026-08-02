package factorysession

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// DurableExecution is the mapping adapter's narrow durable capability. The
// opened value comes from the Factory Sessions root; this type belongs to the
// representation boundary rather than the Sessions service root.
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
	ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error)
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error)
	ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error)
	GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error)
	ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}
