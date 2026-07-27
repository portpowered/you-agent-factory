package canonical

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

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
	if !hasSourceContext && legacy.Context.SessionID == nil {
		legacy.Context.SessionID = sessionID
	}
	legacy.SchemaVersion = factorydefinitions.FactoryEventSchemaVersionV1
	return legacy
}
