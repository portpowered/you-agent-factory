package http

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSubscribeRequestFromAPI_MapsFreshSubscribeScope(t *testing.T) {
	t.Parallel()

	request, err := SubscribeRequestFromAPI(EventSubscribeInput{
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("SubscribeRequestFromAPI: %v", err)
	}
	if request.Scope.FactorySessionID != "session-1" || request.Cursor != nil {
		t.Fatalf("request = %#v, want scoped fresh subscribe", request)
	}
}

func TestSubscribeRequestFromAPI_MapsAfterSequenceReconnectCursor(t *testing.T) {
	t.Parallel()

	sequence := factoryapi.AfterSequence(4)
	request, err := SubscribeRequestFromAPI(EventSubscribeInput{
		SessionID: "session-1",
		Params: factoryapi.GetEventsBySessionIdParams{
			AfterSequence: &sequence,
		},
		StreamGenerationID: "generation-1",
	})
	if err != nil {
		t.Fatalf("SubscribeRequestFromAPI: %v", err)
	}
	if request.Cursor == nil {
		t.Fatal("cursor must be populated for reconnect")
	}
	if request.Cursor.StreamGenerationID != "generation-1" || request.Cursor.Sequence != 4 {
		t.Fatalf("cursor = %#v, want generation-1/4", request.Cursor)
	}
}

func TestSubscribeRequestFromAPI_RejectsMalformedReconnectBeforeRoot(t *testing.T) {
	t.Parallel()

	emptyEventID := factoryapi.AfterEventId("   ")
	negativeSequence := factoryapi.AfterSequence(-1)
	tests := []struct {
		name  string
		input EventSubscribeInput
	}{
		{
			name: "whitespace session scope",
			input: EventSubscribeInput{
				SessionID: "   ",
			},
		},
		{
			name: "empty after_event_id",
			input: EventSubscribeInput{
				SessionID: "session-1",
				Params: factoryapi.GetEventsBySessionIdParams{
					AfterEventId: &emptyEventID,
				},
			},
		},
		{
			name: "negative after_sequence",
			input: EventSubscribeInput{
				SessionID: "session-1",
				Params: factoryapi.GetEventsBySessionIdParams{
					AfterSequence: &negativeSequence,
				},
			},
		},
		{
			name: "after_sequence without stream generation",
			input: EventSubscribeInput{
				SessionID: "session-1",
				Params: factoryapi.GetEventsBySessionIdParams{
					AfterSequence: ptrAfterSequence(2),
				},
			},
		},
		{
			name: "after_event_id without canonical mapping",
			input: EventSubscribeInput{
				SessionID: "session-1",
				Params: factoryapi.GetEventsBySessionIdParams{
					AfterEventId: ptrAfterEventID("event-1"),
				},
				StreamGenerationID: "generation-1",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := SubscribeRequestFromAPI(test.input); err == nil {
				t.Fatal("SubscribeRequestFromAPI must reject malformed reconnect input before root invocation")
			}
		})
	}
}

func TestFactoryEventToAPI_EncodesDetachedCanonicalEvent(t *testing.T) {
	t.Parallel()

	recordedAt := time.Unix(1_700_000_000, 0).UTC()
	apiEvent, err := FactoryEventToAPI(recordings.CanonicalEvent{
		ID:          "event-1",
		Sequence:    4,
		FactoryTick: 7,
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		RecordedAt:  recordedAt,
		Kind:        "WORK_REQUEST",
		Payload:     `{"workTypeId":"task"}`,
	})
	if err != nil {
		t.Fatalf("FactoryEventToAPI: %v", err)
	}
	if apiEvent.Id != "event-1" || apiEvent.Type != factoryapi.FactoryEventTypeWorkRequest {
		t.Fatalf("api event = %#v, want encoded WORK_REQUEST event-1", apiEvent)
	}
}

func ptrAfterEventID(value string) *factoryapi.AfterEventId {
	typed := factoryapi.AfterEventId(value)
	return &typed
}

func ptrAfterSequence(value int) *factoryapi.AfterSequence {
	typed := factoryapi.AfterSequence(value)
	return &typed
}
