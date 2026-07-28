// Package recordingsqueries constructs Recordings root projection-query requests
// for Factory Visualization presentation paths.
package recordingsqueries

import (
	"encoding/json"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const visualizationCanonicalStreamGenerationID = "factory-visualization"

// ReconstructWorldState projects retained Factory events through recordings.Service.
func ReconstructWorldState(
	service recordings.Service,
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (recordings.WorldStateView, error) {
	if service == nil {
		return recordings.WorldStateView{}, recordings.ErrInvalidProjectionInput
	}
	result, err := service.ReconstructWorldState(ReconstructWorldStateRequest(events, selectedTick))
	if err != nil {
		return recordings.WorldStateView{}, err
	}
	return result.WorldState, nil
}

// QuerySimpleDashboard returns dashboard render data through recordings.Service.
func QuerySimpleDashboard(
	service recordings.Service,
	worldState recordings.WorldStateView,
) (recordings.SimpleDashboardRenderData, error) {
	if service == nil {
		return recordings.SimpleDashboardRenderData{}, recordings.ErrInvalidProjectionInput
	}
	result, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: worldState,
	})
	if err != nil {
		return recordings.SimpleDashboardRenderData{}, err
	}
	return result.Data, nil
}

// ValidateReconnectReplay validates reconnect observe input through recordings.Service.
func ValidateReconnectReplay(
	service recordings.Service,
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	if service == nil {
		return recordings.ErrInvalidProjectionInput
	}
	request, err := ValidateReconnectReplayRequest(events, cursor, scope)
	if err != nil {
		return err
	}
	return service.ValidateReconnectReplayFrom(request)
}

// ReconstructWorldStateRequest maps retained Factory events to the Recordings root
// reconstruction request shape.
func ReconstructWorldStateRequest(
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) recordings.ReconstructWorldStateRequest {
	return recordings.ReconstructWorldStateRequest{
		Scope:        reconnectScope(scopeFromFactoryReconnect(factorydefinitions.FactoryEventReconnectScope{})),
		Events:       canonicalEventsFromFactory(events),
		SelectedTick: selectedTick,
	}
}

// ValidateReconnectReplayRequest maps reconnect observe input to the Recordings root
// validation request shape.
func ValidateReconnectReplayRequest(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (recordings.ValidateReconnectReplayRequest, error) {
	canonicalEvents := canonicalEventsFromFactory(events)
	canonicalCursor, err := canonicalReconnectCursor(events, cursor)
	if err != nil {
		return recordings.ValidateReconnectReplayRequest{}, err
	}
	return recordings.ValidateReconnectReplayRequest{
		Events: canonicalEvents,
		Cursor: canonicalCursor,
		Scope:  reconnectScope(scopeFromFactoryReconnect(scope)),
	}, nil
}

// DecodeWorldStatePayload decodes one detached Recordings world-state view payload.
func DecodeWorldStatePayload(view recordings.WorldStateView) (factorydefinitions.FactoryWorldState, error) {
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 ||
		strings.TrimSpace(view.Payload) == "" {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrUnsupportedProjectionView
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
	}
	return state, nil
}

func scopeFromFactoryReconnect(scope factorydefinitions.FactoryEventReconnectScope) recordings.CanonicalEventScope {
	sessionID := strings.TrimSpace(scope.SessionID)
	if sessionID == "" {
		return recordings.CanonicalEventScope{}
	}
	return recordings.CanonicalEventScope{FactorySessionID: sessionID}
}

func reconnectScope(scope recordings.CanonicalEventScope) recordings.CanonicalEventScope {
	return scope
}

func canonicalEventsFromFactory(events []factorydefinitions.FactoryEvent) []recordings.CanonicalEvent {
	if len(events) == 0 {
		return nil
	}
	canonical := make([]recordings.CanonicalEvent, len(events))
	for index, event := range events {
		canonical[index] = canonicalEventFromFactory(event)
	}
	return canonical
}

func canonicalEventFromFactory(event factorydefinitions.FactoryEvent) recordings.CanonicalEvent {
	sourceContext, _ := json.Marshal(event.Context)
	scope := recordings.CanonicalEventScope{}
	if event.Context.SessionID != nil {
		scope.FactorySessionID = strings.TrimSpace(*event.Context.SessionID)
	}
	sequence := recordings.CanonicalEventSequence(event.Context.Sequence)
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(event.Id),
		Sequence:    sequence,
		FactoryTick: event.Context.Tick,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: visualizationCanonicalStreamGenerationID,
			Sequence:           sequence,
		},
		RecordedAt:    event.Context.EventTime,
		Kind:          recordings.CanonicalEventKind(event.Type),
		Payload:       string(event.Payload),
		SourceContext: string(sourceContext),
	}
}

func canonicalReconnectCursor(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
) (recordings.CanonicalEventCursor, error) {
	if afterEventID := strings.TrimSpace(cursor.AfterEventID); afterEventID != "" {
		for _, event := range events {
			if event.Id == afterEventID {
				return canonicalEventFromFactory(event).Cursor, nil
			}
		}
		return recordings.CanonicalEventCursor{}, recordings.ErrReconnectCursorNotFound
	}
	if cursor.AfterSequence == nil {
		return recordings.CanonicalEventCursor{}, recordings.ErrInvalidReconnectCursor
	}
	for _, event := range events {
		sequence := event.Context.Sequence
		if event.Context.SessionSequence != nil {
			sequence = *event.Context.SessionSequence
		}
		if sequence == *cursor.AfterSequence {
			return canonicalEventFromFactory(event).Cursor, nil
		}
	}
	return recordings.CanonicalEventCursor{}, recordings.ErrReconnectCursorNotFound
}
