package engine

import (
	"context"
	"fmt"
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

var (
	ErrMoveWorkNotFound         = factory.ErrMoveWorkNotFound
	ErrMoveWorkInvalidState     = factory.ErrMoveWorkInvalidState
	ErrMoveWorkInFlightDispatch = factory.ErrMoveWorkInFlightDispatch
	ErrMoveWorkEngineTerminated = factory.ErrMoveWorkEngineTerminated
)

// MoveWork validates and applies a synchronous operator relocation for one work item.
// It does not emit dispatch events or require the factory lifecycle to be running.
func (e *FactoryEngine) MoveWork(ctx context.Context, workID string, stateName string) (work.OperatorMoveResult, error) {
	select {
	case <-ctx.Done():
		return work.OperatorMoveResult{}, ctx.Err()
	default:
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.acceptingSubmits {
		return work.OperatorMoveResult{}, ErrMoveWorkEngineTerminated
	}

	activeEntries := activeDispatchEntriesForWork(e.runtimeState.Dispatches, workID)
	var activeToken *factorytoken.Token
	if len(activeEntries) > 0 {
		var activeTokenFound bool
		activeToken, activeTokenFound = findWorkTokenInDispatchEntries(activeEntries, workID)
		if !activeTokenFound {
			return work.OperatorMoveResult{}, ErrMoveWorkNotFound
		}
	}
	token, ok := findWorkTokenByID(e.runtimeState.Marking.Tokens, workID)
	if activeToken != nil {
		// The dispatch entry is authoritative while a Work token is consumed;
		// this keeps the move tied to the exact token that the late result will
		// otherwise correlate.
		token, ok = activeToken, true
	}
	if !ok {
		return work.OperatorMoveResult{}, ErrMoveWorkNotFound
	}

	toPlaceID, err := resolveTargetPlace(e.state, token.Color.WorkTypeID, stateName)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}

	fromPlaceID := token.PlaceID
	fromState := stateValueForPlace(e.state, fromPlaceID)
	if fromState == "" {
		return work.OperatorMoveResult{}, fmt.Errorf("resolve current state for place %q: place not found", fromPlaceID)
	}
	if len(activeEntries) > 0 {
		if err := restoreActiveDispatchTokens(e.runtimeState.Marking, e.state.Places, activeEntries); err != nil {
			return work.OperatorMoveResult{}, fmt.Errorf("restore active dispatch tokens for Work %q: %w", workID, err)
		}
		var restored bool
		token, restored = e.runtimeState.Marking.Tokens[activeToken.ID]
		if !restored {
			return work.OperatorMoveResult{}, ErrMoveWorkNotFound
		}
		fromPlaceID = token.PlaceID
		fromState = stateValueForPlace(e.state, fromPlaceID)
	}
	if fromPlaceID == toPlaceID {
		if len(activeEntries) > 0 {
			e.publishRuntimeSnapshotLocked()
		}
		return work.OperatorMoveResult{
			WorkID:     workID,
			WorkTypeID: token.Color.WorkTypeID,
			FromState:  fromState,
			ToState:    stateName,
			TokenID:    token.ID,
		}, nil
	}

	if leavingFailedPlace(e.state, token.Color.WorkTypeID, fromState) {
		factorytoken.ClearGuardBlockingFields(&token.History)
	}

	mutation := interfaces.MarkingMutation{
		Type:      interfaces.MutationMove,
		TokenID:   token.ID,
		FromPlace: fromPlaceID,
		ToPlace:   toPlaceID,
		Reason:    fmt.Sprintf("operator move to %s", stateName),
	}
	if err := applyMutations(e.runtimeState.Marking, e.state.Places, []interfaces.MarkingMutation{mutation}, e.clock.Now()); err != nil {
		return work.OperatorMoveResult{}, fmt.Errorf("apply operator move: %w", err)
	}
	e.publishRuntimeSnapshotLocked()
	if len(activeEntries) == 0 {
		e.wakeForOperatorControl()
	}

	return work.OperatorMoveResult{
		WorkID:     workID,
		WorkTypeID: token.Color.WorkTypeID,
		FromState:  fromState,
		ToState:    stateName,
		TokenID:    token.ID,
	}, nil
}

func findWorkTokenByID(tokens map[string]*factorytoken.Token, workID string) (*factorytoken.Token, bool) {
	for _, token := range tokens {
		if token == nil || token.Color.WorkID != workID {
			continue
		}
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		return token, true
	}
	return nil, false
}

func workIDInActiveDispatches(dispatches map[string]*interfaces.DispatchEntry, workID string) bool {
	return len(activeDispatchEntriesForWork(dispatches, workID)) > 0
}

// activeDispatchEntriesForWork returns matching active entries in map-key
// order. The engine keeps their consumed tokens until the terminal result is
// retired, so an operator move must restore those exact tokens before moving
// the requested Work. The entries themselves remain active for result
// correlation and the Runtime outbox invalidation/cancellation boundary.
func activeDispatchEntriesForWork(
	dispatches map[string]*interfaces.DispatchEntry,
	workID string,
) []*interfaces.DispatchEntry {
	if len(dispatches) == 0 || workID == "" {
		return nil
	}
	keys := make([]string, 0, len(dispatches))
	for key, entry := range dispatches {
		if entry == nil || !dispatchEntryContainsWork(entry, workID) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]*interfaces.DispatchEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, dispatches[key])
	}
	return entries
}

func dispatchEntryContainsWork(entry *interfaces.DispatchEntry, workID string) bool {
	for _, token := range entry.ConsumedTokens {
		if token.Color.WorkID == workID && token.Color.DataType != factorytoken.DataTypeResource {
			return true
		}
	}
	return false
}

func findWorkTokenInDispatchEntries(
	entries []*interfaces.DispatchEntry,
	workID string,
) (*factorytoken.Token, bool) {
	for _, entry := range entries {
		for _, workerToken := range entry.ConsumedTokens {
			if workerToken.Color.WorkID != workID || workerToken.Color.DataType == factorytoken.DataTypeResource {
				continue
			}
			token := factorytoken.FromWorker(workerToken)
			cloned := factorytoken.Clone(token)
			return &cloned, true
		}
	}
	return nil, false
}

func restoreActiveDispatchTokens(
	marking *petri.Marking,
	places map[string]*petri.Place,
	entries []*interfaces.DispatchEntry,
) error {
	restored := make([]factorytoken.Token, 0)
	seen := make(map[string]struct{})
	for _, entry := range entries {
		for _, workerToken := range entry.ConsumedTokens {
			token := factorytoken.FromWorker(workerToken)
			if token.ID == "" {
				return fmt.Errorf("active dispatch contains a token without an ID")
			}
			if token.PlaceID == "" {
				return fmt.Errorf("active dispatch token %q has no source place", token.ID)
			}
			if _, ok := places[token.PlaceID]; !ok {
				return fmt.Errorf("active dispatch token %q source place %q not found", token.ID, token.PlaceID)
			}
			if _, exists := seen[token.ID]; exists {
				continue
			}
			seen[token.ID] = struct{}{}
			if _, exists := marking.Tokens[token.ID]; exists {
				continue
			}
			restored = append(restored, factorytoken.Clone(token))
		}
	}
	for index := range restored {
		cloned := restored[index]
		marking.AddToken(&cloned)
	}
	return nil
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
