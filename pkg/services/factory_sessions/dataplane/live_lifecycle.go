package dataplane

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// LiveLifecycle supervises live session pause, resume, and close through the runtime host seam.
type LiveLifecycle struct {
	host LiveLifecycleHost
}

// NewLiveLifecycle constructs a live lifecycle dataplane collaborator.
func NewLiveLifecycle(host LiveLifecycleHost) *LiveLifecycle {
	return &LiveLifecycle{host: host}
}

// ApplyControl evaluates and applies one live lifecycle control request.
func (l *LiveLifecycle) ApplyControl(
	ctx context.Context,
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if l == nil || l.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("live session dataplane host is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}

	if _, err := factorysessions.NormalizeControlRequest(control); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}

	activeFactory, err := l.host.SessionFactory(sessionID)
	if err != nil {
		l.host.ObserveLiveLifecycleControl(sessionID, operation, control, "", "", err)
		return factorysessions.LifecycleControlResult{}, err
	}

	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("get engine state snapshot: %w", err)
	}

	currentStatus := factorysessions.LifecycleStatusFromFactoryRuntimeState(snapshot.FactoryState)
	outcome := factorysessions.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessions.LifecycleControlOutcomeInvalidState ||
		outcome == factorysessions.LifecycleControlOutcomeTerminalSession {
		controlErr := &factorysessions.ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message: fmt.Sprintf(
				"%s rejected for session %s in status %s",
				operation,
				sessionID,
				currentStatus,
			),
			Links: factorysessions.LiveLifecycleControlLinksForSession(sessionID),
		}
		l.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, currentStatus, controlErr)
		return factorysessions.LifecycleControlResult{}, controlErr
	}

	resultStatus := currentStatus
	if outcome == factorysessions.LifecycleControlOutcomeAccepted {
		switch operation {
		case factorysessions.LifecycleControlPause:
			if err := activeFactory.Pause(ctx); err != nil {
				return factorysessions.LifecycleControlResult{}, fmt.Errorf("pause live factory session: %w", err)
			}
			resultStatus = factorysessions.LifecycleStatusPaused
		case factorysessions.LifecycleControlResume:
			if err := activeFactory.Resume(ctx); err != nil {
				return factorysessions.LifecycleControlResult{}, fmt.Errorf("resume live factory session: %w", err)
			}
			resultStatus = factorysessions.LifecycleStatusRunning
		default:
			return factorysessions.LifecycleControlResult{}, fmt.Errorf("unsupported live lifecycle operation %s", operation)
		}
	}

	result := factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    resultStatus,
		Links:     factorysessions.LiveLifecycleControlLinksForSession(sessionID),
	}
	l.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, resultStatus, nil)
	return result, nil
}

// CloseSession stops one live session and updates live registry state.
func (l *LiveLifecycle) CloseSession(sessionID string) error {
	if l == nil || l.host == nil {
		return fmt.Errorf("live session dataplane host is required")
	}
	return l.host.StopLiveSession(sessionID)
}
