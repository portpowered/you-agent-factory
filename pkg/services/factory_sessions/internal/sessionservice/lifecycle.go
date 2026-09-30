package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// PauseLiveFactorySession applies live pause control through the dataplane.
func (s *Service) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlPause, request)
}

// ResumeLiveFactorySession applies live resume control through the dataplane.
func (s *Service) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlResume, request)
}

// CancelLiveFactorySession requests graceful cancellation for one live
// session while retaining the stopped session in the registry for inspection
// and a subsequent safe delete.
func (s *Service) CancelLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlCancel, request)
}

// TerminateLiveFactorySession requests forced termination for one live session
// while retaining the stopped session in the registry for inspection and a
// subsequent safe delete.
func (s *Service) TerminateLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlTerminate, request)
}

// CloseFactorySession stops one live session through the dataplane.
func (s *Service) CloseFactorySession(ctx context.Context, sessionID string) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("live-runtime service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("factory session id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return err
	}
	if runtime := runtimebinding.ServiceForSession(session); runtime != nil {
		_, terminateErr := runtime.ControlTerminate(ctx, factoryruntime.TerminateRequest{
			Reason: "factory session closed",
		})
		if terminateErr != nil &&
			!errors.Is(terminateErr, factoryruntime.ErrAlreadyStopped) &&
			!errors.Is(terminateErr, factoryruntime.ErrNotRunning) {
			return fmt.Errorf("terminate live factory session: %w", terminateErr)
		}
	}
	return s.host.StopLiveSession(sessionID)
}

func (s *Service) applyLiveLifecycleControl(ctx context.Context, sessionID string, operation factorysessions.LifecycleControlKind, control factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("live-runtime service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	control, err := factorysessionexecution.NormalizeControlRequest(control)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	activeFactory, err := s.host.SessionFactory(sessionID)
	if err != nil {
		s.host.ObserveLiveLifecycleControl(sessionID, operation, control, "", "", err)
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
		s.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, currentStatus, controlErr)
		return factorysessions.LifecycleControlResult{}, controlErr
	}
	resultStatus, appliedOutcome, err := currentStatus, outcome, error(nil)
	if outcome == factorysessions.LifecycleControlOutcomeAccepted {
		resultStatus, appliedOutcome, err = applyOwnedControl(ctx, activeFactory, operation, currentStatus, control)
	}
	if err != nil {
		s.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, currentStatus, err)
		return factorysessions.LifecycleControlResult{}, err
	}
	if appliedOutcome != "" {
		outcome = appliedOutcome
	}
	result := factorysessions.LifecycleControlResult{SessionID: sessionID, Operation: operation, Outcome: outcome, Status: resultStatus, Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID)}
	s.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, resultStatus, nil)
	return result, nil
}

var _ factorysessions.LiveDeletionService = (*Service)(nil)
var _ factorysessions.LiveDeletionService = (*Assembly)(nil)

// DeleteFactorySession applies the Factory Sessions-owned safe deletion policy
// without routing the request through the destructive close lifecycle path.
func (s *Service) DeleteFactorySession(ctx context.Context, sessionID string) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("factory session gateway is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return fmt.Errorf("factory session ID is required")
	}
	session := s.host.GetLiveSession(id)
	if session == nil {
		return fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}
	canonicalID := strings.TrimSpace(session.ID)
	if session.IsDefault || canonicalID == factorysessions.DefaultSessionID {
		return &factorysessions.SessionDeletionError{
			SessionID: canonicalID, Reason: factorysessions.SessionDeletionReasonDefault,
			Message: fmt.Sprintf("factory session %q cannot be deleted: the default session cannot be deleted; make a different session the default first", canonicalID),
		}
	}
	status, err := liveDeletionState(ctx, session)
	if err != nil {
		return err
	}
	if !stoppedDeletionState(status) {
		return &factorysessions.SessionDeletionError{
			SessionID: canonicalID, Reason: factorysessions.SessionDeletionReasonRuntimeActive,
			Status:  factorysessions.LifecycleStatus(status),
			Message: fmt.Sprintf("factory session %q cannot be deleted: runtime is %s and must be stopped with cancel or terminate before retrying deletion", canonicalID, status),
		}
	}
	return s.host.StopLiveSession(canonicalID)
}
