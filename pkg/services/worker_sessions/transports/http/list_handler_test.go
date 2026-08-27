package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestListWorkerSessionsTranslatesPositiveLimit(t *testing.T) {
	limit := factoryapi.WorkerSessionLimit(2)
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{}}}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, listWorkServiceStub{}), zap.NewNop()).ListWorkerSessions(
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
	NewHandler(NewAdapter(service, listWorkServiceStub{}), zap.NewNop()).ListWorkerSessions(
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
	NewHandler(NewAdapter(service, listWorkServiceStub{}), zap.NewNop()).ListWorkerSessions(
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

func TestWorkerSessionObservationHTTPExposesConfirmationAlongsideRecordingHealth(t *testing.T) {
	service := &fakeObservationService{
		topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
			{
				WorkerSessionID:   "worker-unconfirmed",
				AttemptID:         "attempt-unconfirmed",
				State:             workersessions.StateRunning,
				ConfirmationState: workersessions.ConfirmationStateUnconfirmed,
			},
			{
				WorkerSessionID:       "worker-confirmed",
				AttemptID:             "attempt-confirmed",
				State:                 workersessions.StateCompleted,
				ConfirmationState:     workersessions.ConfirmationStateConfirmed,
				RecordingHealth:       recordings.WorkerRecordingStatusDegraded,
				RecordingHealthReason: "PERSISTENCE_FAILED",
			},
		}},
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID:       "worker-confirmed",
			AttemptID:             "attempt-confirmed",
			State:                 workersessions.StateCompleted,
			ConfirmationState:     workersessions.ConfirmationStateConfirmed,
			RecordingHealth:       recordings.WorkerRecordingStatusIncomplete,
			RecordingHealthReason: "PROCESS_INTERRUPTED",
		},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())

	listRecorder := httptest.NewRecorder()
	handler.ListWorkerSessions(
		listRecorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
		factoryapi.ListWorkerSessionsParams{},
	)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listResponse factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResponse.Sessions) != 2 || listResponse.Sessions[0].ConfirmationState != factoryapi.UNCONFIRMED || listResponse.Sessions[1].ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("list confirmation states = %#v, want UNCONFIRMED/CONFIRMED", listResponse.Sessions)
	}
	if listResponse.Sessions[1].RecordingHealth == nil || *listResponse.Sessions[1].RecordingHealth != factoryapi.WorkerSessionObservationRecordingHealthDegraded {
		t.Fatalf("list recording health = %#v, want independent DEGRADED health", listResponse.Sessions[1].RecordingHealth)
	}

	showRecorder := httptest.NewRecorder()
	handler.GetWorkerSessionObservationByWorkerSessionId(
		showRecorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions/worker-confirmed", nil),
		factoryapi.WorkerSessionID("worker-confirmed"),
	)
	if showRecorder.Code != http.StatusOK {
		t.Fatalf("show status = %d, want 200; body=%s", showRecorder.Code, showRecorder.Body.String())
	}
	var showResponse factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(showRecorder.Body.Bytes(), &showResponse); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if showResponse.ConfirmationState != factoryapi.CONFIRMED || showResponse.RecordingHealth == nil || *showResponse.RecordingHealth != factoryapi.WorkerSessionObservationRecordingHealthIncomplete {
		t.Fatalf("show confirmation/health = %q/%#v, want CONFIRMED/INCOMPLETE", showResponse.ConfirmationState, showResponse.RecordingHealth)
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
			NewHandler(NewAdapter(service, listWorkServiceStub{}), zap.NewNop()).ListWorkerSessions(
				recorder,
				httptest.NewRequest(http.MethodGet, "/worker-sessions", nil),
				testCase.params,
			)
			assertBadRequestResponse(t, recorder)
		})
	}
}

func TestListWorkerSessionsMapsAndLogsBackendFailure(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	service := &fakeObservationService{topLevelErr: errors.New("recording projection failed")}
	handler := NewHandler(NewAdapter(service, listWorkServiceStub{}), zap.New(core))
	recorder := httptest.NewRecorder()
	scope := factoryapi.ListWorkerSessionsParamsScope("all")
	handler.ListWorkerSessions(
		recorder,
		httptest.NewRequest(http.MethodGet, "/worker-sessions?scope=all", nil),
		factoryapi.ListWorkerSessionsParams{Scope: &scope},
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR || response.Message != "failed to list Worker Sessions" {
		t.Fatalf("error response = %#v, want safe typed list failure", response)
	}
	entries := logs.All()
	if len(entries) != 1 || entries[0].Message != "list Worker Sessions failed" {
		t.Fatalf("log entries = %#v, want one list failure entry", entries)
	}
	fields := entries[0].ContextMap()
	if fields["operation"] != "worker_sessions.list" || fields["scope"] != "all" || fields["state_count"] != int64(0) {
		t.Fatalf("log fields = %#v, want safe operation context", fields)
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
	NewHandler(NewAdapter(service, listWorkServiceStub{getResults: fleetWorkResults()}), zap.NewNop()).ListWorkerSessions(
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

func TestListWorkerSessionsBatchesWorkAttributionReadsByFactorySession(t *testing.T) {
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
	workListReads := 0
	workGetReads := 0
	workListMaxResults := []int{}
	workReader := listWorkServiceStub{
		getResults:     fleetWorkResults(),
		getCallCount:   &workGetReads,
		listCallCount:  &workListReads,
		listMaxResults: &workListMaxResults,
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
	if workListReads != 1 {
		t.Fatalf("Work list reads = %d, want one bounded read for the shared Factory Session", workListReads)
	}
	if len(workListMaxResults) != 1 || workListMaxResults[0] != work.DefaultListMaxResults {
		t.Fatalf("Work list max results = %#v, want one default bounded page", workListMaxResults)
	}
	if workGetReads != 0 {
		t.Fatalf("Work get reads = %d, want no per-observation reads", workGetReads)
	}
}

func TestListWorkerSessionsWorkAttributionScalesWithoutPerObservationReads(t *testing.T) {
	started := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	const observationCount = 100
	observations := make([]workersessions.Observation, 0, observationCount)
	workResults := make(map[string]work.ReadModel, observationCount)
	for index := 0; index < observationCount; index++ {
		workID := fmt.Sprintf("work-%03d", index)
		workerSessionID := fmt.Sprintf("worker-session-%03d", index)
		observations = append(observations, workersessions.Observation{
			WorkerSessionID:  workerSessionID,
			FactorySessionID: "~default",
			WorkIDs:          []string{workID},
			AttemptID:        "attempt-" + workerSessionID,
			State:            workersessions.StateCompleted,
			StartedAt:        &started,
		})
		workResults[workID] = work.ReadModel{WorkID: workID, Name: "Work " + workID}
	}
	service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: observations}}
	workListReads := 0
	workGetReads := 0
	workListMaxResults := []int{}
	workReader := listWorkServiceStub{
		getCallCount:   &workGetReads,
		listCallCount:  &workListReads,
		listMaxResults: &workListMaxResults,
		listResult:     work.ListResult{Results: readModels(workResults)},
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
	if len(response.Sessions) != observationCount {
		t.Fatalf("session count = %d, want %d", len(response.Sessions), observationCount)
	}
	if workListReads != 1 || workGetReads != 0 {
		t.Fatalf("Work reads = list:%d get:%d, want one list and no gets", workListReads, workGetReads)
	}
	if len(workListMaxResults) != 1 || workListMaxResults[0] != observationCount {
		t.Fatalf("Work list max results = %#v, want one page sized to the bounded observation set", workListMaxResults)
	}
	for _, observation := range response.Sessions {
		if observation.WorkName == nil || *observation.WorkName == "" {
			t.Fatalf("observation %q missing Work attribution: %#v", observation.WorkerSessionId, observation)
		}
	}
}

func readModels(results map[string]work.ReadModel) []work.ReadModel {
	models := make([]work.ReadModel, 0, len(results))
	for _, result := range results {
		models = append(models, result)
	}
	return models
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

type listWorkServiceStub struct {
	work.Service
	getErr         error
	getResults     map[string]work.ReadModel
	getCallCount   *int
	listErr        error
	listResult     work.ListResult
	listCallCount  *int
	listSessionIDs *[]string
	listMaxResults *[]int
}

func (s listWorkServiceStub) GetWork(_ context.Context, _, workID string) (work.ReadModel, error) {
	if s.getCallCount != nil {
		*s.getCallCount = *s.getCallCount + 1
	}
	if s.getErr != nil {
		return work.ReadModel{}, s.getErr
	}
	if result, ok := s.getResults[workID]; ok {
		return result, nil
	}
	return work.ReadModel{WorkID: "known-work"}, nil
}

func (s listWorkServiceStub) ListWork(_ context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	if s.listCallCount != nil {
		*s.listCallCount = *s.listCallCount + 1
	}
	if s.listSessionIDs != nil {
		*s.listSessionIDs = append(*s.listSessionIDs, sessionID)
	}
	if s.listMaxResults != nil {
		*s.listMaxResults = append(*s.listMaxResults, options.MaxResults)
	}
	if s.listErr != nil {
		return work.ListResult{}, s.listErr
	}
	if s.listResult.Results != nil || s.listResult.MaxResults != 0 || s.listResult.NextToken != "" || s.listResult.Counts != nil {
		return s.listResult, nil
	}
	results := make([]work.ReadModel, 0, len(s.getResults))
	for _, result := range s.getResults {
		results = append(results, result)
	}
	return work.ListResult{Results: results}, nil
}

var _ work.Service = listWorkServiceStub{}
