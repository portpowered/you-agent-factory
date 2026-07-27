package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestGetEventsBySessionId_RejectsInvalidReconnectBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			invoked = true
			return recordings.SubscribeResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()
	afterEventID := factoryapi.AfterEventId("event-1")

	adapter.GetEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events?after_event_id=event-1", nil),
		"session-1",
		factoryapi.GetEventsBySessionIdParams{AfterEventId: &afterEventID},
	)

	if invoked {
		t.Fatal("invalid reconnect cursor must be rejected before Recordings root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestGetEventsBySessionId_EncodesFakeRootHistoryAsSSE(t *testing.T) {
	t.Parallel()

	recordedAt := time.Unix(1_700_000_000, 0).UTC()
	event := recordings.CanonicalEvent{
		ID:          "event-1",
		Sequence:    0,
		FactoryTick: 1,
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           0,
		},
		RecordedAt: recordedAt,
		Kind:       "WORK_REQUEST",
		Payload:    `{"workTypeId":"task"}`,
	}
	adapter := NewAdapter(&rootFake{
		streamGenerationID: "generation-1",
		subscribeFrom: func(_ context.Context, request recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			if request.Scope.FactorySessionID != "session-1" {
				t.Fatalf("scope = %#v, want session-1", request.Scope)
			}
			delivered := false
			return recordings.SubscribeResult{
				Subscription: recordings.EventSubscription(func(context.Context) recordings.SubscriptionOutcome {
					if delivered {
						return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
					}
					delivered = true
					return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event}
				}),
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil),
		"session-1",
		factoryapi.GetEventsBySessionIdParams{},
	)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") ||
		!strings.Contains(body, `"id":"event-1"`) ||
		recorder.Header().Get(SessionEventStreamFactorySessionHeader) != "session-1" {
		t.Fatalf("response = %d headers=%#v body=%s, want encoded SSE history", recorder.Code, recorder.Header(), body)
	}
}

func TestGetEventsBySessionId_ProbeMapsStaleCursorWithoutTreatingFakeRootAsSuccessful(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			invoked = true
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorExpired
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events?after_sequence=4", nil)
	request.Header.Set("Accept", "application/json")
	request.Header.Set(SessionEventStreamGenerationHeader, "generation-1")

	adapter.GetEventsBySessionId(
		recorder,
		request,
		"session-1",
		factoryapi.GetEventsBySessionIdParams{AfterSequence: ptrAfterSequence(4)},
	)

	if !invoked {
		t.Fatal("reconnect probe must invoke the Recordings root")
	}
	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"outcome":"CURSOR_STALE"`) ||
		!strings.Contains(recorder.Body.String(), `"omitAfterSequence":true`) {
		t.Fatalf("response = %d %s, want cursor_stale recovery probe", recorder.Code, recorder.Body.String())
	}
}

func TestGetEventsBySessionId_MapsReconnectQueryIntoFakeRootRequest(t *testing.T) {
	t.Parallel()

	var request recordings.SubscribeRequest
	adapter := NewAdapter(&rootFake{
		streamGenerationID: "generation-1",
		subscribeFrom: func(_ context.Context, got recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			request = got
			return recordings.SubscribeResult{
				Subscription: recordings.EventSubscription(func(context.Context) recordings.SubscriptionOutcome {
					return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
				}),
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events?after_sequence=2", nil)
	req.Header.Set(SessionEventStreamGenerationHeader, "generation-1")

	adapter.GetEventsBySessionId(
		recorder,
		req,
		"session-1",
		factoryapi.GetEventsBySessionIdParams{AfterSequence: ptrAfterSequence(2)},
	)

	if request.Scope.FactorySessionID != "session-1" ||
		request.Cursor == nil ||
		request.Cursor.StreamGenerationID != "generation-1" ||
		request.Cursor.Sequence != 2 {
		t.Fatalf("SubscribeRequest = %#v, want reconnect cursor generation-1/2 for session-1", request)
	}
}
