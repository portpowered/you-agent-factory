package service

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (fs *FactoryService) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if _, err := factorysession.ControlRequestFromAPI(request); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return fs.applyLiveFactorySessionLifecycleControl(ctx, sessionID, factorysessionexecution.LifecycleControlPause)
}

func (fs *FactoryService) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if _, err := factorysession.ControlRequestFromAPI(request); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return fs.applyLiveFactorySessionLifecycleControl(ctx, sessionID, factorysessionexecution.LifecycleControlResume)
}

func (fs *FactoryService) applyLiveFactorySessionLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}

	snapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("get engine state snapshot: %w", err)
	}

	currentState := interfaces.FactoryState(snapshot.FactoryState)
	currentStatus := factorysession.FactoryStateToLifecycleStatus(currentState)
	outcome := factorysessionexecution.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessionexecution.LifecycleControlOutcomeInvalidState ||
		outcome == factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		return factoryapi.FactorySessionLifecycleControlResponse{}, &factorysessionexecution.ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message:   fmt.Sprintf("%s rejected for session %s in factory state %s", operation, sessionID, currentState),
		}
	}

	if outcome == factorysessionexecution.LifecycleControlOutcomeAccepted {
		switch operation {
		case factorysessionexecution.LifecycleControlPause:
			if err := activeFactory.Pause(ctx); err != nil {
				return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("pause factory session: %w", err)
			}
		case factorysessionexecution.LifecycleControlResume:
			if err := activeFactory.Resume(ctx); err != nil {
				return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("resume factory session: %w", err)
			}
		}
	}

	updatedSnapshot, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("get engine state snapshot after control: %w", err)
	}
	updatedStatus := factorysession.FactoryStateToLifecycleStatus(interfaces.FactoryState(updatedSnapshot.FactoryState))
	return factorysession.LiveLifecycleControlResponse(sessionID, operation, outcome, updatedStatus), nil
}
