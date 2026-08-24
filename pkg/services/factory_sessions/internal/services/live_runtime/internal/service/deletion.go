package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
)

const (
	factoryRuntimeStateCompleted  = "COMPLETED"
	factoryRuntimeStateFailed     = "FAILED"
	factoryRuntimeStateFinished   = "FINISHED"
	factoryRuntimeStateStopped    = "STOPPED"
	factoryRuntimeStateSucceeded  = "SUCCEEDED"
	factoryRuntimeStateCanceled   = "CANCELED"
	factoryRuntimeStateTerminated = "TERMINATED"
)

var _ liveruntime.DeletionService = (*service)(nil)

// Delete removes one non-default live session only after its runtime is
// already stopped. It never sends a stop request as part of eligibility.
func (s *service) Delete(ctx context.Context, sessionID string) error {
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
	if session == nil {
		return factorysessions.ErrSessionNotFound
	}
	canonicalID := strings.TrimSpace(session.ID)
	if session.IsDefault || canonicalID == factorysessions.DefaultSessionID {
		return &factorysessions.SessionDeletionError{
			SessionID: canonicalID,
			Reason:    factorysessions.SessionDeletionReasonDefault,
			Message:   fmt.Sprintf("factory session %q cannot be deleted: the default session cannot be deleted; make a different session the default first", canonicalID),
		}
	}

	state, err := s.deletionRuntimeState(ctx, session)
	if err != nil {
		return err
	}
	if !isStoppedRuntimeState(state) {
		return &factorysessions.SessionDeletionError{
			SessionID: canonicalID,
			Reason:    factorysessions.SessionDeletionReasonRuntimeActive,
			Status:    factorysessions.LifecycleStatus(state),
			Message:   fmt.Sprintf("factory session %q cannot be deleted: runtime is %s and must be stopped with cancel or terminate before retrying deletion", canonicalID, state),
		}
	}

	return s.dependencies.StopSession(canonicalID)
}

func (s *service) deletionRuntimeState(ctx context.Context, session *livesession.LiveSession) (string, error) {
	runtime := runtimebinding.ServiceForSession(session)
	if runtime == nil {
		return factoryRuntimeStateStopped, nil
	}
	observed, err := runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeHealth,
	})
	if err != nil {
		if errors.Is(err, factoryruntime.ErrAlreadyStopped) || errors.Is(err, factoryruntime.ErrNotRunning) {
			return factoryRuntimeStateStopped, nil
		}
		return "", fmt.Errorf("read live factory session runtime state: %w", err)
	}
	factoryState := strings.ToUpper(strings.TrimSpace(observed.Observation.Health.FactoryState))
	controlState := strings.ToUpper(strings.TrimSpace(observed.Observation.Health.LifecycleControlStatus))
	if isActiveLifecycleState(controlState) {
		return controlState, nil
	}
	if isStoppedRuntimeState(factoryState) {
		return factoryState, nil
	}
	if factoryState == "" && observed.Observation.Status == factoryruntime.ObservationStatusFinished {
		return factoryRuntimeStateFinished, nil
	}
	if isStoppedRuntimeState(controlState) {
		return controlState, nil
	}
	return factoryState, nil
}

func isStoppedRuntimeState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case factoryRuntimeStateCompleted,
		factoryRuntimeStateFailed,
		factoryRuntimeStateFinished,
		factoryRuntimeStateStopped,
		factoryRuntimeStateSucceeded,
		factoryRuntimeStateCanceled,
		factoryRuntimeStateTerminated:
		return true
	default:
		return false
	}
}

func isActiveLifecycleState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "RUNNING", "PAUSING", "PAUSED", "RESUMING", "CANCELING", "TERMINATING", "QUEUED", "AWAITING_APPROVAL":
		return true
	default:
		return false
	}
}
