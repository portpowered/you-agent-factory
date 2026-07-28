package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type durableResponseEventsProjectionFake struct {
	apisurface.DurableSessionProjectionAPI
	subscribe func(context.Context, factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error)
}

func (fake durableResponseEventsProjectionFake) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (apisurface.FactoryResponseEventSubscription, error) {
	if fake.subscribe == nil {
		panic("unexpected SubscribeDurableFactoryResponseEvents call")
	}
	return fake.subscribe(ctx, request)
}

type staticResponseEventSubscription struct {
	records []apisurface.FactoryResponseEventRecord
	index   int
}

func (s *staticResponseEventSubscription) Next(context.Context) ([]apisurface.FactoryResponseEventRecord, error) {
	if s.index >= len(s.records) {
		return nil, context.Canceled
	}
	batch := s.records[s.index:]
	s.index = len(s.records)
	return batch, nil
}

func (s *staticResponseEventSubscription) Detach() {}

func TestGetFactoryResponseEventsBySessionId_CanceledStreamCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{
		DurableProjection: durableResponseEventsProjectionFake{
			subscribe: func(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error) {
				return &blockingResponseEventSubscription{}, nil
			},
		},
	}, zap.NewNop())

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

	handler := NewHandler(Dependencies{
		DurableProjection: durableResponseEventsProjectionFake{
			subscribe: func(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error) {
				return &blockingResponseEventSubscription{}, nil
			},
		},
	}, zap.NewNop())

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

type blockingResponseEventSubscription struct{}

func (blockingResponseEventSubscription) Next(ctx context.Context) ([]apisurface.FactoryResponseEventRecord, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingResponseEventSubscription) Detach() {}

func TestGetFactoryResponseEventsBySessionId_DurableSessionStreamsSSE(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(factorysessions.FactoryResponseEvent{
		Sequence:   1,
		Kind:       factorysessions.ResponseEventKindMessage,
		DispatchID: "dispatch-1",
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	handler := NewHandler(Dependencies{
		DurableProjection: durableResponseEventsProjectionFake{
			subscribe: func(_ context.Context, request factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error) {
				if request.SessionID != "dur-sess-1" || request.AfterSequence != 2 {
					t.Fatalf("subscribe request = %#v", request)
				}
				return &staticResponseEventSubscription{records: []apisurface.FactoryResponseEventRecord{{
					Sequence: 1,
					Kind:     string(factorysessions.ResponseEventKindMessage),
					Data:     payload,
				}}}, nil
			},
		},
	}, zap.NewNop())

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

	handler := NewHandler(Dependencies{DurableProjection: durableResponseEventsProjectionFake{}}, zap.NewNop())
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

	handler := NewHandler(Dependencies{DurableProjection: durableResponseEventsProjectionFake{}}, zap.NewNop())
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

	handler := NewHandler(Dependencies{
		DurableProjection: durableResponseEventsProjectionFake{
			subscribe: func(context.Context, factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error) {
				return nil, apisurface.ErrFactorySessionNotFound
			},
		},
	}, zap.NewNop())
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

func TestGetFactoryResponseEventsBySessionId_RequiresDurableProjection(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetFactoryResponseEventsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/dur-sess-1/response-events", nil),
		"dur-sess-1",
		factoryapi.GetFactoryResponseEventsBySessionIdParams{},
	)
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "response-event replay is unavailable") {
		t.Fatalf("response = %d %s, want unavailable dependency error", recorder.Code, recorder.Body.String())
	}
}
