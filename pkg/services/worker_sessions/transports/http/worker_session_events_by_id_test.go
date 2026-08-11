package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestStreamWorkerSessionEventsByWorkerSessionIDWritesProviderNeutralReplay(t *testing.T) {
	replayOnly := true
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID: "worker-no-reference", WorkIDs: []string{"work-1"}, State: workersessions.StateCompleted,
		},
		streamByWorkerSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{
				Position: 1, SourceType: "worker_session_lifecycle", SourceID: "worker-no-reference", SourceSequence: 1,
				SourceEventID: "opening", SchemaID: "worker_session.started", Payload: json.RawMessage(`{"state":"STARTING"}`),
			}},
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: true, EventsEmitted: 1}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/worker-no-reference/events?replayOnly=true", nil)

	handler.StreamWorkerSessionEventsByWorkerSessionId(
		recorder, request, factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-no-reference"),
		factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{ReplayOnly: &replayOnly},
	)

	assertProviderNeutralReplayResponse(t, recorder)
	assertProviderNeutralReplayFrames(t, decodeSSEFrames(t, recorder.Body.String()))
	assertProviderNeutralReplayServiceCalls(t, service)
}

func assertProviderNeutralReplayResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want SSE 200", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response content-type = %q, want text/event-stream", recorder.Header().Get("Content-Type"))
	}
}

func assertProviderNeutralReplayFrames(t *testing.T, frames []sseTestFrame) {
	t.Helper()
	if len(frames) != 2 {
		t.Fatalf("frames = %#v, want one record and one replay summary", frames)
	}
	if frames[0].Delivery != "RECORD" {
		t.Fatalf("first frame delivery = %q, want RECORD", frames[0].Delivery)
	}
	if frames[1].Delivery != "REPLAY_SUMMARY" {
		t.Fatalf("second frame delivery = %q, want REPLAY_SUMMARY", frames[1].Delivery)
	}
	if frames[0].WorkerSessionID != "worker-no-reference" {
		t.Fatalf("provider-neutral frame identity = %#v, want Worker Session identity", frames[0])
	}
	if frames[0].ProviderSession == nil {
		t.Fatalf("provider-neutral frame = %#v, want empty provider envelope", frames[0])
	}
	if frames[1].ReplaySummary == nil {
		t.Fatalf("replay summary = %#v, want complete one-event summary", frames[1].ReplaySummary)
	}
	if !frames[1].ReplaySummary.Complete {
		t.Fatalf("replay summary = %#v, want complete one-event summary", frames[1].ReplaySummary)
	}
	if frames[1].ReplaySummary.EventsEmitted != 1 {
		t.Fatalf("replay summary = %#v, want one emitted event", frames[1].ReplaySummary)
	}
}

func assertProviderNeutralReplayServiceCalls(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if !service.getByWorkerCalled {
		t.Fatal("Worker Session lookup was not called")
	}
	if service.getWorkerSessionID != "worker-no-reference" {
		t.Fatalf("Worker Session lookup id = %q, want canonical identity", service.getWorkerSessionID)
	}
	if service.streamByWorkerRequest.WorkerSessionID != "worker-no-reference" {
		t.Fatalf("Worker Session stream id = %q, want canonical identity", service.streamByWorkerRequest.WorkerSessionID)
	}
	if !service.streamByWorkerRequest.ReplayOnly {
		t.Fatal("Worker Session stream request was not replay-only")
	}
	if service.streamByWorkerRequest.Limit != workersessions.DefaultObservationStreamLimit {
		t.Fatalf("Worker Session stream limit = %d, want bounded replay limit", service.streamByWorkerRequest.Limit)
	}
	if !service.streamByWorkerSubscription.closed {
		t.Fatal("Worker Session stream subscription was not closed")
	}
}

func TestStreamWorkerSessionEventsByWorkerSessionIDMapsServiceFailures(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		getErr    error
		streamErr error
		code      factoryapi.ErrorResponseCode
		status    int
	}{
		{name: "missing observation", getErr: workersessions.ErrObservationSessionNotFound, code: factoryapi.ErrorResponseCodeNOTFOUND, status: http.StatusNotFound},
		{name: "unavailable stream", streamErr: workersessions.ErrObservationSourceUnavailable, code: factoryapi.ErrorResponseCodeWORKERSESSIONSTREAMUNAVAILABLE, status: http.StatusInternalServerError},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{
				getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1"},
				getByWorkerErr:    testCase.getErr, streamByWorkerErr: testCase.streamErr,
			}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.StreamWorkerSessionEventsByWorkerSessionId(
				recorder, httptest.NewRequest("GET", "/events", nil), factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
				factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{},
			)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.status, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code {
				t.Fatalf("error code = %q, want %q", response.Code, testCase.code)
			}
		})
	}
}

func TestStreamWorkerSessionEventsByWorkerSessionIDRejectsInvalidRequests(t *testing.T) {
	handler := NewHandler(NewAdapter(&fakeObservationService{}, workServiceStub{}), zap.NewNop())
	for _, testCase := range []struct {
		name            string
		sessionID       factoryapi.SessionID
		workerSessionID factoryapi.WorkerSessionID
		request         *http.Request
	}{
		{name: "session id", workerSessionID: "worker-1", request: httptest.NewRequest("GET", "/events", nil)},
		{name: "worker session id", sessionID: "session-1", request: httptest.NewRequest("GET", "/events", nil)},
		{name: "request", sessionID: "session-1", workerSessionID: "worker-1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.StreamWorkerSessionEventsByWorkerSessionId(recorder, testCase.request, testCase.sessionID, testCase.workerSessionID, factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	var nilHandler *Handler
	recorder := httptest.NewRecorder()
	nilHandler.StreamWorkerSessionEventsByWorkerSessionId(recorder, httptest.NewRequest("GET", "/events", nil), "session-1", "worker-1", factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("nil handler status = %d, want 500", recorder.Code)
	}

	noFlusher := &responseWriterWithoutFlusher{}
	handler.StreamWorkerSessionEventsByWorkerSessionId(noFlusher, httptest.NewRequest("GET", "/events", nil), "session-1", "worker-1", factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{})
	if noFlusher.status != http.StatusInternalServerError {
		t.Fatalf("non-flusher status = %d, want 500", noFlusher.status)
	}
}

func TestAdapterStreamWorkerSessionEventsByWorkerSessionIDValidatesBoundary(t *testing.T) {
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1"},
		streamByWorkerSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: true}},
		}},
	}
	adapter := NewAdapter(service, workServiceStub{})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var nilAdapter *Adapter
	for _, testCase := range []struct {
		name            string
		adapter         *Adapter
		ctx             context.Context
		sessionID       string
		workerSessionID string
		want            error
	}{
		{name: "missing service", adapter: nilAdapter, sessionID: "session-1", workerSessionID: "worker-1", want: errors.New("Worker Sessions service is required")},
		{name: "missing session", adapter: adapter, sessionID: "", workerSessionID: "worker-1", want: errors.New("session id is required")},
		{name: "missing worker session", adapter: adapter, sessionID: "session-1", want: errors.New("worker session id is required")},
		{name: "canceled", adapter: adapter, ctx: canceled, sessionID: "session-1", workerSessionID: "worker-1", want: context.Canceled},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := testCase.adapter.StreamWorkerSessionEventsByWorkerSessionID(testCase.ctx, testCase.sessionID, testCase.workerSessionID, false)
			if err == nil || err.Error() != testCase.want.Error() {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}

	response, subscription, err := adapter.StreamWorkerSessionEventsByWorkerSessionID(nil, " session-1 ", " worker-1 ", true)
	if err != nil || response.WorkerSessionId != "worker-1" || subscription.NextFunc == nil {
		t.Fatalf("nil-context stream = response %#v subscription %#v error %v, want normalized success", response, subscription, err)
	}
	subscription.Close()

	service.streamByWorkerNil = true
	if _, _, err := adapter.StreamWorkerSessionEventsByWorkerSessionID(context.Background(), "session-1", "worker-1", false); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("nil subscription error = %v, want %v", err, workersessions.ErrObservationSourceUnavailable)
	}
}

type responseWriterWithoutFlusher struct {
	header http.Header
	body   strings.Builder
	status int
}

func (w *responseWriterWithoutFlusher) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *responseWriterWithoutFlusher) WriteHeader(status int) {
	w.status = status
}

func (w *responseWriterWithoutFlusher) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(payload)
}
