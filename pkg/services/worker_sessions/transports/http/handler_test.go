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

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestListWorkerSessionsBySessionIDProjectsPopulatedObservation(t *testing.T) {
	total := 17
	duration := 2500 * time.Millisecond
	turnCount, finalContext, peakContext := 3, 450, 450
	observations := []workersessions.Observation{
		{
			WorkerSessionID:          "worker-session-1",
			Model:                    stringPtr("gpt-5.6-luna"),
			ReasoningEffort:          stringPtr("high"),
			ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
			ProviderSessionAvailable: true,
			WorkIDs:                  []string{"work-1"},
			TurnID:                   "turn-1",
			AttemptID:                "attempt-1",
			State:                    workersessions.StateCompleted,
			Duration:                 &duration,
			DurationBasis:            workersessions.DurationBasisRecordedTimestamps,
			TokenUsage:               &workersessions.TokenUsage{TotalTokens: &total},
			TurnUsage:                &workersessions.TurnUsage{TurnCount: turnCount, FinalContextTokens: finalContext, PeakContextTokens: peakContext},
			Transcript:               workersessions.TranscriptAvailabilityAvailable,
		},
	}
	service := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: observations}}
	handler := NewHandler(NewAdapter(service, workServiceStub{getResult: work.ReadModel{WorkID: "work-1", Name: "Review Work"}}), zap.NewNop())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=work-1", nil)
	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertPopulatedListResponse(t, recorder.Body.Bytes(), total)
}

// TestListWorkerSessionsBySessionIDPreservesBoundedTerminalReasons covers the
// API half of the operator diagnosis path. Every bounded reason must survive
// the boundary paired with the state it ended in, including the operator
// control outcomes: before those existed a canceled session arrived with a null
// failure, which reads exactly like a cause that was never recorded.
func TestListWorkerSessionsBySessionIDPreservesBoundedTerminalReasons(t *testing.T) {
	reasons := []struct {
		state workersessions.State
		kind  workersessions.FailureCauseKind
	}{
		{state: workersessions.StateFailed, kind: workersessions.FailureCauseRejected},
		{state: workersessions.StateFailed, kind: workersessions.FailureCauseIncompleteOutput},
		{state: workersessions.StateFailed, kind: workersessions.FailureCauseWorkersExecutionFailure},
		{state: workersessions.StateFailed, kind: workersessions.FailureCauseProcessGone},
		{state: workersessions.StateFailed, kind: workersessions.FailureCauseTimeout},
		{state: workersessions.StateCanceled, kind: workersessions.FailureCauseOperatorCanceled},
		{state: workersessions.StateTerminated, kind: workersessions.FailureCauseOperatorTerminated},
	}
	observations := make([]workersessions.Observation, 0, len(reasons))
	for _, reason := range reasons {
		observations = append(observations, workersessions.Observation{
			WorkerSessionID: "worker-session-" + string(reason.kind),
			AttemptID:       "attempt-" + string(reason.kind),
			WorkIDs:         []string{"work-1"},
			State:           reason.state,
			DurationBasis:   workersessions.DurationBasisRecordedTimestamps,
			Transcript:      workersessions.TranscriptAvailabilityUnavailable,
			Failure:         &workersessions.FailureCause{Kind: reason.kind, Detail: "safe bounded detail"},
		})
	}
	service := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: observations}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != len(reasons) {
		t.Fatalf("session count = %d, want %d", len(response.Sessions), len(reasons))
	}
	byWorkerSessionID := make(map[string]factoryapi.WorkerSessionObservation, len(response.Sessions))
	for _, session := range response.Sessions {
		byWorkerSessionID[session.WorkerSessionId] = session
	}
	for _, reason := range reasons {
		session, ok := byWorkerSessionID["worker-session-"+string(reason.kind)]
		if !ok {
			t.Fatalf("response is missing the %q session", reason.kind)
		}
		if session.Failure == nil {
			t.Fatalf("session ended by %q reports no named reason", reason.kind)
		}
		if session.Failure.Kind != string(reason.kind) || string(session.State) != string(reason.state) {
			t.Fatalf("session reason = %q in state %q, want %q in state %q",
				session.Failure.Kind, session.State, reason.kind, reason.state)
		}
	}
}

func assertPopulatedListResponse(t *testing.T, payload []byte, total int) {
	t.Helper()
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(response.Sessions))
	}
	got := response.Sessions[0]
	assertListObservationIdentity(t, got)
	assertListObservationProvider(t, got)
	assertListObservationUsage(t, got, total)
	assertListObservationTurnUsage(t, got, 3, 450, 450)
	assertListObservationTiming(t, got)
	assertListObservationTurn(t, got)
	assertListObservationExecutionFacts(t, got)
	if got.WorkId == nil || *got.WorkId != "work-1" || got.WorkName == nil || *got.WorkName != "Review Work" {
		t.Fatalf("work attribution = id=%v name=%v, want work-1/Review Work", got.WorkId, got.WorkName)
	}
}

func assertListObservationIdentity(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.WorkerSessionId != "worker-session-1" || observation.AttemptId != "attempt-1" || observation.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("identity/state = %#v, want worker-session-1/attempt-1/COMPLETED", observation)
	}
}

func assertListObservationProvider(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.ProviderSession == nil || observation.ProviderSession.Provider != "codex" || observation.ProviderSession.Kind != providers.SessionIDKind || observation.ProviderSession.Id != "provider-session-1" {
		t.Fatalf("provider session = %#v, want projected provider identity", observation.ProviderSession)
	}
}

func assertListObservationUsage(t *testing.T, observation factoryapi.WorkerSessionObservation, total int) {
	t.Helper()
	if observation.TokenUsage == nil || observation.TokenUsage.TotalTokens == nil || *observation.TokenUsage.TotalTokens != total {
		t.Fatalf("token usage = %#v, want total %d", observation.TokenUsage, total)
	}
}

func assertListObservationTiming(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.DurationMillis == nil || *observation.DurationMillis != 2500 {
		t.Fatalf("durationMillis = %#v, want 2500", observation.DurationMillis)
	}
}

func assertListObservationTurn(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.TurnId == nil || *observation.TurnId != "turn-1" {
		t.Fatalf("turnId = %#v, want turn-1", observation.TurnId)
	}
}

func TestListWorkerSessionsBySessionIDReturnsEmptyForKnownWorkWithoutObservations(t *testing.T) {
	service := &fakeObservationService{listErr: workersessions.ErrObservationWorkNotFound}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Sessions == nil || len(response.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want non-nil empty array", response.Sessions)
	}
}

func TestListWorkerSessionsBySessionIDOrdersAttemptsByStartTime(t *testing.T) {
	earlier := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Minute)
	service := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{
		{WorkerSessionID: "session-later", AttemptID: "attempt-2", State: workersessions.StateCompleted, StartedAt: &later, DurationBasis: workersessions.DurationBasisRecordedTimestamps, Transcript: workersessions.TranscriptAvailabilityUnavailable},
		{WorkerSessionID: "session-earlier", AttemptID: "attempt-1", State: workersessions.StateFailed, StartedAt: &earlier, DurationBasis: workersessions.DurationBasisRecordedTimestamps, Transcript: workersessions.TranscriptAvailabilityUnavailable},
	}}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 2 || response.Sessions[0].WorkerSessionId != "session-earlier" || response.Sessions[1].WorkerSessionId != "session-later" {
		t.Fatalf("session order = %#v, want chronological attempt order", response.Sessions)
	}
}

func TestListWorkerSessionsBySessionIDReturnsNotFoundForMissingWork(t *testing.T) {
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapter(service, workServiceStub{getErr: work.ErrWorkNotFound}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=missing", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "missing"},
	)

	if recorder.Code != 404 {
		t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want NOT_FOUND", response.Code)
	}
	if service.listCalled {
		t.Fatal("observation service called for missing Work")
	}
}

func TestListWorkerSessionsProjectsTopLevelScopeFiltersAndPagination(t *testing.T) {
	scope := factoryapi.ListWorkerSessionsParamsScope("direct")
	states := []factoryapi.ListWorkerSessionsParamsState{factoryapi.ListWorkerSessionsParamsState("COMPLETED")}
	maxResults := 1
	nextToken := "Y3Vyc29yLTE="
	duration := 1500 * time.Millisecond
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{
		Observations: []workersessions.Observation{{
			WorkerSessionID: "direct-1", Direct: true, ProviderSessionAvailable: true,
			ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
			AttemptID:       "attempt-1", State: workersessions.StateCompleted, Duration: &duration,
			DurationBasis: workersessions.DurationBasisRecordedTimestamps, Transcript: workersessions.TranscriptAvailabilityAvailable,
		}},
		MaxResults: 1, NextToken: "Y3Vyc29yLTI=",
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ListWorkerSessions(recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions", nil), factoryapi.ListWorkerSessionsParams{
		Scope: &scope, State: &states, MaxResults: &maxResults, NextToken: &nextToken,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := service.topLevelRequest.Scope; got != workersessions.ObservationScopeDirect {
		t.Fatalf("scope = %q, want direct", got)
	}
	if len(service.topLevelRequest.States) != 1 || service.topLevelRequest.States[0] != workersessions.StateCompleted || service.topLevelRequest.MaxResults != 1 || service.topLevelRequest.NextToken != nextToken {
		t.Fatalf("top-level request = %#v, want translated filters and cursor", service.topLevelRequest)
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || !response.Sessions[0].Direct {
		t.Fatalf("sessions = %#v, want one direct observation", response.Sessions)
	}
	if response.PaginationContext == nil || response.PaginationContext.MaxResults != 1 || response.PaginationContext.NextToken == nil || *response.PaginationContext.NextToken != "Y3Vyc29yLTI=" {
		t.Fatalf("pagination = %#v, want bounded continuation context", response.PaginationContext)
	}
}

func TestStartWorkerSessionReturnsAcceptedAfterStartBarrier(t *testing.T) {
	service := &fakeObservationService{startResult: workersessions.StartResult{Session: workersessions.Session{
		ID: "worker-session-1", State: workersessions.StateRunning,
	}}}
	handler := NewHandler(NewAdapterWithStart(service, service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/worker-sessions", strings.NewReader(`{
		"requestId": " request-1 ",
		"workerSessionId": " worker-session-1 ",
		"execution": {
			"workstationName": "swe",
			"dispatch": {"dispatchId": "dispatch-1", "transitionId": "transition-1", "workstationName": "swe"},
			"userMessage": "run the resolved work"
		}
	}`))

	handler.StartWorkerSession(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionStartResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Accepted || response.RequestId != "request-1" || response.WorkerSessionId != "worker-session-1" || response.State != factoryapi.WorkerSessionStartResponseState("RUNNING") {
		t.Fatalf("response = %#v, want admitted Worker Session snapshot", response)
	}
	if response.EventTopic != "worker-session/worker-session-1/events" {
		t.Fatalf("event topic = %q, want deterministic Worker Session topic", response.EventTopic)
	}
	if !service.startCalled {
		t.Fatal("Worker Sessions Start was not called")
	}
	if service.startRequest.RequestID != "request-1" || service.startRequest.ID != "worker-session-1" {
		t.Fatalf("start request identity = %#v, want normalized values", service.startRequest)
	}
	if service.startRequest.Execution.WorkstationName != "swe" || service.startRequest.Execution.Execution.Dispatch.DispatchID != "dispatch-1" {
		t.Fatalf("start execution = %#v, want resolved dispatch", service.startRequest.Execution)
	}
}

func TestStartWorkerSessionRejectsMalformedOrUnboundedInputBeforeService(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "missing request id", body: `{"workerSessionId":"worker-1","execution":{"workstationName":"swe","dispatch":{"dispatchId":"dispatch-1","workstationName":"swe"}}}`},
		{name: "retry budget too large", body: `{"requestId":"request-1","workerSessionId":"worker-1","retry":{"maxAttempts":17},"execution":{"workstationName":"swe","dispatch":{"dispatchId":"dispatch-1","workstationName":"swe"}}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{}
			handler := NewHandler(NewAdapterWithStart(service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.StartWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions", strings.NewReader(testCase.body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
			if service.startCalled {
				t.Fatal("Worker Sessions Start was called for malformed input")
			}
		})
	}
}

func TestStartWorkerSessionMapsStableServiceFailures(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code factoryapi.ErrorResponseCode
		want int
	}{
		{name: "request id conflict", err: workersessions.ErrStartRequestIDConflict, code: factoryapi.ErrorResponseCodeWORKERSESSIONSTARTREQUESTIDCONFLICT, want: http.StatusConflict},
		{name: "identity conflict", err: workersessions.ErrSessionNotStartable, code: factoryapi.ErrorResponseCodeWORKERSESSIONNOTSTARTABLE, want: http.StatusConflict},
		{name: "event unavailable", err: workersessions.ErrEventTopicUnavailable, code: factoryapi.ErrorResponseCodeWORKERSESSIONEVENTTOPICUNAVAILABLE, want: http.StatusServiceUnavailable},
		{name: "admission unavailable", err: workersessions.ErrStartAdmissionFailed, code: factoryapi.ErrorResponseCodeWORKERSESSIONADMISSIONFAILED, want: http.StatusServiceUnavailable},
		{name: "server stopping", err: workersessions.ErrStartServerStopping, code: factoryapi.ErrorResponseCodeWORKERSESSIONADMISSIONFAILED, want: http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{startErr: testCase.err}
			handler := NewHandler(NewAdapterWithStart(service, service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.StartWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions", strings.NewReader(`{"requestId":"request-1","workerSessionId":"worker-1","execution":{"workstationName":"swe","dispatch":{"dispatchId":"dispatch-1","workstationName":"swe"}}}`)))
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code || response.Family != errorFamilyForStatus(testCase.want) {
				t.Fatalf("error response = %#v, want code=%q family=%q", response, testCase.code, errorFamilyForStatus(testCase.want))
			}
		})
	}
}

type fakeObservationService struct {
	result                     workersessions.ListObservationsResult
	listErr                    error
	listCalled                 bool
	topLevelResult             workersessions.ListWorkerSessionObservationsResult
	topLevelErr                error
	topLevelCalled             bool
	topLevelRequest            workersessions.ListWorkerSessionObservationsRequest
	getResult                  workersessions.Observation
	getErr                     error
	getCalled                  bool
	getProviderSession         providers.SessionRef
	getByWorkerResult          workersessions.Observation
	getByWorkerErr             error
	getByWorkerCalled          bool
	getWorkerSessionID         string
	readResult                 workersessions.ReadTranscriptResult
	readErr                    error
	readCalled                 bool
	readWorkerSessionID        string
	readProviderSession        providers.SessionRef
	readByWorkerResult         workersessions.ReadTranscriptResult
	readByWorkerErr            error
	readByWorkerCalled         bool
	readByWorkerSessionID      string
	streamSubscription         *fakeObservationSubscription
	streamRequest              workersessions.StreamObservationsRequest
	streamErr                  error
	streamByWorkerSubscription *fakeObservationSubscription
	streamByWorkerRequest      workersessions.StreamObservationsByWorkerSessionIDRequest
	streamByWorkerErr          error
	streamByWorkerNil          bool
	startResult                workersessions.StartResult
	startErr                   error
	startCalled                bool
	startRequest               workersessions.StartRequest
	continueResult             workersessions.ContinueResult
	continueErr                error
	continueCalled             bool
	continueRequest            workersessions.ContinueRequest
	interruptResult            workersessions.InterruptResult
	interruptErr               error
	interruptCalled            bool
	interruptRequest           workersessions.InterruptRequest
}

func (f *fakeObservationService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	f.listCalled = true
	return f.result, f.listErr
}

func (f *fakeObservationService) ListWorkerSessionObservations(_ context.Context, request workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	f.topLevelCalled = true
	f.topLevelRequest = request
	return f.topLevelResult, f.topLevelErr
}

func (f *fakeObservationService) GetObservation(_ context.Context, request workersessions.GetObservationRequest) (workersessions.Observation, error) {
	f.getCalled = true
	f.getProviderSession = request.ProviderSession
	return f.getResult, f.getErr
}

func (f *fakeObservationService) GetObservationByWorkerSessionID(_ context.Context, request workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	f.getByWorkerCalled = true
	f.getWorkerSessionID = request.WorkerSessionID
	return f.getByWorkerResult, f.getByWorkerErr
}

func (f *fakeObservationService) ReadTranscript(_ context.Context, request workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	f.readCalled = true
	f.readWorkerSessionID = request.WorkerSessionID
	f.readProviderSession = request.ProviderSession
	return f.readResult, f.readErr
}

func (f *fakeObservationService) ReadTranscriptByWorkerSessionID(_ context.Context, request workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	f.readByWorkerCalled = true
	f.readByWorkerSessionID = request.WorkerSessionID
	return f.readByWorkerResult, f.readByWorkerErr
}

func (f *fakeObservationService) StreamObservations(_ context.Context, request workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	f.streamRequest = request
	if f.streamSubscription == nil {
		return workersessions.ObservationSubscription{}, f.streamErr
	}
	return workersessions.ObservationSubscription{
		NextFunc:  f.streamSubscription.Next,
		CloseFunc: f.streamSubscription.Close,
	}, f.streamErr
}

func (f *fakeObservationService) Start(_ context.Context, request workersessions.StartRequest) (workersessions.StartResult, error) {
	f.startCalled = true
	f.startRequest = request
	return f.startResult, f.startErr
}

func (f *fakeObservationService) Continue(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	f.continueCalled = true
	f.continueRequest = request
	return f.continueResult, f.continueErr
}

func (f *fakeObservationService) Interrupt(_ context.Context, request workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	f.interruptCalled = true
	f.interruptRequest = request
	return f.interruptResult, f.interruptErr
}

func (f *fakeObservationService) StreamObservationsByWorkerSessionID(_ context.Context, request workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	f.streamByWorkerRequest = request
	if f.streamByWorkerNil {
		return workersessions.ObservationSubscription{}, nil
	}
	if f.streamByWorkerSubscription == nil {
		return workersessions.ObservationSubscription{}, f.streamByWorkerErr
	}
	return workersessions.ObservationSubscription{
		NextFunc:  f.streamByWorkerSubscription.Next,
		CloseFunc: f.streamByWorkerSubscription.Close,
	}, f.streamByWorkerErr
}

type fakeObservationSubscription struct {
	deliveries []workersessions.ObservationDelivery
	index      int
	closed     bool
}

func (s *fakeObservationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
	}
	if s.index >= len(s.deliveries) {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	delivery := s.deliveries[s.index]
	s.index++
	return delivery
}

func (s *fakeObservationSubscription) Close() {
	s.closed = true
}

type sseTestFrame struct {
	Delivery              string                                      `json:"delivery"`
	WorkerSessionID       string                                      `json:"workerSessionId"`
	FactorySessionID      *string                                     `json:"factorySessionId"`
	ProviderSession       *factoryapi.WorkerSessionProviderSessionRef `json:"providerSession"`
	Event                 *sseTestEvent                               `json:"event"`
	ErrorCode             *string                                     `json:"errorCode"`
	ErrorMessage          *string                                     `json:"errorMessage"`
	ReplaySummary         *sseTestReplaySummary                       `json:"replaySummary"`
	RecordingHealth       *string                                     `json:"recordingHealth"`
	RecordingHealthReason *string                                     `json:"recordingHealthReason"`
}

type sseTestReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

type sseTestEvent struct {
	Cursor   *factoryapi.WorkerSessionEventCursor `json:"cursor"`
	Position uint64                               `json:"position"`
	Payload  json.RawMessage                      `json:"payload"`
}

func decodeSSEFrames(t *testing.T, body string) []sseTestFrame {
	t.Helper()
	var frames []sseTestFrame
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block), "data:"))
		if line == "" {
			continue
		}
		var frame sseTestFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode SSE frame %q: %v", line, err)
		}
		frames = append(frames, frame)
	}
	return frames
}

type workServiceStub struct {
	work.Service
	getErr     error
	getResult  work.ReadModel
	getResults map[string]work.ReadModel
}

func (s workServiceStub) GetWork(_ context.Context, _, workID string) (work.ReadModel, error) {
	if s.getErr != nil {
		return work.ReadModel{}, s.getErr
	}
	if result, ok := s.getResults[workID]; ok {
		return result, nil
	}
	if s.getResult.WorkID != "" || s.getResult.Name != "" {
		return s.getResult, nil
	}
	return work.ReadModel{WorkID: "known-work"}, nil
}

var _ observationService = (*fakeObservationService)(nil)
var _ work.Service = workServiceStub{}

func durationPtr(value time.Duration) *time.Duration { return &value }
