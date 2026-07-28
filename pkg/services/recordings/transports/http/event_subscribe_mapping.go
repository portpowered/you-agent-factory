package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

var (
	errInvalidEventReconnectCursor = errors.New("invalid event reconnect cursor")
	errInvalidSubscribeScope       = errors.New("invalid subscribe scope")
)

// EventSubscribeInput carries decoded HTTP inputs for one session-scoped event
// subscribe or reconnect probe owned by Recordings.
type EventSubscribeInput struct {
	SessionID          string
	Params             factoryapi.GetEventsBySessionIdParams
	StreamGenerationID string
}

// EventReconnectCursorFromAPI maps public reconnect query parameters into the
// Recordings reconnect cursor vocabulary before root invocation.
func EventReconnectCursorFromAPI(
	params factoryapi.GetEventsBySessionIdParams,
) (*recordings.EventReconnectCursor, error) {
	if params.AfterEventId == nil && params.AfterSequence == nil {
		return nil, nil
	}
	cursor := &recordings.EventReconnectCursor{}
	if params.AfterEventId != nil {
		afterEventID := strings.TrimSpace(string(*params.AfterEventId))
		if afterEventID == "" {
			return nil, fmt.Errorf("%w: after_event_id must not be empty", errInvalidEventReconnectCursor)
		}
		cursor.AfterEventID = afterEventID
	}
	if params.AfterSequence != nil {
		sequence := int(*params.AfterSequence)
		if sequence < 0 {
			return nil, fmt.Errorf("%w: after_sequence must be non-negative", errInvalidEventReconnectCursor)
		}
		cursor.AfterSequence = &sequence
	}
	if cursor.AfterEventID != "" {
		cursor.AfterSequence = nil
	}
	return cursor, nil
}

// SubscribeRequestFromAPI maps one public event subscribe request into the
// accepted Recordings root subscribe request.
func SubscribeRequestFromAPI(input EventSubscribeInput) (recordings.SubscribeRequest, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return recordings.SubscribeRequest{}, errInvalidSubscribeScope
	}
	request := recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
	}
	reconnect, err := EventReconnectCursorFromAPI(input.Params)
	if err != nil {
		return recordings.SubscribeRequest{}, err
	}
	if reconnect == nil {
		return request, nil
	}
	if reconnect.AfterEventID != "" {
		return recordings.SubscribeRequest{}, fmt.Errorf(
			"%w: after_event_id reconnect requires a canonical cursor mapping",
			errInvalidEventReconnectCursor,
		)
	}
	if reconnect.AfterSequence == nil {
		return request, nil
	}
	generationID := strings.TrimSpace(input.StreamGenerationID)
	if generationID == "" {
		return recordings.SubscribeRequest{}, fmt.Errorf(
			"%w: stream generation is required for reconnect",
			errInvalidEventReconnectCursor,
		)
	}
	request.Cursor = &recordings.CanonicalEventCursor{
		StreamGenerationID: generationID,
		Sequence:           recordings.CanonicalEventSequence(*reconnect.AfterSequence),
	}
	return request, nil
}

// FactoryEventToAPI encodes one detached canonical Recordings event into the
// public FactoryEvent transport shape.
func FactoryEventToAPI(event recordings.CanonicalEvent) (factoryapi.FactoryEvent, error) {
	legacy, err := factoryEventFromCanonical(event)
	if err != nil {
		return factoryapi.FactoryEvent{}, err
	}
	return apisurface.FactoryEventToAPI(legacy)
}

func factoryEventFromCanonical(event recordings.CanonicalEvent) (interfaces.FactoryEvent, error) {
	if len(event.SourceContext) > 0 && json.Valid([]byte(event.SourceContext)) {
		var context interfaces.FactoryEventContext
		if err := json.Unmarshal([]byte(event.SourceContext), &context); err == nil {
			return interfaces.FactoryEvent{
				Id:            string(event.ID),
				Type:          interfaces.FactoryEventType(event.Kind),
				Payload:       json.RawMessage(event.Payload),
				Context:       context,
				SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
			}, nil
		}
	}
	context := interfaces.FactoryEventContext{
		EventTime: event.RecordedAt,
		Sequence:  int(event.Sequence),
		Tick:      event.FactoryTick,
	}
	if sessionID := strings.TrimSpace(event.Scope.FactorySessionID); sessionID != "" {
		context.SessionID = &sessionID
	}
	return interfaces.FactoryEvent{
		Id:            string(event.ID),
		Type:          interfaces.FactoryEventType(event.Kind),
		Payload:       json.RawMessage(event.Payload),
		Context:       context,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
	}, nil
}

// EventStreamRecoveryToAPI maps one reconnect probe outcome into the public JSON
// recovery response shape.
func EventStreamRecoveryToAPI(
	sessionID string,
	outcome factoryapi.FactorySessionEventStreamRecoveryOutcome,
	omitReconnectCursor bool,
) factoryapi.FactorySessionEventStreamRecovery {
	return factoryapi.FactorySessionEventStreamRecovery{
		FactorySessionId: sessionID,
		Outcome:          outcome,
		Retry: factoryapi.FactorySessionEventStreamRecoveryRetry{
			OmitAfterEventId:  omitReconnectCursor,
			OmitAfterSequence: omitReconnectCursor,
		},
	}
}
