package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type sessionScopeResolverStub struct {
	scope SessionScope
	err   error
}

func (stub *sessionScopeResolverStub) ResolveWorkerSessionScope(context.Context, string) (SessionScope, error) {
	return stub.scope, stub.err
}

type decoratedSessionScopeResolverStub struct {
	sessionScopeResolverStub
	observations         workersessions.ObservationService
	resolveCalls         int
	observationSessionID string
}

func (stub *decoratedSessionScopeResolverStub) ResolveWorkerSessionScope(ctx context.Context, sessionID string) (SessionScope, error) {
	stub.resolveCalls++
	return stub.sessionScopeResolverStub.ResolveWorkerSessionScope(ctx, sessionID)
}

func (stub *decoratedSessionScopeResolverStub) WorkerSessionsObservationForSession(sessionID string) workersessions.ObservationService {
	stub.observationSessionID = sessionID
	return stub.observations
}

type sessionObservationServiceStub struct {
	workersessions.Service
	result workersessions.ListObservationsResult
}

func (stub *sessionObservationServiceStub) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return stub.result, nil
}

func assertListObservationExecutionFacts(t *testing.T, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	if observation.Model == nil || *observation.Model != "gpt-5.6-luna" ||
		observation.ReasoningEffort == nil || *observation.ReasoningEffort != "high" {
		t.Fatalf("execution facts = model:%#v reasoningEffort:%#v, want gpt-5.6-luna/high", observation.Model, observation.ReasoningEffort)
	}
}

func assertFailureObservationExecutionFacts(t *testing.T, response factoryapi.WorkerSessionObservation) {
	t.Helper()
	if response.Model == nil || *response.Model != "gpt-5.6-luna" ||
		response.ReasoningEffort == nil || *response.ReasoningEffort != "medium" {
		t.Fatalf("execution facts = model:%#v reasoningEffort:%#v, want gpt-5.6-luna/medium", response.Model, response.ReasoningEffort)
	}
}

func TestListWorkerSessionsBySessionIDPreservesRequestedDefaultAlias(t *testing.T) {
	resolvedID := "550e8400-e29b-41d4-a716-446655440000"
	service := &sessionObservationServiceStub{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
		WorkerSessionID:  "worker-session-default",
		FactorySessionID: defaultFactorySessionAlias,
		WorkIDs:          []string{"work-1"},
		AttemptID:        "attempt-default",
		State:            workersessions.StateCompleted,
		DurationBasis:    workersessions.DurationBasisRecordedTimestamps,
		Transcript:       workersessions.TranscriptAvailabilityUnavailable,
	}}}}
	resolver := &sessionScopeResolverStub{scope: SessionScope{EffectiveID: resolvedID, IsDefault: true}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}, resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/~default/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID(defaultFactorySessionAlias),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].FactorySessionId == nil || *response.Sessions[0].FactorySessionId != defaultFactorySessionAlias {
		t.Fatalf("default alias response = %#v, want one session scoped to %q", response.Sessions, defaultFactorySessionAlias)
	}
}

func TestListWorkerSessionsBySessionIDValidatesWorkBeforeResolvingScope(t *testing.T) {
	service := &sessionObservationServiceStub{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
		WorkerSessionID:  "worker-session-1",
		FactorySessionID: "session-1",
		WorkIDs:          []string{"work-1"},
		AttemptID:        "attempt-1",
		State:            workersessions.StateCompleted,
		DurationBasis:    workersessions.DurationBasisRecordedTimestamps,
		Transcript:       workersessions.TranscriptAvailabilityUnavailable,
	}}}}
	resolver := &decoratedSessionScopeResolverStub{
		sessionScopeResolverStub: sessionScopeResolverStub{scope: SessionScope{EffectiveID: "session-1"}},
		observations:             service,
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}, resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ListWorkerSessionsBySessionId(
		recorder,
		httptest.NewRequest("GET", "/factory-sessions/session-1/worker-sessions?workId=work-1", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if resolver.resolveCalls != 1 {
		t.Fatalf("Factory Session scope resolution calls = %d, want 1 after Work validation", resolver.resolveCalls)
	}
	if resolver.observationSessionID != "session-1" {
		t.Fatalf("scoped observation session ID = %q, want session-1", resolver.observationSessionID)
	}
}

func TestListWorkerSessionsBySessionIDDoesNotFallBackToRetainedObservationSource(t *testing.T) {
	retained := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
		WorkerSessionID:  "stale-worker-session",
		FactorySessionID: "session-1",
		WorkIDs:          []string{"work-1"},
		AttemptID:        "stale-attempt",
		State:            workersessions.StateCompleted,
	}}}}
	resolver := &decoratedSessionScopeResolverStub{
		sessionScopeResolverStub: sessionScopeResolverStub{scope: SessionScope{EffectiveID: "session-1"}},
	}
	handler := NewHandler(NewAdapter(retained, workServiceStub{}, resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ListWorkerSessionsBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/worker-sessions?workId=work-1", nil),
		factoryapi.SessionID("session-1"),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("error code = %q, want INTERNAL_ERROR", response.Code)
	}
	if retained.listCalled {
		t.Fatal("retained observation service was used after scoped source became unavailable")
	}
}

func TestListWorkerSessionsBySessionIDResolvesDefaultIdentityForLegacyObservation(t *testing.T) {
	resolvedID := "550e8400-e29b-41d4-a716-446655440000"
	service := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
		WorkerSessionID:  "worker-session-default",
		FactorySessionID: defaultFactorySessionAlias,
		WorkIDs:          []string{"work-1"},
		AttemptID:        "attempt-default",
		State:            workersessions.StateRunning,
		DurationBasis:    workersessions.DurationBasisRecordedTimestamps,
		Transcript:       workersessions.TranscriptAvailabilityUnavailable,
	}}}}
	resolver := &sessionScopeResolverStub{scope: SessionScope{EffectiveID: resolvedID, IsDefault: true}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}, resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/"+resolvedID+"/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID(resolvedID),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].FactorySessionId == nil || *response.Sessions[0].FactorySessionId != resolvedID {
		t.Fatalf("resolved default response = %#v, want one session scoped to %q", response.Sessions, resolvedID)
	}
}

func TestListWorkerSessionsBySessionIDMapsMismatchedObservationScopeToNotFound(t *testing.T) {
	resolvedID := "factory-session-1"
	service := &fakeObservationService{result: workersessions.ListObservationsResult{Observations: []workersessions.Observation{{
		WorkerSessionID:  "worker-session-other",
		FactorySessionID: "factory-session-other",
		WorkIDs:          []string{"work-1"},
		AttemptID:        "attempt-other",
		State:            workersessions.StateCompleted,
		DurationBasis:    workersessions.DurationBasisRecordedTimestamps,
		Transcript:       workersessions.TranscriptAvailabilityUnavailable,
	}}}}
	resolver := &sessionScopeResolverStub{scope: SessionScope{EffectiveID: resolvedID}}
	handler := NewHandler(NewAdapter(service, workServiceStub{}, resolver), zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/factory-sessions/"+resolvedID+"/worker-sessions?workId=work-1", nil)

	handler.ListWorkerSessionsBySessionId(
		recorder,
		request,
		factoryapi.SessionID(resolvedID),
		factoryapi.ListWorkerSessionsBySessionIdParams{WorkId: "work-1"},
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want NOT_FOUND", response.Code)
	}
}

const workerSessionStartMappingJSON = `{
	"requestId":"request-1",
	"workerSessionId":"worker-1",
	"retry":{"maxAttempts":3},
	"execution":{
		"workstationName":"review",
		"workerType":"worker",
		"workstationType":"type",
		"runnerId":"runner",
		"runnerSelectionSource":"explicit",
		"executorProvider":"provider",
		"projectId":"project",
		"factorySessionId":"factory-session",
		"modelOperation":"operation",
		"model":"model",
		"modelProvider":"codex",
		"reasoningEffort":"high",
		"systemPrompt":"system",
		"userMessage":"message",
		"outputSchema":"schema",
		"outputContract":"contract",
		"worktree":"worktree",
		"workingDirectory":"workspace",
		"workingDirectoryAuthored":true,
		"skipPermissions":true,
		"inputTokens":["input"],
		"envVars":{"KEY":"VALUE"},
		"resumeSession":{"provider":"codex","kind":"session_id","id":"provider-session"},
		"modelBindings":[{"slot":"slot","source":"INPUT","content":[]}],
		"dispatch":{
			"dispatchId":"dispatch-1",
			"transitionId":"transition",
			"workerType":"worker",
			"workstationName":"review",
			"projectId":"project",
			"currentChainingTraceId":"trace",
			"previousChainingTraceIds":["previous"],
			"expectedArtifactContext":{"project":"project","sessionId":"factory-session","inputs":[]},
			"execution":{"dispatchCreatedTick":1,"currentTick":2,"requestId":"request-1","traceId":"trace","workIds":["work-1"],"replayKey":"replay"},
			"inputTokens":["dispatch-input"],
			"inputBindings":{"slot":["work-1"]}
		}
	}
}`

func TestWorkerSessionStartMappingRoundTripsResolvedExecutionAndOptionalValues(t *testing.T) {
	serviceRequest := workerSessionStartServiceRequest(t)
	assertWorkerSessionStartServiceRequest(t, serviceRequest)

	apiRoundTrip, err := WorkerSessionStartRequestToAPI(serviceRequest)
	if err != nil {
		t.Fatalf("WorkerSessionStartRequestToAPI() = %v, want nil", err)
	}
	assertWorkerSessionStartAPIRoundTrip(t, apiRoundTrip)

	minimal, err := WorkerSessionStartRequestToAPI(workersessions.StartRequest{RequestID: "request", ID: "worker"})
	if err != nil {
		t.Fatalf("minimal WorkerSessionStartRequestToAPI() = %v, want nil", err)
	}
	if minimal.Retry != nil {
		t.Fatalf("minimal retry = %#v, want omitted", minimal.Retry)
	}
	if minimal.Execution.Dispatch.Execution != nil {
		t.Fatalf("minimal dispatch execution = %#v, want omitted", minimal.Execution.Dispatch.Execution)
	}
	if minimal.Execution.ResumeSession != nil {
		t.Fatalf("minimal resume session = %#v, want omitted", minimal.Execution.ResumeSession)
	}
}

func workerSessionStartServiceRequest(t *testing.T) workersessions.StartRequest {
	t.Helper()
	var apiRequest factoryapi.WorkerSessionStartRequest
	if err := json.Unmarshal([]byte(workerSessionStartMappingJSON), &apiRequest); err != nil {
		t.Fatalf("decode mapping fixture: %v", err)
	}
	serviceRequest, err := WorkerSessionStartRequestFromAPI(apiRequest)
	if err != nil {
		t.Fatalf("WorkerSessionStartRequestFromAPI() = %v, want nil", err)
	}
	return serviceRequest
}

func assertWorkerSessionStartServiceRequest(t *testing.T, request workersessions.StartRequest) {
	t.Helper()
	if request.RequestID != "request-1" {
		t.Fatalf("request ID = %q, want request-1", request.RequestID)
	}
	if request.ID != "worker-1" {
		t.Fatalf("worker session ID = %q, want worker-1", request.ID)
	}
	if request.Retry.MaxAttempts != 3 {
		t.Fatalf("retry max attempts = %d, want 3", request.Retry.MaxAttempts)
	}
	if request.Execution.WorkstationName != "review" {
		t.Fatalf("workstation = %q, want review", request.Execution.WorkstationName)
	}
	if request.Execution.Execution.Dispatch.DispatchID != "dispatch-1" {
		t.Fatalf("dispatch ID = %q, want dispatch-1", request.Execution.Execution.Dispatch.DispatchID)
	}
	if request.Execution.Execution.Continuation == nil || request.Execution.Execution.Continuation.ProviderSessionID != "provider-session" {
		t.Fatalf("continuation = %#v, want provider-session", request.Execution.Execution.Continuation)
	}
	if len(request.Execution.Execution.ModelBindings) != 1 || request.Execution.Execution.ModelBindings[0].Slot != "slot" {
		t.Fatalf("model bindings = %#v, want slot binding", request.Execution.Execution.ModelBindings)
	}
	if !request.Execution.Execution.WorkingDirectoryAuthored || !request.Execution.Execution.SkipPermissions {
		t.Fatalf("execution flags = authored:%t skip:%t, want true/true", request.Execution.Execution.WorkingDirectoryAuthored, request.Execution.Execution.SkipPermissions)
	}
}

func assertWorkerSessionStartAPIRoundTrip(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.RequestId != "request-1" {
		t.Fatalf("request ID = %q, want request-1", request.RequestId)
	}
	if request.WorkerSessionId != "worker-1" {
		t.Fatalf("worker session ID = %q, want worker-1", request.WorkerSessionId)
	}
	if request.Retry == nil || request.Retry.MaxAttempts == nil || *request.Retry.MaxAttempts != 3 {
		t.Fatalf("retry = %#v, want max attempts 3", request.Retry)
	}
	if request.Execution.ResumeSession == nil || request.Execution.ResumeSession.Id != "provider-session" {
		t.Fatalf("resume session = %#v, want provider-session", request.Execution.ResumeSession)
	}
	if request.Execution.Dispatch.Execution == nil || request.Execution.Dispatch.Execution.RequestId == nil || *request.Execution.Dispatch.Execution.RequestId != "request-1" {
		t.Fatalf("dispatch execution = %#v, want request-1", request.Execution.Dispatch.Execution)
	}
	if request.Execution.EnvVars == nil {
		t.Fatal("env vars are nil, want retained optional fields")
	}
	if request.Execution.ModelBindings == nil {
		t.Fatal("model bindings are nil, want retained optional fields")
	}
}

func TestTopLevelWorkerSessionIdentityRoutesResolveWithoutProviderTuple(t *testing.T) {
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID: "direct-1", Direct: true, ProviderSessionAvailable: false,
			AttemptID: "attempt-1", State: workersessions.StateRunning,
			DurationBasis: workersessions.DurationBasisActiveClock, Transcript: workersessions.TranscriptAvailabilityUnavailable,
		},
		readByWorkerResult: workersessions.ReadTranscriptResult{
			WorkerSessionID: "direct-1", ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
			AttemptID: "attempt-1", State: workersessions.StateCompleted,
		},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())

	detailRecorder := httptest.NewRecorder()
	handler.GetWorkerSessionObservationByWorkerSessionId(detailRecorder, httptest.NewRequest(http.MethodGet, "/worker-sessions/direct-1", nil), factoryapi.WorkerSessionID("direct-1"))
	if detailRecorder.Code != http.StatusOK || !strings.Contains(detailRecorder.Body.String(), `"direct":true`) || service.getWorkerSessionID != "direct-1" {
		t.Fatalf("detail status/body/request = %d/%s/%q, want identity-only direct lookup", detailRecorder.Code, detailRecorder.Body.String(), service.getWorkerSessionID)
	}

	transcriptRecorder := httptest.NewRecorder()
	handler.ReadWorkerSessionTranscriptByWorkerSessionId(transcriptRecorder, httptest.NewRequest(http.MethodGet, "/worker-sessions/direct-1/transcript", nil), factoryapi.WorkerSessionID("direct-1"))
	if transcriptRecorder.Code != http.StatusOK || !service.readByWorkerCalled || service.readByWorkerSessionID != "direct-1" {
		t.Fatalf("transcript status/request = %d/%t/%q, want identity-only transcript lookup", transcriptRecorder.Code, service.readByWorkerCalled, service.readByWorkerSessionID)
	}
}

func TestTopLevelWorkerSessionStreamPreservesReplaySummary(t *testing.T) {
	replayOnly := true
	service := &fakeObservationService{
		getByWorkerResult: workersessions.Observation{
			WorkerSessionID: "direct-1", Direct: true, State: workersessions.StateRunning,
		},
		streamByWorkerSubscription: &fakeObservationSubscription{deliveries: []workersessions.ObservationDelivery{
			{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{Position: 1, Payload: json.RawMessage(`{"state":"RUNNING"}`)}},
			{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: false, Reason: "session-active", EventsEmitted: 1}},
		}},
	}
	handler := NewHandler(NewAdapter(service, workServiceStub{}), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StreamWorkerSessionEventsByTopLevelWorkerSessionId(recorder, httptest.NewRequest(http.MethodGet, "/worker-sessions/direct-1/events", nil), factoryapi.WorkerSessionID("direct-1"), factoryapi.StreamWorkerSessionEventsByTopLevelWorkerSessionIdParams{ReplayOnly: &replayOnly})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	frames := decodeSSEFrames(t, recorder.Body.String())
	if len(frames) != 2 || frames[0].Delivery != "RECORD" || frames[1].Delivery != "REPLAY_SUMMARY" || service.streamByWorkerRequest.WorkerSessionID != "direct-1" || !service.streamByWorkerRequest.ReplayOnly {
		t.Fatalf("frames/request = %#v/%#v, want identity stream with replay summary", frames, service.streamByWorkerRequest)
	}
}

func TestWorkerSessionObservationToAPIMapsDurabilityConfirmation(t *testing.T) {
	defaulted := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID: "worker-unconfirmed",
		AttemptID:       "attempt-unconfirmed",
		State:           workersessions.StateRunning,
		DurationBasis:   workersessions.DurationBasisActiveClock,
		Transcript:      workersessions.TranscriptAvailabilityUnavailable,
	})
	if defaulted.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("default confirmationState = %q, want UNCONFIRMED", defaulted.ConfirmationState)
	}

	confirmed := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID:   "worker-confirmed",
		AttemptID:         "attempt-confirmed",
		State:             workersessions.StateCompleted,
		ConfirmationState: workersessions.ConfirmationStateConfirmed,
		DurationBasis:     workersessions.DurationBasisRecordedTimestamps,
		Transcript:        workersessions.TranscriptAvailabilityUnavailable,
	})
	if confirmed.ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("mapped confirmationState = %q, want CONFIRMED", confirmed.ConfirmationState)
	}
}

func TestWorkerSessionObservationToAPIPreservesOptionalTurnUsage(t *testing.T) {
	populated := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID: "worker-session-1",
		AttemptID:       "attempt-1",
		State:           workersessions.StateCompleted,
		DurationBasis:   workersessions.DurationBasisRecordedTimestamps,
		Transcript:      workersessions.TranscriptAvailabilityAvailable,
		TurnUsage:       &workersessions.TurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 700},
	})
	if populated.TurnUsage == nil || populated.TurnUsage.TurnCount != 3 || populated.TurnUsage.FinalContextTokens != 450 || populated.TurnUsage.PeakContextTokens != 700 {
		t.Fatalf("mapped turn usage = %#v, want count/final/peak 3/450/700", populated.TurnUsage)
	}
	assertTurnUsageJSONPresence(t, populated, true)

	omitted := WorkerSessionObservationToAPI(workersessions.Observation{
		WorkerSessionID: "worker-session-2",
		AttemptID:       "attempt-2",
		State:           workersessions.StateCompleted,
		DurationBasis:   workersessions.DurationBasisRecordedTimestamps,
		Transcript:      workersessions.TranscriptAvailabilityUnavailable,
	})
	if omitted.TurnUsage != nil {
		t.Fatalf("mapped turn usage = %#v, want nil without supported evidence", omitted.TurnUsage)
	}
	assertTurnUsageJSONPresence(t, omitted, false)
}

func assertTurnUsageJSONPresence(t *testing.T, observation any, wantPresent bool) {
	t.Helper()
	payload, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode observation: %v", err)
	}
	_, present := document["turnUsage"]
	if present != wantPresent {
		t.Fatalf("turnUsage present = %t, want %t; payload=%s", present, wantPresent, payload)
	}
}
