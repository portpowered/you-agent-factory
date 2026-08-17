package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
				RetainedEventCount: 1,
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
		recorder.Header().Get(SessionEventStreamFactorySessionHeader) != "session-1" ||
		recorder.Header().Get(factorysessions.SessionEventStreamRetainedCountHeader) != "1" {
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

func TestGetEventsBySessionId_MapsTypedReconnectErrorsToBadRequest(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			return recordings.SubscribeResult{}, recordings.ErrInvalidReconnectCursor
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
	if recorder.Code != http.StatusBadRequest ||
		!strings.Contains(body, `"code":"BAD_REQUEST"`) ||
		!strings.Contains(body, `"family":"BAD_REQUEST"`) ||
		!strings.Contains(body, `"message":"invalid event reconnect cursor"`) {
		t.Fatalf("response = %d %s, want typed bad request ErrorResponse", recorder.Code, body)
	}
	if strings.Contains(body, "pkg/services/recordings") {
		t.Fatalf("response leaked internal package path: %s", body)
	}
}

func TestGetEventsBySessionId_SanitizesUnmappedRootFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		subscribeFrom: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			return recordings.SubscribeResult{}, errors.New("pkg/services/recordings/internal/service: boom")
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
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"message":"failed to subscribe to factory events"`) {
		t.Fatalf("response = %d %s, want sanitized internal ErrorResponse", recorder.Code, body)
	}
	if strings.Contains(body, "pkg/services/recordings") || strings.Contains(body, "boom") {
		t.Fatalf("response leaked internal failure details: %s", body)
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

func TestGetEventsBySessionId_DurableCompatibilityPrefersDurableHistoryOverLive(t *testing.T) {
	t.Parallel()

	const sessionID = "dur-sess-live-compat-http-001"
	liveCalls := 0
	durableCalls := 0
	events := make(chan interfaces.FactoryEvent)
	close(events)
	adapter := NewAdapterWithLegacyFallback(
		&rootFake{
			queryRecordingStatus: func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
				return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
					RecordingID: recordings.RecordingID(sessionID),
				}}, nil
			},
		},
		&legacyHistoryFake{
			events: func(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
				durableCalls++
				return &interfaces.FactoryEventStream{
					FactorySessionID: sessionID,
					History:          []interfaces.FactoryEvent{{Id: "durable-event-1", Type: interfaces.FactoryEventTypeWorkRequest}},
					Events:           events,
				}, nil
			},
		},
		nil,
		&legacyLiveEventsFake{
			subscribe: func(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
				liveCalls++
				return nil, factorysessions.ErrSessionNotFound
			},
		},
	)
	recorder := httptest.NewRecorder()

	adapter.GetEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/events", nil),
		factoryapi.SessionID(sessionID),
		factoryapi.GetEventsBySessionIdParams{},
	)

	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"id":"durable-event-1"`) ||
		durableCalls != 1 || liveCalls != 0 {
		t.Fatalf("response = %d %s, durableCalls=%d liveCalls=%d, want durable compatibility history only", recorder.Code, recorder.Body.String(), durableCalls, liveCalls)
	}
}

func TestGetEventsBySessionId_MapsLegacyRecordingsCursorErrorToBadRequest(t *testing.T) {
	t.Parallel()

	adapter := NewAdapterWithLegacyFallback(
		nil,
		nil,
		nil,
		&legacyLiveEventsFake{
			subscribe: func(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
				return nil, recordings.ErrReconnectCursorNotFound
			},
		},
	)
	recorder := httptest.NewRecorder()

	adapter.GetEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/events", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.GetEventsBySessionIdParams{},
	)

	if recorder.Code != http.StatusBadRequest ||
		!strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) ||
		!strings.Contains(recorder.Body.String(), `"message":"invalid event reconnect cursor"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestGetEventsBySessionId_LiveCompatibilityProbeMapsCursorAndReadyOutcome(t *testing.T) {
	t.Parallel()

	var gotCursor *interfaces.FactoryEventReconnectCursor
	adapter := NewAdapterWithLegacyFallback(nil, nil, nil, &legacyLiveEventsFake{
		probe: func(_ context.Context, sessionID string, cursor *interfaces.FactoryEventReconnectCursor) error {
			if sessionID != "session-live-probe-001" {
				t.Fatalf("probe sessionID = %q, want session-live-probe-001", sessionID)
			}
			gotCursor = cursor
			return nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-live-probe-001/events", nil)
	request.Header.Set("Accept", "application/json")
	afterSequence := factoryapi.AfterSequence(5)
	recorder := httptest.NewRecorder()
	adapter.GetEventsBySessionId(
		recorder, request, factoryapi.SessionID("session-live-probe-001"),
		factoryapi.GetEventsBySessionIdParams{AfterSequence: &afterSequence},
	)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"outcome":"STREAM_READY"`) {
		t.Fatalf("live probe = %d %s, want stream-ready outcome", recorder.Code, recorder.Body.String())
	}
	if gotCursor == nil || gotCursor.AfterSequence == nil || *gotCursor.AfterSequence != 5 {
		t.Fatalf("live reconnect cursor = %#v, want afterSequence=5", gotCursor)
	}
}

type legacyLiveEventsFake struct {
	subscribe func(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	probe     func(context.Context, string, *interfaces.FactoryEventReconnectCursor) error
}

func (fake *legacyLiveEventsFake) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, cursor *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return fake.subscribe(ctx, sessionID, cursor)
}

func (fake *legacyLiveEventsFake) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, cursor *interfaces.FactoryEventReconnectCursor) error {
	if fake.probe == nil {
		return nil
	}
	return fake.probe(ctx, sessionID, cursor)
}

func TestLegacyFactoryEventWriterProjectsProviderSession(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(map[string]any{
		"attempt":            1,
		"continuation":       providers.ContinuationRef{Provider: "antigravity", Kind: providers.SessionIDKind, ProviderSessionID: "provider-session-1"},
		"durationMillis":     12,
		"inferenceRequestId": "inference-request-1",
		"outcome":            "SUCCEEDED",
		"response":           "ok",
	})
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	event := interfaces.FactoryEvent{
		Id:            "event-inference-response-1",
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeInferenceResponse,
	}
	recorder := httptest.NewRecorder()

	if err := legacyFactoryEventWriter(recorder)(event); err != nil {
		t.Fatalf("legacyFactoryEventWriter: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"providerSession":{"provider":"antigravity","kind":"session_id","id":"provider-session-1"}`) {
		t.Fatalf("SSE body = %s, want projected provider session", body)
	}
	if strings.Contains(body, `"continuation"`) {
		t.Fatalf("SSE body = %s, must not expose the worker continuation", body)
	}
}
