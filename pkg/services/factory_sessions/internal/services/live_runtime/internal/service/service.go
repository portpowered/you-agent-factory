package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
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

func (s *service) Resolve(sessionID string) *livesession.LiveSession {
	if s == nil {
		return nil
	}
	return s.dependencies.GetSession(sessionID)
}

func (s *service) Snapshot(ctx context.Context, sessionID string) (*legacysnapshot.Snapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("live-runtime service is required")
	}
	runtime, err := s.dependencies.SessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	legacyObservation, err := runtimebinding.LegacyObservationForService(runtime)
	if err != nil {
		return nil, err
	}
	snapshot, err := legacyObservation.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *service) Observe(
	ctx context.Context,
	sessionID string,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if s == nil {
		return factoryruntime.ObserveResult{}, fmt.Errorf("live-runtime service is required")
	}
	runtime, err := s.dependencies.SessionFactory(sessionID)
	if err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	return runtime.Observe(ctx, req)
}

func (s *service) ApplyControl(ctx context.Context, sessionID string, operation factorysessions.LifecycleControlKind, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if s == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("live-runtime service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	if _, err := factorysessionexecution.NormalizeControlRequest(control); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	activeFactory, err := s.dependencies.SessionFactory(sessionID)
	if err != nil {
		s.dependencies.ObserveControl(sessionID, operation, control, "", "", err)
		return factorysessions.LifecycleControlResult{}, err
	}
	observeResult, err := activeFactory.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeHealth,
	})
	if err != nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("observe live factory session: %w", err)
	}
	currentStatus := factorysessions.LifecycleStatusFromFactoryRuntimeState(observeResult.Observation.Health.FactoryState)
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
		if _, err := activeFactory.ControlPause(ctx, factoryruntime.PauseRequest{}); err != nil {
			return "", fmt.Errorf("pause live factory session: %w", err)
		}
		return factorysessions.LifecycleStatusPaused, nil
	case factorysessions.LifecycleControlResume:
		if _, err := activeFactory.ControlResume(ctx, factoryruntime.ResumeRequest{}); err != nil {
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
	session, err := s.dependencies.RequireSession(sessionID)
	if err != nil {
		return err
	}
	if session != nil && session.Runtime != nil && session.Runtime.Factory != nil {
		_, terminateErr := session.Runtime.Factory.ControlTerminate(ctx, factoryruntime.TerminateRequest{
			Reason: "factory session closed",
		})
		if terminateErr != nil &&
			!errors.Is(terminateErr, factoryruntime.ErrAlreadyStopped) &&
			!errors.Is(terminateErr, factoryruntime.ErrNotRunning) {
			return fmt.Errorf("terminate live factory session: %w", terminateErr)
		}
	}
	return s.dependencies.StopSession(sessionID)
}

type liveReadHost struct{ dependencies liveruntime.Dependencies }

func (h liveReadHost) ListLiveSessionIDs() []string { return h.dependencies.ListSessionIDs() }
func (h liveReadHost) GetLiveSession(id string) *livesession.LiveSession {
	return h.dependencies.GetSession(id)
}
func (h liveReadHost) RequireSession(id string) (*livesession.LiveSession, error) {
	return h.dependencies.RequireSession(id)
}
func (h liveReadHost) BuildSessionProjectionContext(ctx context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return h.dependencies.BuildProjectionContext(ctx, session)
}
