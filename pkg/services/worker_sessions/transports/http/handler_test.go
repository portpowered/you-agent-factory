package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	observations := []workersessions.Observation{
		{
			WorkerSessionID:          "worker-session-1",
			ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
			ProviderSessionAvailable: true,
			WorkIDs:                  []string{"work-1"},
			TurnID:                   "turn-1",
			AttemptID:                "attempt-1",
			State:                    workersessions.StateCompleted,
			Duration:                 &duration,
			DurationBasis:            workersessions.DurationBasisRecordedTimestamps,
			TokenUsage:               &workersessions.TokenUsage{TotalTokens: &total},
			Transcript:               workersessions.TranscriptAvailabilityAvailable,
		},
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

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(response.Sessions))
	}
	got := response.Sessions[0]
	if got.WorkerSessionId != "worker-session-1" || got.AttemptId != "attempt-1" || got.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("identity/state = %#v, want worker-session-1/attempt-1/COMPLETED", got)
	}
	if got.ProviderSession == nil || got.ProviderSession.Provider != "codex" || got.ProviderSession.Kind != providers.SessionIDKind || got.ProviderSession.Id != "provider-session-1" {
		t.Fatalf("provider session = %#v, want projected provider identity", got.ProviderSession)
	}
	if got.TokenUsage == nil || got.TokenUsage.TotalTokens == nil || *got.TokenUsage.TotalTokens != total {
		t.Fatalf("token usage = %#v, want total %d", got.TokenUsage, total)
	}
	if got.DurationMillis == nil || *got.DurationMillis != 2500 {
		t.Fatalf("durationMillis = %#v, want 2500", got.DurationMillis)
	}
	if got.TurnId == nil || *got.TurnId != "turn-1" {
		t.Fatalf("turnId = %#v, want turn-1", got.TurnId)
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

func TestGetWorkerSessionObservationBySessionIDProjectsFailureDiagnostics(t *testing.T) {
	total := 17
	duration := int64(2500)
	failure := &workersessions.FailureCause{
		Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "provider exited with status 1",
		ProviderFailureKind: providers.ExecuteFailureKindDependency,
	}
	service := &fakeObservationService{getResult: workersessions.Observation{
		WorkerSessionID:          "worker-session-1",
		ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
		ProviderSessionAvailable: true,
		WorkIDs:                  []string{"work-1"}, TurnID: "turn-1", AttemptID: "attempt-1",
		State: workersessions.StateFailed, Duration: durationPtr(2500 * time.Millisecond),
		DurationBasis: workersessions.DurationBasisRecordedTimestamps,
		TokenUsage:    &workersessions.TokenUsage{TotalTokens: &total}, Transcript: workersessions.TranscriptAvailabilityAvailable,
		Failure: failure,
		Parse:   workersessions.ParseDiagnostics{EventCount: 4, Errors: []workersessions.ParseDiagnostic{{Code: "provider_session_parse_error", LineNumber: 3, Message: "malformed event"}}},
	}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail?provider=codex&kind=session_id&id=provider-session-1", nil)

	handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
		Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "provider-session-1",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.WorkerSessionId != "worker-session-1" || response.State != factoryapi.WorkerSessionObservationStateFailed || response.AttemptId != "attempt-1" {
		t.Fatalf("identity/state = %#v, want failed attempt projection", response)
	}
	if response.Failure == nil || response.Failure.Detail != "provider exited with status 1" || response.Failure.ProviderFailureKind == nil {
		t.Fatalf("failure = %#v, want structured failure diagnostics", response.Failure)
	}
	if response.TokenUsage == nil || response.TokenUsage.TotalTokens == nil || *response.TokenUsage.TotalTokens != total || response.DurationMillis == nil || *response.DurationMillis != duration {
		t.Fatalf("usage/duration = %#v/%v, want %d/%d", response.TokenUsage, response.DurationMillis, total, duration)
	}
	if response.Parse.EventCount != 4 || len(response.Parse.Errors) != 1 {
		t.Fatalf("parse = %#v, want event and parse diagnostics", response.Parse)
	}
	if service.getProviderSession != (providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}) {
		t.Fatalf("service identity = %#v, want exact provider session ref", service.getProviderSession)
	}
}

func TestAdapterGetWorkerSessionObservationUsesExactProviderSessionIdentity(t *testing.T) {
	service := &fakeObservationService{getResult: workersessions.Observation{
		WorkerSessionID: "worker-session-1", ProviderSessionAvailable: true,
		ProviderSession: providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "cursor-session-1"},
		AttemptID:       "attempt-1", State: workersessions.StateCompleted,
		DurationBasis: workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable,
	}}
	adapter := NewAdapter(service, workServiceStub{})
	response, err := adapter.GetWorkerSessionObservation(context.Background(), "session-1", " cursor ", " session_id ", " cursor-session-1 ")
	if err != nil {
		t.Fatalf("GetWorkerSessionObservation() error = %v", err)
	}
	if response.WorkerSessionId != "worker-session-1" || response.ProviderSession == nil || response.ProviderSession.Id != "cursor-session-1" {
		t.Fatalf("response = %#v, want detached observation", response)
	}
	want := providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "cursor-session-1"}
	if service.getProviderSession != want {
		t.Fatalf("service identity = %#v, want %#v", service.getProviderSession, want)
	}
}

func TestGetWorkerSessionObservationBySessionIDMapsMissingAndUnavailable(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		status int
		code   factoryapi.ErrorResponseCode
	}{
		{name: "missing", err: workersessions.ErrObservationSessionNotFound, status: http.StatusNotFound, code: factoryapi.ErrorResponseCodeNOTFOUND},
		{name: "unavailable", err: workersessions.ErrObservationProjectionUnavailable, status: http.StatusInternalServerError, code: factoryapi.ErrorResponseCode("PROJECTION_UNAVAILABLE")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{getErr: testCase.err}
			handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail", nil)
			handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
				Provider: factoryapi.LoadableProviderSessionProvider("codex"), Kind: factoryapi.LoadableProviderSessionKind("session_id"), Id: "missing",
			})
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

func TestGetWorkerSessionObservationBySessionIDRejectsUnsupportedIdentity(t *testing.T) {
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	for _, testCase := range []struct {
		name     string
		provider string
		kind     string
		code     factoryapi.ErrorResponseCode
	}{
		{name: "provider", provider: "other", kind: "session_id", code: factoryapi.ErrorResponseCode("PROVIDER_UNSUPPORTED")},
		{name: "kind", provider: "codex", kind: "rollout", code: factoryapi.ErrorResponseCode("SESSION_KIND_UNSUPPORTED")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions/detail", nil)
			handler.GetWorkerSessionObservationBySessionId(recorder, request, factoryapi.SessionID("session-1"), factoryapi.GetWorkerSessionObservationBySessionIdParams{
				Provider: factoryapi.LoadableProviderSessionProvider(testCase.provider), Kind: factoryapi.LoadableProviderSessionKind(testCase.kind), Id: "session-1",
			})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != testCase.code {
				t.Fatalf("error code = %q, want %q", response.Code, testCase.code)
			}
			if service.getCalled {
				t.Fatal("observation service called for unsupported identity")
			}
		})
	}
}

type fakeObservationService struct {
	result             workersessions.ListObservationsResult
	listErr            error
	listCalled         bool
	getResult          workersessions.Observation
	getErr             error
	getCalled          bool
	getProviderSession providers.SessionRef
}

func (f *fakeObservationService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	f.listCalled = true
	return f.result, f.listErr
}

func (f *fakeObservationService) GetObservation(_ context.Context, request workersessions.GetObservationRequest) (workersessions.Observation, error) {
	f.getCalled = true
	f.getProviderSession = request.ProviderSession
	return f.getResult, f.getErr
}

func (*fakeObservationService) StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	return nil, nil
}

type workServiceStub struct {
	work.Service
	getErr error
}

func (s workServiceStub) GetWork(context.Context, string, string) (work.ReadModel, error) {
	if s.getErr != nil {
		return work.ReadModel{}, s.getErr
	}
	return work.ReadModel{WorkID: "known-work"}, nil
}

var _ workersessions.ObservationService = (*fakeObservationService)(nil)
var _ work.Service = workServiceStub{}

func durationPtr(value time.Duration) *time.Duration { return &value }
