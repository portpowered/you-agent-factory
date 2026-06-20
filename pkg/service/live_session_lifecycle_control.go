package service

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func (fs *FactoryService) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.applyLiveLifecycleControl(
		ctx,
		sessionID,
		factorysessionexecution.LifecycleControlPause,
		control,
	)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := fs.applyLiveLifecycleControl(
		ctx,
		sessionID,
		factorysessionexecution.LifecycleControlResume,
		control,
	)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *FactoryService) applyLiveLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if fs == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	if _, err := factorysessionexecution.NormalizeControlRequest(control); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}

	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
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
		return factorysessionexecution.LifecycleControlResult{}, &factorysessionexecution.ControlError{
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

	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    resultStatus,
		Links:     factorysession.LiveLifecycleControlLinksForSession(sessionID),
	}, nil
}
