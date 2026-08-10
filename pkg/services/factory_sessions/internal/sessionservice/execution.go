package service

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func (s *Service) durableExecution() (durableexecution.Service, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.durable, nil
}

func (s *Service) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	return execution.StartAsync(ctx, request)
}

func (s *Service) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SyncStartResult{}, err
	}
	return execution.StartSync(ctx, request)
}

// StartSyncWithEventConsumer forwards the owner-private response presentation
// capability when the selected durable execution supports it. The public
// Factory Sessions contract remains value-only; invocation presentation is
// discovered only by the internal runtime-opening capability check.
func (s *Service) StartSyncWithEventConsumer(
	ctx context.Context,
	request factorysessions.StartRequest,
	consume factorysessions.FactoryEventConsumer,
) (factorysessions.SyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SyncStartResult{}, err
	}
	observed, ok := execution.(interface {
		StartSyncWithEventConsumer(
			context.Context,
			factorysessions.StartRequest,
			factorysessions.FactoryEventConsumer,
		) (factorysessions.SyncStartResult, error)
	})
	if !ok {
		return execution.StartSync(ctx, request)
	}
	return observed.StartSyncWithEventConsumer(ctx, request, consume)
}

func (s *Service) ResumeInterruptedSession(ctx context.Context, sessionID string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	return execution.ResumeInterruptedSession(ctx, sessionID, request)
}

func (s *Service) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SessionReadResult{}, err
	}
	return execution.GetSession(ctx, sessionID)
}

func (s *Service) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Pause(ctx, sessionID, request)
}

func (s *Service) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Resume(ctx, sessionID, request)
}

func (s *Service) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Cancel(ctx, sessionID, request)
}

func (s *Service) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Terminate(ctx, sessionID, request)
}

func (s *Service) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Approve(ctx, sessionID, request)
}

func (s *Service) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.RetryDispatch(ctx, sessionID, request)
}

func (s *Service) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.InterruptDispatch(ctx, sessionID, request)
}

func (s *Service) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ResultReadResult{}, err
	}
	return execution.GetResult(ctx, sessionID, request)
}

func (s *Service) ListDispatches(ctx context.Context, sessionID string) (factorysessions.ListDispatchesResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return execution.ListDispatches(ctx, sessionID)
}

func (s *Service) QueryDispatches(ctx context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return execution.QueryDispatches(ctx, request)
}

func (s *Service) GetDispatch(ctx context.Context, sessionID, dispatchID string) (factorysessions.DispatchDetail, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.DispatchDetail{}, err
	}
	return execution.GetDispatch(ctx, sessionID, dispatchID)
}

func (s *Service) ListArtifacts(ctx context.Context, sessionID string) (factorysessions.ListArtifactsResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListArtifactsResult{}, err
	}
	return execution.ListArtifacts(ctx, sessionID)
}

func (s *Service) GetArtifact(ctx context.Context, sessionID, artifactID string) (factorysessions.ArtifactDetail, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ArtifactDetail{}, err
	}
	return execution.GetArtifact(ctx, sessionID, artifactID)
}

func (s *Service) ReadEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.EventReadResult{}, err
	}
	return execution.ReadEvents(ctx, sessionID, request)
}

func (s *Service) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListSessionsResult{}, err
	}
	return execution.ListSessions(ctx, request)
}
