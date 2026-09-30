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
)

// DeleteFactorySession enforces the safe-delete policy against the canonical
// live session, then uses the same retryable cleanup as explicit close.
func (a *Assembly) DeleteFactorySession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return fmt.Errorf("factory session ID is required")
	}
	session := a.Resolve(id)
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
	return a.CloseSession(ctx, canonicalID)
}

func liveDeletionState(ctx context.Context, session *livesession.LiveSession) (string, error) {
	runtime := runtimebinding.ServiceForSession(session)
	if runtime == nil {
		return "STOPPED", nil
	}
	observed, err := runtime.Observe(ctx, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeHealth})
	if err != nil {
		if errors.Is(err, factoryruntime.ErrAlreadyStopped) || errors.Is(err, factoryruntime.ErrNotRunning) {
			return "STOPPED", nil
		}
		return "", fmt.Errorf("read live Factory Session runtime state: %w", err)
	}
	factoryState := strings.ToUpper(strings.TrimSpace(observed.Observation.Health.FactoryState))
	controlState := strings.ToUpper(strings.TrimSpace(observed.Observation.Health.LifecycleControlStatus))
	if activeDeletionState(controlState) {
		return controlState, nil
	}
	if stoppedDeletionState(factoryState) {
		return factoryState, nil
	}
	if factoryState == "" && observed.Observation.Status == factoryruntime.ObservationStatusFinished {
		return "FINISHED", nil
	}
	if stoppedDeletionState(controlState) {
		return controlState, nil
	}
	return factoryState, nil
}

func stoppedDeletionState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "COMPLETED", "FAILED", "FINISHED", "STOPPED", "SUCCEEDED", "CANCELED", "TERMINATED":
		return true
	default:
		return false
	}
}

func activeDeletionState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "RUNNING", "PAUSING", "PAUSED", "RESUMING", "CANCELING", "TERMINATING", "QUEUED", "AWAITING_APPROVAL":
		return true
	default:
		return false
	}
}
