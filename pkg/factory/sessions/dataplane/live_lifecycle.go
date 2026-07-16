package dataplane

import (
	"context"
	"fmt"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
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
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if l == nil || l.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("live session dataplane host is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	if _, err := factorysessionexecution.NormalizeControlRequest(control); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	activeFactory, err := l.host.SessionFactory(sessionID)
	if err != nil {
		l.host.ObserveLiveLifecycleControl(sessionID, operation, control, "", "", err)
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("get engine state snapshot: %w", err)
	}

	currentStatus := factorysessionexecution.LifecycleStatusFromFactoryRuntimeState(snapshot.FactoryState)
	outcome := factorysessionexecution.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessionexecution.LifecycleControlOutcomeInvalidState ||
		outcome == factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		controlErr := &factorysessionexecution.ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message: fmt.Sprintf(
				"%s rejected for session %s in status %s",
				operation,
				sessionID,
				currentStatus,
			),
		}
		l.host.ObserveLiveLifecycleControl(sessionID, operation, control, outcome, currentStatus, controlErr)
		return factorysessionexecution.LifecycleControlResult{}, controlErr
	}

	resultStatus := currentStatus
	if outcome == factorysessionexecution.LifecycleControlOutcomeAccepted {
		switch operation {
		case factorysessionexecution.LifecycleControlPause:
			if err := activeFactory.Pause(ctx); err != nil {
				return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("pause live factory session: %w", err)
			}
			resultStatus = factorysessionexecution.LifecycleStatusPaused
		case factorysessionexecution.LifecycleControlResume:
			if err := activeFactory.Resume(ctx); err != nil {
				return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("resume live factory session: %w", err)
			}
			resultStatus = factorysessionexecution.LifecycleStatusRunning
		default:
			return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("unsupported live lifecycle operation %s", operation)
		}
	}

	result := factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    resultStatus,
		Links:     factorysessionexecution.LiveLifecycleControlLinksForSession(sessionID),
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
