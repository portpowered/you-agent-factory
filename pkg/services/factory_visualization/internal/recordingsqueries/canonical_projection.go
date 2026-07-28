package recordingsqueries

import (
	"encoding/json"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ReconstructWorldStateFromProjection reduces canonical facts through a legacy
// ProjectionService peer and returns a detached world-state view.
func ReconstructWorldStateFromProjection(
	projection recordings.ProjectionService,
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if projection == nil {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	if err := validateProjectionEvents(request.Scope, request.After, request.Events); err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	events := make([]factorydefinitions.FactoryEvent, len(request.Events))
	for index, event := range request.Events {
		events[index] = factoryEventFromCanonical(event)
	}
	state, err := projection.ReconstructFactoryWorldState(events, request.SelectedTick)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	through := recordings.CanonicalEventCursor{}
	if request.After != nil {
		through = *request.After
	}
	if len(request.Events) > 0 {
		through = request.Events[len(request.Events)-1].Cursor
	}
	return recordings.ReconstructWorldStateResult{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         request.Scope,
			Through:       through,
			SelectedTick:  request.SelectedTick,
			Payload:       string(payload),
		},
	}, nil
}

// QuerySimpleDashboardFromProjection returns dashboard render data through a
// legacy ProjectionService peer.
func QuerySimpleDashboardFromProjection(
	projection recordings.ProjectionService,
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	if projection == nil {
		return recordings.SimpleDashboardQueryResult{}, recordings.ErrInvalidProjectionInput
	}
	state, err := DecodeWorldStatePayload(request.WorldState)
	if err != nil {
		return recordings.SimpleDashboardQueryResult{}, err
	}
	return recordings.SimpleDashboardQueryResult{
		Data: projection.SimpleDashboardRenderData(state),
	}, nil
}

// ValidateReconnectReplayFromProjection validates reconnect observe input through
// a legacy ProjectionService peer.
func ValidateReconnectReplayFromProjection(
	projection recordings.ProjectionService,
	request recordings.ValidateReconnectReplayRequest,
) error {
	if projection == nil {
		return recordings.ErrInvalidProjectionInput
	}
	if err := validateReconnectReplayHistory(
		request.Scope,
		request.Cursor,
		request.Events,
	); err != nil {
		return err
	}
	afterSequence := int(request.Cursor.Sequence)
	events := make([]factorydefinitions.FactoryEvent, len(request.Events))
	for index, event := range request.Events {
		events[index] = factoryEventFromCanonical(event)
	}
	return projection.ValidateReconnectReplay(
		events,
		factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &afterSequence},
		factorydefinitions.FactoryEventReconnectScope{SessionID: request.Scope.FactorySessionID},
	)
}

func validateReconnectReplayHistory(
	scope recordings.CanonicalEventScope,
	cursor recordings.CanonicalEventCursor,
	events []recordings.CanonicalEvent,
) error {
	if cursor.StreamGenerationID == "" || cursor.Sequence < 0 {
		return recordings.ErrMalformedProjectionOrder
	}
	if err := validateProjectionEvents(scope, nil, events); err != nil {
		return err
	}
	for _, event := range events {
		if event.Cursor == cursor {
			return nil
		}
	}
	return recordings.ErrReconnectCursorNotFound
}

func validateProjectionEvents(
	scope recordings.CanonicalEventScope,
	after *recordings.CanonicalEventCursor,
	events []recordings.CanonicalEvent,
) error {
	if scope.FactorySessionID != "" && strings.TrimSpace(scope.FactorySessionID) == "" {
		return recordings.ErrInvalidProjectionScope
	}
	expected := recordings.CanonicalEventSequence(0)
	generationID := ""
	if after != nil {
		if after.StreamGenerationID == "" || after.Sequence < 0 {
			return recordings.ErrMalformedProjectionOrder
		}
		expected = after.Sequence + 1
		generationID = after.StreamGenerationID
	}
	previous := expected - 1
	for _, event := range events {
		if err := validateProjectionEvent(
			scope,
			event,
			expected,
			previous,
			generationID,
		); err != nil {
			return err
		}
		generationID = event.Cursor.StreamGenerationID
		previous = event.Sequence
		expected++
	}
	return nil
}

func validateProjectionEvent(
	scope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
	expected recordings.CanonicalEventSequence,
	previous recordings.CanonicalEventSequence,
	generationID string,
) error {
	if event.Scope != scope {
		return recordings.ErrInvalidProjectionScope
	}
	if event.Cursor.Sequence != event.Sequence ||
		event.Cursor.StreamGenerationID == "" {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID == "" && event.Sequence != expected {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID != "" && event.Sequence <= previous {
		return recordings.ErrMalformedProjectionOrder
	}
	if generationID != "" && event.Cursor.StreamGenerationID != generationID {
		return recordings.ErrMalformedProjectionOrder
	}
	return nil
}

func factoryEventFromCanonical(event recordings.CanonicalEvent) factorydefinitions.FactoryEvent {
	context := factorydefinitions.FactoryEventContext{
		EventTime: event.RecordedAt,
		Sequence:  int(event.Sequence),
		Tick:      event.FactoryTick,
	}
	hasSourceContext := json.Valid([]byte(event.SourceContext))
	if hasSourceContext {
		_ = json.Unmarshal([]byte(event.SourceContext), &context)
	}
	var sessionID *string
	if event.Scope.FactorySessionID != "" {
		value := event.Scope.FactorySessionID
		sessionID = &value
	}
	legacy := factorydefinitions.FactoryEvent{
		Context: context,
		Id:      string(event.ID),
		Payload: json.RawMessage(event.Payload),
		Type:    factorydefinitions.FactoryEventType(event.Kind),
	}
	if !hasSourceContext && legacy.Context.SessionID == nil {
		legacy.Context.SessionID = sessionID
	}
	legacy.SchemaVersion = factorydefinitions.FactoryEventSchemaVersionV1
	return legacy
}
