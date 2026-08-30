package canonical

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// CanonicalEventFromFactory maps one legacy Factory event to the detached
// canonical event shape returned by the Recordings append/subscribe slice.
func CanonicalEventFromFactory(
	event factorydefinitions.FactoryEvent,
	generationID string,
) recordings.CanonicalEvent {
	sourceContext, _ := json.Marshal(event.Context)
	scope := recordings.CanonicalEventScope{}
	if event.Context.SessionID != nil {
		scope.FactorySessionID = *event.Context.SessionID
	}
	sequence := recordings.CanonicalEventSequence(event.Context.Sequence)
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(event.Id),
		Sequence:    sequence,
		FactoryTick: event.Context.Tick,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt:    event.Context.EventTime,
		Kind:          recordings.CanonicalEventKind(event.Type),
		Payload:       string(event.Payload),
		SourceContext: string(sourceContext),
	}
}

// FactoryEventFromCanonical maps one detached canonical event to the legacy
// Factory-event shape consumed by projection reducers.
func FactoryEventFromCanonical(event recordings.CanonicalEvent) factorydefinitions.FactoryEvent {
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
	if sessionID != nil {
		// The detached canonical scope is authoritative. SourceContext is
		// retained for correlation metadata, but must not erase the session
		// scope when the event is written back to the legacy artifact shape.
		legacy.Context.SessionID = sessionID
	}
	legacy.SchemaVersion = factorydefinitions.FactoryEventSchemaVersionV1
	return legacy
}
