package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestStreamWorkerSessionEventsByWorkerSessionIDWritesProviderNeutralReplay(t *testing.T) {
	replayOnly := true
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID: "worker-no-reference", WorkIDs: []string{"work-1"}, State: workersessions.StateCompleted,
			RecordingHealth: recordings.WorkerRecordingStatusDegraded, RecordingHealthReason: "PERSISTENCE_FAILED",
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

func TestStreamWorkerSessionEventsByWorkerSessionIDMapsExclusiveCursor(t *testing.T) {
	position := factoryapi.WorkerSessionAfterPosition(7)
	generation := factoryapi.WorkerSessionStreamGenerationID("generation-1")
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1", State: workersessions.StateRunning},
		streamByWorkerSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: false}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StreamWorkerSessionEventsByWorkerSessionId(
		recorder,
		httptest.NewRequest("GET", "/events?after_position=7&stream_generation_id=generation-1", nil),
		factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
		factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{
			AfterPosition:      &position,
			StreamGenerationId: &generation,
		},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.streamByWorkerRequest.Cursor == nil || service.streamByWorkerRequest.Cursor.Position != 7 || service.streamByWorkerRequest.Cursor.StreamGenerationID != "generation-1" {
		t.Fatalf("stream cursor = %#v, want position 7/generation-1", service.streamByWorkerRequest.Cursor)
	}
}

func TestStreamWorkerSessionEventsByWorkerSessionIDRejectsConflictingCursorAliases(t *testing.T) {
	position := factoryapi.WorkerSessionAfterPosition(7)
	sequence := factoryapi.AfterSequence(8)
	service := &fakeObservationService{getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1"}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StreamWorkerSessionEventsByWorkerSessionId(
		recorder, httptest.NewRequest("GET", "/events", nil), factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
		factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{AfterPosition: &position, AfterSequence: &sequence},
	)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.getByWorkerCalled {
		t.Fatal("observation lookup ran before conflicting cursor validation")
	}
}

func TestStreamWorkerSessionEventsByProviderReferenceDelegatesCursor(t *testing.T) {
	position := factoryapi.WorkerSessionAfterPosition(4)
	generation := factoryapi.WorkerSessionStreamGenerationID("generation-1")
	service := &fakeObservationService{
		getResult: workersessions.Observation{
			WorkerSessionID: "worker-1",
			ProviderSession: providers.SessionRef{Provider: "codex", Kind: providers.SessionIDKind, ID: "provider-1"},
		},
		streamSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: false}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StreamWorkerSessionEventsBySessionId(
		recorder,
		httptest.NewRequest("GET", "/events?provider=codex&kind=session_id&id=provider-1&after_position=4&stream_generation_id=generation-1", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.StreamWorkerSessionEventsBySessionIdParams{
			Provider:           factoryapi.LoadableProviderSessionProvider("codex"),
			Kind:               factoryapi.LoadableProviderSessionKind("session_id"),
			Id:                 "provider-1",
			AfterPosition:      &position,
			StreamGenerationId: &generation,
		},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if service.streamRequest.Cursor == nil || service.streamRequest.Cursor.Position != 4 || service.streamRequest.Cursor.StreamGenerationID != "generation-1" {
		t.Fatalf("provider stream cursor = %#v, want position 4/generation-1", service.streamRequest.Cursor)
	}
}

func TestStreamWorkerSessionEventsByWorkerSessionIDMapsTypedCursorFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code factoryapi.ErrorResponseCode
	}{
		{name: "invalid", err: workersessions.ErrInvalidObservationCursor, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORINVALID},
		{name: "foreign", err: workersessions.ErrObservationCursorForeign, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFOREIGN},
		{name: "future", err: workersessions.ErrObservationCursorFuture, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFUTURE},
		{name: "stale", err: workersessions.ErrObservationCursorStale, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORSTALE},
		{name: "unavailable", err: workersessions.ErrObservationCursorUnavailable, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORUNAVAILABLE},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeObservationService{
				getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1"},
				streamByWorkerErr: test.err,
			}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.StreamWorkerSessionEventsByWorkerSessionId(
				recorder, httptest.NewRequest("GET", "/events", nil),
				factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
				factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{},
			)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != test.code {
				t.Fatalf("error code = %q, want %q", response.Code, test.code)
			}
		})
	}
}

func TestStreamWorkerSessionEventsByWorkerSessionIDReplayTerminalCarriesSummary(t *testing.T) {
	replayOnly := true
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{WorkerSessionID: "worker-1", State: workersessions.StateCompleted},
		streamByWorkerSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{Position: 1, Payload: json.RawMessage(`{"step":1}`)}},
			{Kind: workersessions.ObservationDeliveryTerminalReplay, Event: workersessions.ObservationEvent{Position: 2}, Summary: &workersessions.ReplaySummary{Complete: true, EventsEmitted: 2}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StreamWorkerSessionEventsByWorkerSessionId(
		recorder, httptest.NewRequest("GET", "/events?replayOnly=true", nil),
		factoryapi.SessionID("session-1"), factoryapi.WorkerSessionID("worker-1"),
		factoryapi.StreamWorkerSessionEventsByWorkerSessionIdParams{ReplayOnly: &replayOnly},
	)
	frames := decodeSSEFrames(t, recorder.Body.String())
	if len(frames) != 2 || frames[1].Delivery != "TERMINAL_REPLAY" || frames[1].ReplaySummary == nil || !frames[1].ReplaySummary.Complete {
		t.Fatalf("replay terminal frames = %#v, want terminal with complete summary and no failure frame", frames)
	}
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
	if frames[0].FactorySessionID == nil || *frames[0].FactorySessionID != "session-1" {
		t.Fatalf("provider-neutral frame Factory Session scope = %#v, want session-1", frames[0].FactorySessionID)
	}
	if frames[0].RecordingHealth == nil || *frames[0].RecordingHealth != string(recordings.WorkerRecordingStatusDegraded) || frames[0].RecordingHealthReason == nil || *frames[0].RecordingHealthReason != "PERSISTENCE_FAILED" {
		t.Fatalf("provider-neutral frame recording health = %#v/%#v, want DEGRADED/PERSISTENCE_FAILED", frames[0].RecordingHealth, frames[0].RecordingHealthReason)
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

func TestStreamWorkerSessionEventsBySessionIDWritesRetainedAndTerminalFrames(t *testing.T) {
	service := &fakeObservationService{
		getResult: workersessions.Observation{
			WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
			ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
			WorkIDs:         []string{"work-1"}, State: workersessions.StateRunning,
		},
		streamSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{
				Position: 1, SourceType: "worker_session", SourceID: "worker-session-1", SourceSequence: 1,
				SourceEventID: "event-1", SchemaID: "worker_session.started", Payload: json.RawMessage(`{"state":"RUNNING"}`),
			}},
			{Kind: workersessions.ObservationDeliveryTerminal, Event: workersessions.ObservationEvent{
				Position: 2, SourceType: "worker_session", SourceID: "worker-session-1", SourceSequence: 2,
				SourceEventID: "event-2", SchemaID: "worker_session.completed", Payload: json.RawMessage(`{"state":"COMPLETED"}`),
			}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/events?provider=codex&kind=session_id&id=provider-session-1", nil)

	handler.StreamWorkerSessionEventsBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.StreamWorkerSessionEventsBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
	})
	assertRetainedTerminalFrames(t, recorder, service)
}

func TestStreamWorkerSessionEventsBySessionIDReplayOnlyWritesSummaryAndPreservesMode(t *testing.T) {
	replayOnly := true
	service := &fakeObservationService{
		getResult: workersessions.Observation{
			WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
			ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
			WorkIDs:         []string{"work-1"}, State: workersessions.StateRunning,
		},
		streamSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{Position: 1, Payload: json.RawMessage(`{"step":1}`)}},
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: false, Reason: "session-active", EventsEmitted: 1}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/events?provider=codex&kind=session_id&id=provider-session-1&replayOnly=true", nil)

	handler.StreamWorkerSessionEventsBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.StreamWorkerSessionEventsBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1", ReplayOnly: &replayOnly,
	})

	frames := decodeSSEFrames(t, recorder.Body.String())
	if len(frames) != 2 || frames[0].Delivery != "RECORD" || frames[1].Delivery != "REPLAY_SUMMARY" {
		t.Fatalf("frames = %#v, want RECORD then REPLAY_SUMMARY", frames)
	}
	if frames[1].ReplaySummary == nil || frames[1].ReplaySummary.Kind != "replay-summary" || frames[1].ReplaySummary.Complete || frames[1].ReplaySummary.Reason != "session-active" || frames[1].ReplaySummary.EventsEmitted != 1 {
		t.Fatalf("replay summary = %#v, want active count-one summary", frames[1].ReplaySummary)
	}
	if !service.streamRequest.ReplayOnly {
		t.Fatal("adapter did not preserve replay-only mode in the Worker Sessions request")
	}
}

func TestStreamWorkerSessionEventsBySessionIDWritesExplicitSourceFailure(t *testing.T) {
	service := &fakeObservationService{
		getResult: workersessions.Observation{
			WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
			ProviderSession: providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "cursor-session-1"},
		},
		streamSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceGap},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/events", nil)

	handler.StreamWorkerSessionEventsBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.StreamWorkerSessionEventsBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("cursor"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "cursor-session-1",
	})

	frames := decodeSSEFrames(t, recorder.Body.String())
	if len(frames) != 1 || frames[0].Delivery != "SOURCE_FAILURE" || frames[0].Event != nil {
		t.Fatalf("frames = %#v, want one source failure without an event", frames)
	}
	if frames[0].ErrorCode == nil || *frames[0].ErrorCode != "WORKER_SESSION_STREAM_GAP" {
		t.Fatalf("error code = %#v, want WORKER_SESSION_STREAM_GAP", frames[0].ErrorCode)
	}
	if frames[0].ErrorMessage == nil || !strings.Contains(*frames[0].ErrorMessage, "retained") {
		t.Fatalf("error message = %#v, want safe retained-history message", frames[0].ErrorMessage)
	}
}

func TestStreamWorkerSessionEventsBySessionIDMapsUnavailableBeforeOpening(t *testing.T) {
	service := &fakeObservationService{getResult: workersessions.Observation{
		WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
		ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
	}, streamErr: workersessions.ErrObservationSourceUnavailable}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/events", nil)

	handler.StreamWorkerSessionEventsBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.StreamWorkerSessionEventsBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
	})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeWORKERSESSIONSTREAMUNAVAILABLE {
		t.Fatalf("error code = %q, want WORKER_SESSION_STREAM_UNAVAILABLE", response.Code)
	}
}

func assertListObservationTurnUsage(t *testing.T, observation factoryapi.WorkerSessionObservation, turnCount, finalContext, peakContext int) {
	t.Helper()
	if observation.TurnUsage == nil || observation.TurnUsage.TurnCount != turnCount ||
		observation.TurnUsage.FinalContextTokens != finalContext || observation.TurnUsage.PeakContextTokens != peakContext {
		t.Fatalf("turn usage = %#v, want count/final/peak %d/%d/%d", observation.TurnUsage, turnCount, finalContext, peakContext)
	}
}

func assertFailureObservationTurnUsage(t *testing.T, response factoryapi.WorkerSessionObservation) {
	t.Helper()
	if response.TurnUsage == nil || response.TurnUsage.TurnCount != 3 || response.TurnUsage.FinalContextTokens != 450 || response.TurnUsage.PeakContextTokens != 450 {
		t.Fatalf("turn usage = %#v, want count/final/peak 3/450/450", response.TurnUsage)
	}
}

func assertRetainedTerminalFrames(t *testing.T, recorder *httptest.ResponseRecorder, service *fakeObservationService) {
	t.Helper()
	assertSSESuccess(t, recorder)
	frames := decodeSSEFrames(t, recorder.Body.String())
	assertSSEFrameKinds(t, frames)
	assertSSEFrameIdentity(t, frames[0])
	assertSSEFramePayload(t, frames[0])
	assertSSESubscriptionClosed(t, service)
}

func assertSSESuccess(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
}

func assertSSEFrameKinds(t *testing.T, frames []sseTestFrame) {
	t.Helper()
	if len(frames) != 2 || frames[0].Delivery != "RECORD" || frames[1].Delivery != "TERMINAL" {
		t.Fatalf("frames = %#v, want RECORD then TERMINAL", frames)
	}
}

func assertSSEFrameIdentity(t *testing.T, frame sseTestFrame) {
	t.Helper()
	if frame.WorkerSessionID != "worker-session-1" || frame.ProviderSession == nil || frame.ProviderSession.Id != "provider-session-1" {
		t.Fatalf("frame identity = %#v, want exact worker/provider identity", frame)
	}
}

func assertSSEFramePayload(t *testing.T, frame sseTestFrame) {
	t.Helper()
	if frame.Event == nil || frame.Event.Position != 1 || string(frame.Event.Payload) != `{"state":"RUNNING"}` {
		t.Fatalf("first event = %#v, want canonical event payload", frame.Event)
	}
	if frame.Event.Cursor == nil || frame.Event.Cursor.Position != 1 {
		t.Fatalf("first event cursor = %#v, want position 1", frame.Event.Cursor)
	}
}

func assertSSESubscriptionClosed(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if service.streamSubscription == nil || !service.streamSubscription.closed {
		t.Fatal("stream subscription was not closed after terminal delivery")
	}
	if service.streamRequest.Limit != workersessions.DefaultObservationStreamLimit {
		t.Fatalf("stream limit = %d, want stable default %d", service.streamRequest.Limit, workersessions.DefaultObservationStreamLimit)
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
