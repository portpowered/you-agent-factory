package http

import (
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

func TestWorkerSessionStartMappingRoundTripsResolvedExecutionAndOptionalValues(t *testing.T) {
	var apiRequest factoryapi.WorkerSessionStartRequest
	if err := json.Unmarshal([]byte(`{
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
	}`), &apiRequest); err != nil {
		t.Fatalf("decode mapping fixture: %v", err)
	}
	serviceRequest, err := WorkerSessionStartRequestFromAPI(apiRequest)
	if err != nil {
		t.Fatalf("WorkerSessionStartRequestFromAPI() = %v, want nil", err)
	}
	if serviceRequest.RequestID != "request-1" || serviceRequest.ID != "worker-1" || serviceRequest.Retry.MaxAttempts != 3 ||
		serviceRequest.Execution.WorkstationName != "review" || serviceRequest.Execution.Execution.Dispatch.DispatchID != "dispatch-1" ||
		serviceRequest.Execution.Execution.ResumeSession == nil || serviceRequest.Execution.Execution.ResumeSession.ID != "provider-session" ||
		len(serviceRequest.Execution.Execution.ModelBindings) != 1 || serviceRequest.Execution.Execution.ModelBindings[0].Slot != "slot" ||
		serviceRequest.Execution.Execution.WorkingDirectoryAuthored != true || !serviceRequest.Execution.Execution.SkipPermissions {
		t.Fatalf("mapped service request = %#v, want all resolved execution values", serviceRequest)
	}
	apiRoundTrip, err := WorkerSessionStartRequestToAPI(serviceRequest)
	if err != nil {
		t.Fatalf("WorkerSessionStartRequestToAPI() = %v, want nil", err)
	}
	if apiRoundTrip.RequestId != "request-1" || apiRoundTrip.WorkerSessionId != "worker-1" || apiRoundTrip.Retry == nil || apiRoundTrip.Retry.MaxAttempts == nil || *apiRoundTrip.Retry.MaxAttempts != 3 ||
		apiRoundTrip.Execution.ResumeSession == nil || apiRoundTrip.Execution.ResumeSession.Id != "provider-session" || apiRoundTrip.Execution.Dispatch.Execution == nil ||
		apiRoundTrip.Execution.Dispatch.Execution.RequestId == nil || *apiRoundTrip.Execution.Dispatch.Execution.RequestId != "request-1" || apiRoundTrip.Execution.EnvVars == nil || apiRoundTrip.Execution.ModelBindings == nil {
		t.Fatalf("round-tripped API request = %#v, want retained optional execution fields", apiRoundTrip)
	}
	minimal, err := WorkerSessionStartRequestToAPI(workersessions.StartRequest{RequestID: "request", ID: "worker"})
	if err != nil || minimal.Retry != nil || minimal.Execution.Dispatch.Execution != nil || minimal.Execution.ResumeSession != nil {
		t.Fatalf("minimal WorkerSessionStartRequestToAPI() = %#v, %v, want omitted optional values", minimal, err)
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
