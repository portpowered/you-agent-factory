package recordingreplay

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

func (s *Scope) Inspection() factorysessions.HistoricalReplayInspection {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Inspection(s)
}

func (s *Scope) IsNonLiveReplay() bool {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.IsNonLiveReplay(s)
}

func (s *Scope) StartAsync(ctx context.Context, request fse.StartRequest) (fse.AsyncStartResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.StartAsync(s, ctx, request)
}

func (s *Scope) StartSync(ctx context.Context, request fse.StartRequest) (fse.SyncStartResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.StartSync(s, ctx, request)
}

func (s *Scope) ResumeInterruptedSession(
	ctx context.Context,
	sessionID string,
	request fse.ResumeSessionRequest,
) (fse.AsyncStartResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.ResumeInterruptedSession(s, ctx, sessionID, request)
}

func (s *Scope) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.SubscribeResponseEvents(s, ctx, sessionID, request)
}

func (s *Scope) GetSession(ctx context.Context, id string) (fse.SessionReadResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.GetSession(s, ctx, id)
}

func (s *Scope) Pause(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Pause(s, ctx, id, request)
}

func (s *Scope) Resume(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Resume(s, ctx, id, request)
}

func (s *Scope) Cancel(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Cancel(s, ctx, id, request)
}

func (s *Scope) Terminate(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Terminate(s, ctx, id, request)
}

func (s *Scope) Approve(ctx context.Context, id string, request fse.ApproveRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.Approve(s, ctx, id, request)
}

func (s *Scope) RetryDispatch(ctx context.Context, id string, request fse.RetryDispatchRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.RetryDispatch(s, ctx, id, request)
}

func (s *Scope) InterruptDispatch(ctx context.Context, id string, request fse.InterruptDispatchRequest) (fse.LifecycleControlResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.InterruptDispatch(s, ctx, id, request)
}

func (s *Scope) GetResult(ctx context.Context, id string, req fse.ResultRequest) (fse.ResultReadResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.GetResult(s, ctx, id, req)
}

func (s *Scope) ListDispatches(ctx context.Context, id string) (fse.ListDispatchesResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.ListDispatches(s, ctx, id)
}

func (s *Scope) QueryDispatches(ctx context.Context, request fse.DispatchQueryRequest) (fse.ListDispatchesResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.QueryDispatches(s, ctx, request)
}

func (s *Scope) GetDispatch(ctx context.Context, id, dispatchID string) (fse.DispatchDetail, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.GetDispatch(s, ctx, id, dispatchID)
}

func (s *Scope) ListArtifacts(ctx context.Context, id string) (fse.ListArtifactsResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.ListArtifacts(s, ctx, id)
}

func (s *Scope) GetArtifact(ctx context.Context, id, artifactID string) (fse.ArtifactDetail, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.GetArtifact(s, ctx, id, artifactID)
}

func (s *Scope) ReadEvents(ctx context.Context, id string, req fse.EventReconnectRequest) (fse.EventReadResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.ReadEvents(s, ctx, id, req)
}

func (s *Scope) ListSessions(ctx context.Context, request fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
	var behavior *Behavior
	if s != nil {
		behavior = s.behavior
	}
	return behavior.ListSessions(s, ctx, request)
}
