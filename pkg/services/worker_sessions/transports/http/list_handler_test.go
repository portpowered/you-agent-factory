package http

import (
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

func TestListWorkerSessionsTranslatesPositiveLimit(t *testing.T) {
	limit := factoryapi.WorkerSessionLimit(2)
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{}}}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions?limit=2", nil),
		factoryapi.ListWorkerSessionsParams{Limit: &limit},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.topLevelCalled || service.topLevelRequest.MaxResults != 2 {
		t.Fatalf("top-level request = %#v, called=%t; want positive limit translated to service bound", service.topLevelRequest, service.topLevelCalled)
	}
}

func TestListWorkerSessionsRejectsNonPositiveLimit(t *testing.T) {
	limit := factoryapi.WorkerSessionLimit(0)
	service := &fakeObservationService{}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions?limit=0", nil),
		factoryapi.ListWorkerSessionsParams{Limit: &limit},
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Code)
	}
	if service.topLevelCalled {
		t.Fatal("observation service called for invalid limit")
	}
}

func TestListWorkerSessionsReturnsTopLevelEmptyCollection(t *testing.T) {
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{}, MaxResults: 50}}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
		factoryapi.ListWorkerSessionsParams{},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Sessions == nil || len(response.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want non-nil empty collection", response.Sessions)
	}
}

func TestListWorkerSessionsMapsInvalidScopeAndStateErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		params factoryapi.ListWorkerSessionsParams
	}{
		{name: "scope", err: workersessions.ErrInvalidObservationScope, params: invalidScopeParams()},
		{name: "state", err: workersessions.ErrInvalidState, params: invalidStateParams()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeObservationService{topLevelErr: testCase.err}
			recorder := httptest.NewRecorder()
			NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(
				recorder,
				httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
				testCase.params,
			)
			assertBadRequestResponse(t, recorder)
		})
	}
}

func invalidScopeParams() factoryapi.ListWorkerSessionsParams {
	scope := factoryapi.ListWorkerSessionsParamsScope("unexpected")
	return factoryapi.ListWorkerSessionsParams{Scope: &scope}
}

func invalidStateParams() factoryapi.ListWorkerSessionsParams {
	states := []factoryapi.ListWorkerSessionsParamsState{factoryapi.ListWorkerSessionsParamsState("UNKNOWN")}
	return factoryapi.ListWorkerSessionsParams{State: &states}
}

func assertBadRequestResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Code)
	}
}

func TestListWorkerSessionsProjectsFleetAttributionAndUnavailableFacts(t *testing.T) {
	service := fleetObservationService()
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workServiceStub{getResults: fleetWorkResults()}), zap.NewNop()).ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
		factoryapi.ListWorkerSessionsParams{},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertFleetObservationFacts(t, response)
}

func TestListWorkerSessionsCharacterizesPerObservationWorkAttributionReads(t *testing.T) {
	started := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	workerSessionIDs := []string{"fleet-worker-a", "fleet-worker-b", "fleet-worker-c"}
	observations := make([]workersessions.Observation, len(workerSessionIDs))
	for index, workerSessionID := range workerSessionIDs {
		observations[index] = workersessions.Observation{
			WorkerSessionID:  workerSessionID,
			FactorySessionID: "~default",
			WorkIDs:          []string{"work-a"},
			AttemptID:        "attempt-" + workerSessionID,
			State:            workersessions.StateCompleted,
			StartedAt:        &started,
		}
	}
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: observations}}
	workReads := 0
	workReader := workServiceStub{
		getResults:   fleetWorkResults(),
		getCallCount: &workReads,
	}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workReader), zap.NewNop()).ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
		factoryapi.ListWorkerSessionsParams{},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != len(observations) {
		t.Fatalf("session count = %d, want %d", len(response.Sessions), len(observations))
	}
	if workReads != len(observations) {
		t.Fatalf("Work attribution reads = %d, want one read per returned observation in the current path", workReads)
	}
}

func fleetWorkResults() map[string]work.ReadModel {
	return map[string]work.ReadModel{
		"work-a": {WorkID: "work-a", Name: "Build API"},
		"work-b": {WorkID: "work-b", Name: "Review UI"},
	}
}

func fleetObservationService() *fakeObservationService {
	started := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	duration := 3 * time.Second
	return &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
		{
			WorkerSessionID:          "fleet-running",
			Direct:                   true,
			FactorySessionID:         "~default",
			WorkIDs:                  []string{"work-a"},
			ProviderSessionAvailable: true,
			ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-a"},
			AttemptID:                "attempt-a",
			State:                    workersessions.StateRunning,
			StartedAt:                &started,
			DurationBasis:            workersessions.DurationBasisActiveClock,
			Transcript:               workersessions.TranscriptAvailabilityUnavailable,
		},
		{
			WorkerSessionID:  "fleet-failed",
			Direct:           false,
			FactorySessionID: "session-b",
			WorkIDs:          []string{"work-b"},
			AttemptID:        "attempt-b",
			State:            workersessions.StateFailed,
			StartedAt:        &started,
			Duration:         &duration,
			DurationBasis:    workersessions.DurationBasisRecordedTimestamps,
			Transcript:       workersessions.TranscriptAvailabilityUnavailable,
			Failure:          &workersessions.FailureCause{Kind: workersessions.FailureCauseTimeout, Detail: "timed out"},
		},
		{
			WorkerSessionID: "fleet-unavailable",
			Direct:          true,
			AttemptID:       "attempt-c",
			State:           workersessions.StateRunning,
			DurationBasis:   workersessions.DurationBasisUnavailable,
			Transcript:      workersessions.TranscriptAvailabilityUnavailable,
		},
	}}}
}

func assertFleetObservationFacts(t *testing.T, response factoryapi.ListWorkerSessionsResponse) {
	t.Helper()
	if len(response.Sessions) != 3 {
		t.Fatalf("session count = %d, want 3", len(response.Sessions))
	}
	assertFleetRunningObservation(t, response.Sessions[0])
	assertFleetFailedObservation(t, response.Sessions[1])
	assertFleetUnavailableObservation(t, response.Sessions[2])
}

func assertFleetRunningObservation(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.WorkId == nil || *observation.WorkId != "work-a" || observation.WorkName == nil || *observation.WorkName != "Build API" {
		t.Fatalf("running attribution = %#v, want work-a/Build API", observation)
	}
}

func assertFleetFailedObservation(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.Failure == nil || observation.Failure.Kind != string(workersessions.FailureCauseTimeout) || observation.DurationMillis == nil || *observation.DurationMillis != (3*time.Second).Milliseconds() {
		t.Fatalf("failed lifecycle facts = %#v, want timeout and duration", observation)
	}
	if observation.WorkId == nil || *observation.WorkId != "work-b" || observation.WorkName == nil || *observation.WorkName != "Review UI" {
		t.Fatalf("failed attribution = %#v, want work-b/Review UI", observation)
	}
}

func assertFleetUnavailableObservation(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.WorkId != nil || observation.WorkName != nil || observation.ProviderSession != nil || observation.DurationMillis != nil || observation.Failure != nil {
		t.Fatalf("unavailable optional facts = %#v, want explicit nulls", observation)
	}
}
