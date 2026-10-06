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

func TestListWorkerSessionsHistoryQuery(t *testing.T) {
	t.Parallel()
	for _, history := range []string{"active", "all", "archived", "", "recent"} {
		t.Run(history, func(t *testing.T) {
			t.Parallel()
			service := &fakeObservationService{topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{}}}
			recorder := httptest.NewRecorder()
			value := factoryapi.ListWorkerSessionsParamsHistory(history)
			NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions", nil), factoryapi.ListWorkerSessionsParams{History: &value})
			if history == "" || history == "recent" {
				if recorder.Code != http.StatusBadRequest || service.topLevelCalled {
					t.Fatalf("invalid history status=%d called=%t", recorder.Code, service.topLevelCalled)
				}
			} else if recorder.Code != http.StatusOK || service.topLevelRequest.History != workersessions.ObservationHistory(history) {
				t.Fatalf("history status=%d request=%#v", recorder.Code, service.topLevelRequest)
			}
		})
	}
}

func TestListWorkerSessionsHistoryUnavailable(t *testing.T) {
	t.Parallel()
	service := &fakeObservationService{topLevelErr: workersessions.ErrObservationProjectionUnavailable}
	recorder := httptest.NewRecorder()
	history := factoryapi.ListWorkerSessionsParamsHistory("archived")
	NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessions(recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions", nil), factoryapi.ListWorkerSessionsParams{History: &history})
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusInternalServerError || response.Code != factoryapi.ErrorResponseCodePROJECTIONUNAVAILABLE {
		t.Fatalf("unavailable history status=%d response=%#v", recorder.Code, response)
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

// The same Work ID can belong to separate Factory Sessions. A fleet page
// resolves each session once, retaining attribution even for terminal Work.
func TestFleetWorkAttributionReadsEachSessionOnce(t *testing.T) {
	t.Parallel()
	reader := &fleetWorkReader{reads: make(map[string]int)}
	adapter := NewAdapter(&fakeObservationService{}, reader)
	observations := make([]workersessions.Observation, 0, 200)
	for index := range 200 {
		observations = append(observations, workersessions.Observation{
			WorkerSessionID:  fmt.Sprintf("worker-%d", index),
			FactorySessionID: fmt.Sprintf("session-%d", index%2),
			WorkIDs:          []string{fmt.Sprintf("work-%d", index/2)},
		})
	}
	for range 2 {
		attribution, err := adapter.resolveWorkAttribution(context.Background(), observations)
		if err != nil {
			t.Fatal(err)
		}
		for index, observation := range observations {
			got := attribution[observation.WorkerSessionID]
			wantName := fmt.Sprintf("session-%d/name-%d", index%2, index/2)
			if got.WorkID != observation.WorkIDs[0] || got.WorkName != wantName {
				t.Fatalf("attribution = %#v, want %s", got, wantName)
			}
		}
	}
	for session, reads := range reader.reads {
		if reads != 2 {
			t.Fatalf("%s reads = %d, want one per request", session, reads)
		}
	}
	if len(reader.reads) != 2 {
		t.Fatalf("session reads = %#v", reader.reads)
	}
}

func TestFleetWorkAttributionMissingAndCanceledReads(t *testing.T) {
	t.Parallel()
	for _, cancelRead := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRead), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &fleetWorkReader{reads: make(map[string]int), fail: true}
			if cancelRead {
				reader.cancel = cancel
			}
			adapter := NewAdapter(&fakeObservationService{}, reader)
			observations := []workersessions.Observation{
				{WorkerSessionID: "worker-a", FactorySessionID: "scope", WorkIDs: []string{"missing"}},
				{WorkerSessionID: "worker-b", FactorySessionID: "scope", WorkIDs: []string{"missing"}},
			}
			got, err := adapter.resolveWorkAttribution(ctx, observations)
			if cancelRead {
				if !errors.Is(err, context.Canceled) || got != nil {
					t.Fatalf("canceled result = %#v, %v", got, err)
				}
			} else {
				if err != nil || len(got) != 2 {
					t.Fatalf("missing result = %#v, %v", got, err)
				}
				for _, item := range got {
					if item.WorkID != "missing" || item.WorkName != "" {
						t.Fatalf("missing attribution = %#v", item)
					}
				}
			}
			if reader.reads["scope"] != 1 {
				t.Fatalf("reads = %#v", reader.reads)
			}
		})
	}
}

func TestSelectedWorkAttributionMatchesList(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"session-a", "session-b"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			observation := workersessions.Observation{
				WorkerSessionID: "worker", FactorySessionID: scope, WorkIDs: []string{"work-0"},
				State: workersessions.StateCompleted,
			}
			service := &fakeObservationService{
				getByWorkerResult: observation,
				topLevelResult:    workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{observation}},
			}
			reader := &fleetWorkReader{reads: make(map[string]int)}
			adapter := NewAdapter(service, reader)
			selected, err := adapter.GetTopLevelWorkerSessionObservation(t.Context(), "worker")
			if err != nil {
				t.Fatal(err)
			}
			listed, err := adapter.ListTopLevelWorkerSessions(t.Context(), "", "", nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if selected.WorkName == nil || *selected.WorkName != scope+"/name-0" || selected.WorkId == nil || *selected.WorkId != "work-0" ||
				len(listed.Sessions) != 1 || listed.Sessions[0].WorkName == nil || *listed.Sessions[0].WorkName != *selected.WorkName {
				t.Fatalf("selected/listed attribution = %+v / %+v", selected, listed)
			}
			if reader.reads[scope] != 2 || len(reader.reads) != 1 {
				t.Fatalf("unexpected Work scope reads: %v", reader.reads)
			}
		})
	}
}

func TestWorkAttributionWithoutFactoryScopeNeverReadsDefaultWork(t *testing.T) {
	t.Parallel()
	reader := &fleetWorkReader{reads: make(map[string]int)}
	adapter := NewAdapter(&fakeObservationService{}, reader)
	got, err := adapter.presentWorkerSessionObservation(t.Context(), workersessions.Observation{
		WorkerSessionID: "legacy-worker", WorkIDs: []string{"work-0"},
	})
	if err != nil || got.WorkName != nil || got.WorkId == nil || *got.WorkId != "work-0" || len(reader.reads) != 0 {
		t.Fatalf("unscoped attribution = %+v, %v; reads = %v", got, err, reader.reads)
	}
}

type fleetWorkReader struct {
	work.Service
	reads  map[string]int
	fail   bool
	cancel context.CancelFunc
}

type recordedAttributionFake struct {
	requests [][]recordings.WorkerWorkAttributionRequest
	err      error
}

func (f *recordedAttributionFake) ResolveWorkerWorkAttribution(_ context.Context, requests []recordings.WorkerWorkAttributionRequest) ([]recordings.WorkerWorkAttribution, error) {
	f.requests = append(f.requests, append([]recordings.WorkerWorkAttributionRequest(nil), requests...))
	results := make([]recordings.WorkerWorkAttribution, len(requests))
	for i, request := range requests {
		results[i] = recordings.WorkerWorkAttribution{WorkerSessionID: request.WorkerSessionID, FactorySessionID: request.FactorySessionID, WorkID: request.WorkID, WorkName: request.FactorySessionID + "/recorded"}
	}
	return results, f.err
}

func TestArchivedWorkAttributionSharesSelectedAndBatchReads(t *testing.T) {
	t.Parallel()
	observations := []workersessions.Observation{
		{WorkerSessionID: "worker-a", FactorySessionID: "scope-a", WorkIDs: []string{"reused"}},
		{WorkerSessionID: "worker-b", FactorySessionID: "scope-b", WorkIDs: []string{"reused"}},
		{WorkerSessionID: "direct"},
	}
	service := &fakeObservationService{getByWorkerResult: observations[0], topLevelResult: workersessions.ListWorkerSessionObservationsResult{Observations: observations}}
	reader := &fleetWorkReader{reads: make(map[string]int), fail: true}
	recorded := &recordedAttributionFake{}
	adapter := &Adapter{observations: service, topLevel: service, work: reader, attribution: recorded}
	listed, err := adapter.ListTopLevelWorkerSessions(t.Context(), "", "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 3 || len(recorded.requests) != 1 || len(recorded.requests[0]) != 2 {
		t.Fatalf("batch = %+v, requests=%+v", listed, recorded.requests)
	}
	assertArchivedAttributionRows(t, listed.Sessions)
	selected, err := adapter.GetTopLevelWorkerSessionObservation(t.Context(), "worker-a")
	if err != nil || selected.WorkName == nil || *selected.WorkName != "scope-a/recorded" {
		t.Fatalf("selected = %+v, %v", selected, err)
	}
}

func TestArchivedWorkAttributionPreservesTypedFailure(t *testing.T) {
	t.Parallel()
	want := &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorCorruptHistory, RecordingID: "corrupt"}
	adapter := &Adapter{work: &fleetWorkReader{reads: make(map[string]int), fail: true}, attribution: &recordedAttributionFake{err: want}}
	got, err := adapter.presentWorkerSessionObservation(t.Context(), workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "scope", WorkIDs: []string{"work"}})
	if !errors.Is(err, want) || got.WorkName != nil {
		t.Fatalf("corrupt attribution = %+v, %v", got, err)
	}
}

func (r *fleetWorkReader) GetWork(context.Context, string, string) (work.ReadModel, error) {
	panic("fleet must not read Work per row")
}

func (r *fleetWorkReader) ListWork(_ context.Context, session string, options work.ListOptions) (work.ListResult, error) {
	r.reads[session]++
	if r.cancel != nil {
		r.cancel()
	}
	if r.fail {
		return work.ListResult{}, work.ErrWorkNotFound
	}
	if !options.IncludeSuperseded || options.MaxResults < 100 || options.NextToken != "" {
		return work.ListResult{}, errors.New("fleet must read all Work including superseded")
	}
	result := work.ListResult{}
	for index := range 100 {
		result.Results = append(result.Results, work.ReadModel{
			WorkID: fmt.Sprintf("work-%d", index), Name: fmt.Sprintf("%s/name-%d", session, index),
		})
	}
	return result, nil
}

func TestListWorkerSessionsWorkHistoryRejectsBeforeEffects(t *testing.T) {
	t.Parallel()
	service := &fakeObservationService{}
	recorder := httptest.NewRecorder()
	NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop()).ListWorkerSessionsBySessionId(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/factory/worker-sessions?workId=work&history=all", nil), factoryapi.SessionID("factory"), factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Work history status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func assertArchivedAttributionRows(t *testing.T, rows []factoryapi.WorkerSessionObservation) {
	t.Helper()
	for _, row := range rows {
		if row.WorkerSessionId == "direct" {
			if row.WorkName != nil || row.WorkId != nil {
				t.Fatalf("direct attribution = %+v", row)
			}
			continue
		}
		if row.WorkName == nil || row.FactorySessionId == nil || *row.WorkName != *row.FactorySessionId+"/recorded" || row.WorkId == nil || *row.WorkId != "reused" {
			t.Fatalf("scoped attribution = %+v", row)
		}
	}
}
