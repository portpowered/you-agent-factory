package service

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
)

type service struct {
	dependencies liveruntime.Dependencies
}

var _ liveruntime.Service = (*service)(nil)

func New(dependencies liveruntime.Dependencies) (liveruntime.Service, error) {
	switch {
	case dependencies.OpenForTarget == nil:
		return nil, fmt.Errorf("construct live-runtime service: open runtime is required")
	case dependencies.ListSessionIDs == nil:
		return nil, fmt.Errorf("construct live-runtime service: session listing is required")
	case dependencies.GetSession == nil:
		return nil, fmt.Errorf("construct live-runtime service: session lookup is required")
	case dependencies.RequireSession == nil:
		return nil, fmt.Errorf("construct live-runtime service: required session lookup is required")
	case dependencies.BuildProjectionContext == nil:
		return nil, fmt.Errorf("construct live-runtime service: projection builder is required")
	case dependencies.SessionFactory == nil:
		return nil, fmt.Errorf("construct live-runtime service: session runtime lookup is required")
	case dependencies.StopSession == nil:
		return nil, fmt.Errorf("construct live-runtime service: stop runtime is required")
	case dependencies.ObserveControl == nil:
		return nil, fmt.Errorf("construct live-runtime service: lifecycle observer is required")
	default:
		return &service{dependencies: dependencies}, nil
	}
}

func (s *service) OpenForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if s == nil {
		return "", fmt.Errorf("live-runtime service is required")
	}
	return s.dependencies.OpenForTarget(ctx, target)
}

func (s *service) List(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if s == nil {
		return nil, fmt.Errorf("live-runtime service is required")
	}
	return controlplane.ListLiveFactorySessions(ctx, liveReadHost{s.dependencies})
}

func (s *service) Get(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if s == nil {
		return factorysessions.SessionProjection{}, fmt.Errorf("live-runtime service is required")
	}
	return controlplane.GetLiveFactorySession(ctx, liveReadHost{s.dependencies}, sessionID)
}

func (s *service) Resolve(sessionID string) *factorysessions.LiveSession {
	if s == nil {
		return nil
	}
	return s.dependencies.GetSession(sessionID)
}

func (s *service) Snapshot(ctx context.Context, sessionID string) (*factoryruntime.StateSnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("live-runtime service is required")
	}
	runtime, err := s.dependencies.SessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := runtime.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *service) ApplyControl(ctx context.Context, sessionID string, operation factorysessions.LifecycleControlKind, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if s == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("live-runtime service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	if _, err := factorysessions.NormalizeControlRequest(control); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	activeFactory, err := s.dependencies.SessionFactory(sessionID)
	if err != nil {
		s.dependencies.ObserveControl(sessionID, operation, control, "", "", err)
		return factorysessions.LifecycleControlResult{}, err
	}
	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("get engine state snapshot: %w", err)
	}
	currentStatus := factorysessions.LifecycleStatusFromFactoryRuntimeState(snapshot.FactoryState)
	outcome := factorysessions.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessions.LifecycleControlOutcomeInvalidState || outcome == factorysessions.LifecycleControlOutcomeTerminalSession {
		controlErr := &factorysessions.ControlError{Operation: operation, Outcome: outcome, Status: currentStatus, Message: fmt.Sprintf("%s rejected for session %s in status %s", operation, sessionID, currentStatus), Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID)}
		s.dependencies.ObserveControl(sessionID, operation, control, outcome, currentStatus, controlErr)
		return factorysessions.LifecycleControlResult{}, controlErr
	}
	resultStatus, err := applyAcceptedControl(ctx, activeFactory, operation, outcome, currentStatus)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	result := factorysessions.LifecycleControlResult{SessionID: sessionID, Operation: operation, Outcome: outcome, Status: resultStatus, Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID)}
	s.dependencies.ObserveControl(sessionID, operation, control, outcome, resultStatus, nil)
	return result, nil
}

func applyAcceptedControl(ctx context.Context, activeFactory factoryruntime.Service, operation factorysessions.LifecycleControlKind, outcome factorysessions.LifecycleControlOutcome, currentStatus factorysessions.LifecycleStatus) (factorysessions.LifecycleStatus, error) {
	if outcome != factorysessions.LifecycleControlOutcomeAccepted {
		return currentStatus, nil
	}
	switch operation {
	case factorysessions.LifecycleControlPause:
		if err := activeFactory.Pause(ctx); err != nil {
			return "", fmt.Errorf("pause live factory session: %w", err)
		}
		return factorysessions.LifecycleStatusPaused, nil
	case factorysessions.LifecycleControlResume:
		if err := activeFactory.Resume(ctx); err != nil {
			return "", fmt.Errorf("resume live factory session: %w", err)
		}
		return factorysessions.LifecycleStatusRunning, nil
	default:
		return "", fmt.Errorf("unsupported live lifecycle operation %s", operation)
	}
}

func (s *service) Close(ctx context.Context, sessionID string) error {
	if s == nil {
		return fmt.Errorf("live-runtime service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("factory session id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.dependencies.StopSession(sessionID)
}

type liveReadHost struct{ dependencies liveruntime.Dependencies }

func (h liveReadHost) ListLiveSessionIDs() []string { return h.dependencies.ListSessionIDs() }
func (h liveReadHost) GetLiveSession(id string) *factorysessions.LiveSession {
	return h.dependencies.GetSession(id)
}
func (h liveReadHost) RequireSession(id string) (*factorysessions.LiveSession, error) {
	return h.dependencies.RequireSession(id)
}
func (h liveReadHost) BuildSessionProjectionContext(ctx context.Context, session *factorysessions.LiveSession) (factorysessions.ProjectionContext, error) {
	return h.dependencies.BuildProjectionContext(ctx, session)
}
