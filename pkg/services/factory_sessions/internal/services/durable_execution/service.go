package durableexecution

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Service owns durable Factory Session start, lifecycle, inspection, replay,
// and restart behavior behind the Factory Sessions private capability boundary.
type Service interface {
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

// CanonicalStartResult is the private durable projection returned by the
// mode-neutral Factory Sessions start seam. It keeps durable implementation
// choice behind the owner without adding a second public result vocabulary.
type CanonicalStartResult struct {
	Async *factorysessions.AsyncStartResult
	Sync  *factorysessions.SyncStartResult
}

// CanonicalService is the private durable start seam used by canonical
// Factory Sessions operations. Compatibility StartAsync/StartSync methods are
// intentionally not selected by the canonical owner.
type CanonicalService interface {
	StartCanonical(context.Context, factorysessions.StartRequest, bool) (CanonicalStartResult, error)
}

// CanonicalReadService is the private durable read seam used by the
// mode-neutral Factory Sessions owner. It keeps compatibility-shaped durable
// methods behind the durable owner while allowing the public root to map
// Session* projections directly.
type CanonicalReadService interface {
	GetCanonical(context.Context, string) (factorysessions.SessionReadResult, error)
	ListCanonical(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

// CanonicalControlResult carries either a durable lifecycle control result or
// the async projection returned by restart recovery.
type CanonicalControlResult struct {
	Lifecycle *factorysessions.LifecycleControlResult
	Recovery  *factorysessions.AsyncStartResult
}

// CanonicalControlService is the private durable control seam used by the
// mode-neutral Factory Sessions owner.
type CanonicalControlService interface {
	ControlCanonical(context.Context, factorysessions.SessionControlRequest) (CanonicalControlResult, error)
}

// CanonicalResultService is the private durable result-read seam used by the
// mode-neutral Factory Sessions owner.
type CanonicalResultService interface {
	ReadResultCanonical(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
}

// CanonicalDispatchService is the private durable dispatch-query seam used by
// the mode-neutral Factory Sessions owner.
type CanonicalDispatchService interface {
	QueryDispatchesCanonical(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
}

// CanonicalResponseService is the private durable response-event subscription
// seam used by the mode-neutral Factory Sessions owner.
type CanonicalResponseService interface {
	SubscribeResponsesCanonical(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
}
