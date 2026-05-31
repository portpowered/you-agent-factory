package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var (
	// ErrMoveWorkNotFound indicates no work token matches the requested work ID.
	ErrMoveWorkNotFound = errors.New("work not found")
	// ErrMoveWorkInvalidState indicates the target state is unknown for the work type.
	ErrMoveWorkInvalidState = errors.New("invalid target state for work type")
	// ErrMoveWorkInFlightDispatch indicates the work item is consumed by an active dispatch.
	ErrMoveWorkInFlightDispatch = errors.New("work is in an active dispatch")
	// ErrMoveWorkEngineTerminated indicates the engine no longer accepts control ingress.
	ErrMoveWorkEngineTerminated = errors.New("engine has terminated")
)

// MoveWork validates and applies a synchronous operator relocation for one work item.
// It does not emit dispatch events or require the factory lifecycle to be running.
func (e *FactoryEngine) MoveWork(ctx context.Context, workID string, stateName string) (interfaces.OperatorMoveResult, error) {
	select {
	case <-ctx.Done():
		return interfaces.OperatorMoveResult{}, ctx.Err()
	default:
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.acceptingSubmits {
		return interfaces.OperatorMoveResult{}, ErrMoveWorkEngineTerminated
	}

	token, ok := findWorkTokenByID(e.runtimeState.Marking.Tokens, workID)
	if !ok {
		return interfaces.OperatorMoveResult{}, ErrMoveWorkNotFound
	}

	if workIDInActiveDispatches(e.runtimeState.Dispatches, workID) {
		return interfaces.OperatorMoveResult{}, ErrMoveWorkInFlightDispatch
	}

	toPlaceID, err := resolveTargetPlace(e.state, token.Color.WorkTypeID, stateName)
	if err != nil {
		return interfaces.OperatorMoveResult{}, err
	}

	fromPlaceID := token.PlaceID
	fromState := stateValueForPlace(e.state, fromPlaceID)
	if fromState == "" {
		return interfaces.OperatorMoveResult{}, fmt.Errorf("resolve current state for place %q: place not found", fromPlaceID)
	}
	if fromPlaceID == toPlaceID {
		return interfaces.OperatorMoveResult{
			WorkID:      workID,
			WorkTypeID:  token.Color.WorkTypeID,
			FromState:   fromState,
			ToState:     stateName,
			FromPlaceID: fromPlaceID,
			ToPlaceID:   toPlaceID,
			TokenID:     token.ID,
		}, nil
	}

	if leavingFailedPlace(e.state, token.Color.WorkTypeID, fromState) {
		interfaces.ClearGuardBlockingFields(&token.History)
	}

	mutation := interfaces.MarkingMutation{
		Type:      interfaces.MutationMove,
		TokenID:   token.ID,
		FromPlace: fromPlaceID,
		ToPlace:   toPlaceID,
		Reason:    fmt.Sprintf("operator move to %s", stateName),
	}
	if err := applyMutations(e.runtimeState.Marking, e.state.Places, []interfaces.MarkingMutation{mutation}); err != nil {
		return interfaces.OperatorMoveResult{}, fmt.Errorf("apply operator move: %w", err)
	}
	e.wakeForOperatorControl()

	return interfaces.OperatorMoveResult{
		WorkID:      workID,
		WorkTypeID:  token.Color.WorkTypeID,
		FromState:   fromState,
		ToState:     stateName,
		FromPlaceID: fromPlaceID,
		ToPlaceID:   toPlaceID,
		TokenID:     token.ID,
	}, nil
}

func findWorkTokenByID(tokens map[string]*interfaces.Token, workID string) (*interfaces.Token, bool) {
	for _, token := range tokens {
		if token == nil || token.Color.WorkID != workID {
			continue
		}
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		return token, true
	}
	return nil, false
}

func workIDInActiveDispatches(dispatches map[string]*interfaces.DispatchEntry, workID string) bool {
	for _, entry := range dispatches {
		if entry == nil {
			continue
		}
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				return true
			}
		}
	}
	return false
}

func resolveTargetPlace(net *state.Net, workTypeID, stateName string) (string, error) {
	workType, ok := net.WorkTypes[workTypeID]
	if !ok {
		return "", ErrMoveWorkInvalidState
	}
	valid := false
	for _, stateDef := range workType.States {
		if stateDef.Value == stateName {
			valid = true
			break
		}
	}
	if !valid {
		return "", ErrMoveWorkInvalidState
	}

	placeID := state.PlaceID(workTypeID, stateName)
	if _, ok := net.Places[placeID]; !ok {
		return "", ErrMoveWorkInvalidState
	}
	return placeID, nil
}

func stateValueForPlace(net *state.Net, placeID string) string {
	place, ok := net.Places[placeID]
	if !ok {
		return ""
	}
	return place.State
}

func leavingFailedPlace(net *state.Net, workTypeID, fromState string) bool {
	return state.CategoryForState(net.WorkTypes, workTypeID, fromState) == state.StateCategoryFailed
}
