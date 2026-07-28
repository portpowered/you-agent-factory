package recordingmcp

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// AppendEventInput is the MCP request shape for you.recording.append_event.
type AppendEventInput struct {
	Event canonicalEventInput `json:"event"`
}

type canonicalEventInput struct {
	ID            string                     `json:"id"`
	Sequence      int64                      `json:"sequence,omitempty"`
	FactoryTick   int                        `json:"factoryTick,omitempty"`
	Scope         canonicalEventScopeInput   `json:"scope,omitempty"`
	Cursor        *canonicalEventCursorInput `json:"cursor,omitempty"`
	RecordedAt    string                     `json:"recordedAt"`
	Kind          string                     `json:"kind"`
	Payload       string                     `json:"payload"`
	SourceContext string                     `json:"sourceContext,omitempty"`
}

type canonicalEventScopeInput struct {
	FactorySessionID string `json:"factorySessionId"`
}

type canonicalEventCursorInput struct {
	StreamGenerationID string `json:"streamGenerationId"`
	Sequence           int64  `json:"sequence"`
}

func (input canonicalEventInput) toCanonicalEvent() (recordings.CanonicalEvent, error) {
	recordedAt, err := time.Parse(time.RFC3339Nano, input.RecordedAt)
	if err != nil {
		recordedAt, err = time.Parse(time.RFC3339, input.RecordedAt)
		if err != nil {
			return recordings.CanonicalEvent{}, err
		}
	}
	event := recordings.CanonicalEvent{
		ID:            recordings.CanonicalEventID(input.ID),
		Sequence:      recordings.CanonicalEventSequence(input.Sequence),
		FactoryTick:   input.FactoryTick,
		Scope:         recordings.CanonicalEventScope{FactorySessionID: input.Scope.FactorySessionID},
		RecordedAt:    recordedAt.UTC(),
		Kind:          recordings.CanonicalEventKind(input.Kind),
		Payload:       input.Payload,
		SourceContext: input.SourceContext,
	}
	if input.Cursor != nil {
		event.Cursor = recordings.CanonicalEventCursor{
			StreamGenerationID: input.Cursor.StreamGenerationID,
			Sequence:           recordings.CanonicalEventSequence(input.Cursor.Sequence),
		}
	}
	return event, nil
}

// AppendEvent appends one canonical Factory Event through the
// you.recording.append_event MCP tool.
func AppendEvent(
	ctx context.Context,
	service recordings.Service,
	input AppendEventInput,
) ToolResponse[recordings.AppendRecordedEventResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope("", errMissingRequestContext)
		return ToolResponse[recordings.AppendRecordedEventResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.AppendRecordedEventResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.AppendRecordedEventResult]{Error: &envelope}
	}

	event, err := input.Event.toCanonicalEvent()
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode append event input", err)
		return ToolResponse[recordings.AppendRecordedEventResult]{Error: &envelope}
	}

	result, err := service.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		envelope := appendEventErrorEnvelope(event, err)
		return ToolResponse[recordings.AppendRecordedEventResult]{Error: &envelope}
	}
	return ToolResponse[recordings.AppendRecordedEventResult]{Result: &result}
}
