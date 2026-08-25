package runtime

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// cloneFactoryEventsInOrder keeps a detached copy without changing the
// append-order prefix used to decide whether restored state is still current.
func cloneFactoryEventsInOrder(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		cloned[index] = event.Clone()
	}
	return cloned
}

func (s *recordedWorkerSessionObservation) projectRecordedWorldState(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	ordered []interfaces.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	if restored, ok := restoredWorldStateForEvents(s.restoredWorldState, s.restoredEventPrefix, events); ok {
		return *restored, nil
	}
	if s == nil || s.projector == nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	world, err := s.projector(ordered, selectedTick)
	if err != nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	return world, nil
}

func restoredWorldStateForEvents(
	state *interfaces.FactoryWorldState,
	prefix []interfaces.FactoryEvent,
	events []interfaces.FactoryEvent,
) (*interfaces.FactoryWorldState, bool) {
	if state == nil || len(prefix) == 0 || len(events) < len(prefix) {
		return nil, false
	}
	for index := range prefix {
		if !sameFactoryEventIdentity(prefix[index], events[index]) {
			return nil, false
		}
	}
	for _, event := range events[len(prefix):] {
		if factoryEventRequiresWorkerSessionProjection(*state, event) {
			return nil, false
		}
	}
	return state, true
}

func sameFactoryEventIdentity(left, right interfaces.FactoryEvent) bool {
	if left.Id != "" || right.Id != "" {
		return left.Id == right.Id
	}
	return left.Type == right.Type &&
		left.Context.Tick == right.Context.Tick &&
		left.Context.Sequence == right.Context.Sequence &&
		left.Context.EventTime.Equal(right.Context.EventTime)
}

func factoryEventRequiresWorkerSessionProjection(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionLifecycleControl,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeRunResponse:
		return false
	case interfaces.FactoryEventTypeWorkRequest:
		return !restoredWorkRequestEventIsKnown(state, event)
	default:
		return true
	}
}

func restoredWorkRequestEventIsKnown(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	workIDs := pointerStringSlice(event.Context.WorkIDs)
	for _, workID := range workIDs {
		if _, ok := state.WorkItemsByID[workID]; !ok {
			return false
		}
	}
	requestID := stringPointerValue(event.Context.RequestID)
	if requestID != "" {
		for key, request := range state.WorkRequestsByID {
			if key == requestID || request.RequestID == requestID {
				return len(workIDs) > 0 || len(request.WorkItems) > 0
			}
		}
		return false
	}
	return len(workIDs) > 0
}
