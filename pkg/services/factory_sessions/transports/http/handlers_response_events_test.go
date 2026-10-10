package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type responseEventsRootFake struct {
	factorysessions.Service
	subscribe func(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error)
}

func (fake responseEventsRootFake) SubscribeResponses(
	ctx context.Context,
	request factorysessions.SessionResponseSubscriptionRequest,
) (factorysessions.SessionResponseSubscriptionResult, error) {
	if fake.subscribe == nil {
		panic("unexpected SubscribeResponses call")
	}
	return fake.subscribe(ctx, request)
}

func blockingResponseEventCursor() *factorysessions.ResponseEventCursor {
	return &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DetachCursor: func() {},
	}
}

func TestGetFactoryResponseEventsBySessionId_CanceledStreamCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{
		subscribe: func(ctx context.Context, request factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
			return factorysessions.SessionResponseSubscriptionResult{Cursor: blockingResponseEventCursor()}, nil
		},
	}, nil, nil, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.GetFactoryResponseEventsBySessionId(recorder, req, "dur-sess-1", factoryapi.GetFactoryResponseEventsBySessionIdParams{})
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetFactoryResponseEventsBySessionId hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented stream outcome", body)
	}
}

func TestGetFactoryResponseEventsBySessionId_DeadlineExceededStreamCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{
		subscribe: func(ctx context.Context, request factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
			return factorysessions.SessionResponseSubscriptionResult{Cursor: blockingResponseEventCursor()}, nil
		},
	}, nil, nil, nil, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.GetFactoryResponseEventsBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil).WithContext(ctx),
			"dur-sess-1",
			factoryapi.GetFactoryResponseEventsBySessionIdParams{},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetFactoryResponseEventsBySessionId hung after request deadline")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty timeout-oriented stream outcome", body)
	}
}

func TestGetFactoryResponseEventsBySessionId_DurableSessionStreamsSSE(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{
		subscribe: func(_ context.Context, request factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
			if request.SessionID != "dur-sess-1" || request.AfterSequence != 2 {
				t.Fatalf("subscribe request = %#v", request)
			}
			called := false
			return factorysessions.SessionResponseSubscriptionResult{Cursor: &factorysessions.ResponseEventCursor{
				NextEvents: func(context.Context) ([]factorysessions.FactoryResponseEvent, error) {
					if called {
						return nil, context.Canceled
					}
					called = true
					return []factorysessions.FactoryResponseEvent{{Sequence: 1, Kind: factorysessions.ResponseEventKindMessage, DispatchID: "dispatch-1"}}, nil
				},
				DetachCursor: func() {},
			}}, nil
		},
	}, nil, nil, nil, zap.NewNop())

	recorder := httptest.NewRecorder()
	afterSequence := factoryapi.ResponseEventAfterSequence(2)
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events?after_sequence=2", nil),
		"dur-sess-1",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{AfterSequence: &afterSequence},
	)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "id: 1") || !strings.Contains(body, `"dispatchId":"dispatch-1"`) {
		t.Fatalf("response = %d %q, want SSE stream with published event", recorder.Code, body)
	}
}

func TestGetFactoryResponseEventsBySessionId_RejectsInvalidAfterSequence(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{}, nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	afterSequence := factoryapi.ResponseEventAfterSequence(-1)
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil),
		"dur-sess-1",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{AfterSequence: &afterSequence},
	)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_RESPONSE_EVENT_CURSOR") {
		t.Fatalf("response = %d %s, want invalid cursor error", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactoryResponseEventsBySessionId_RejectsInvalidKindFilter(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{}, nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	kinds := factoryapi.ResponseEventKind{"NOT_A_KIND"}
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil),
		"dur-sess-1",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{Kind: &kinds},
	)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_RESPONSE_EVENT_FILTER") {
		t.Fatalf("response = %d %s, want invalid filter error", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactoryResponseEventsBySessionId_MapsDurableSessionNotFound(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(responseEventsRootFake{
		subscribe: func(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
			return factorysessions.SessionResponseSubscriptionResult{}, factorysessions.ErrSessionNotFound
		},
	}, nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-missing/response-events", nil),
		"dur-sess-missing",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{},
	)
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "RESPONSE_EVENT_SESSION_NOT_FOUND") {
		t.Fatalf("response = %d %s, want not found", recorder.Code, recorder.Body.String())
	}
}

func TestGetFactoryResponseEventsBySessionId_RequiresSessionsRoot(t *testing.T) {
	t.Parallel()

	handler := NewObservationHandler(nil, nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil),
		"dur-sess-1",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{},
	)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "factory sessions service is unavailable") {
		t.Fatalf("response = %d %s, want unavailable dependency error", recorder.Code, recorder.Body.String())
	}
}
