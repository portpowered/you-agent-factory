package runtime_api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event artifact sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordPath := filepath.Join(t.TempDir(), "inference-events.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-inference-events",
		WorkTypeID: "task",
		TraceID:    "trace-inference-events",
		Payload:    []byte(`{"title":"inspect inference events"}`),
	})

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithRecordPath(recordPath),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertFirstInferenceAttemptOrder(t, events)
	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertInferenceEventsRecordedInArtifact(t, events, artifact.Events)
}

func TestInferenceEvents_ScriptWorkersDoNotEmitInferenceEvents(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event script-worker sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-script-no-inference",
		WorkTypeID: "task",
		TraceID:    "trace-script-no-inference",
		Payload:    []byte("script input"),
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(support.NewStaticSuccessCommandRunner("script-output-ok")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if !hasFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest) ||
		!hasFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("script worker canonical events = %v, want dispatch lifecycle events", functionalEventTypes(events))
	}
	if hasFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest) ||
		hasFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse) {
		t.Fatalf("script worker emitted inference events: %v", functionalEventTypes(events))
	}
}

func TestInferenceEvents_HTTPStreamAndDashboardProjectionCorrelateRetryAttempts(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event stream-projection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{
			{},
			{},
			{Content: "Step one recovered. COMPLETE"},
			{Content: "Step two done. COMPLETE"},
		},
		[]error{
			workers.NewProviderError(interfaces.ProviderErrorTypeTimeout, "provider timeout", nil),
			workers.NewProviderError(interfaces.ProviderErrorTypeInternalServerError, "provider 500", nil),
			nil,
			nil,
		},
	)
	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderOverride = provider
		},
		factory.WithServiceMode(),
	)

	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	_, _ = requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "Retrying inference stream",
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "retry provider attempts",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	events := collectFunctionalEventsUntilDispatchCompletions(t, stream, 2, 10*time.Second)
	firstDispatchID := assertHTTPInferenceRetrySequence(t, events)
	assertDashboardInferenceProjection(t, server.GetDashboard(t), firstDispatchID, traceID)
}

func TestInferenceEvents_ThinEventSmoke_CapturesThinnedDispatchInferenceSequenceAndReconstructsViews(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event thin-event sweep")
	smoke := newThinEventSmokeHarness(t)

	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := smoke.harness.RunInBackground(runCtx)
	active := captureThinEventSmokeActiveSnapshot(t, smoke)
	assertThinEventSmokeActiveSnapshot(t, active)

	smoke.provider.ReleaseFirst()
	waitForFunctionalHarnessCompletion(t, smoke.harness, errCh, cancel, 5*time.Second)

	final := loadThinEventSmokeFinalSnapshot(t, smoke, active)
	assertThinEventSmokeFinalSnapshot(t, active, final)
}

func assertRuntimeAPIProjectionOmitsInferenceFields(t *testing.T, payload any, keys []string) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(%T): %v", payload, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal(%T): %v", payload, err)
	}
	for _, key := range keys {
		if _, ok := raw[key]; ok {
			t.Fatalf("%T unexpectedly carried retired inference-owned field %q: %#v", payload, key, raw[key])
		}
	}
}

func assertFirstInferenceAttemptOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	dispatchIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0)
	if dispatchIndex < 0 {
		t.Fatalf("missing dispatch-created event in %v", functionalEventTypes(events))
	}
	requestIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest, dispatchIndex+1)
	responseIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse, requestIndex+1)
	completedIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, responseIndex+1)
	if requestIndex < 0 || responseIndex < 0 || completedIndex < 0 {
		t.Fatalf("event order = %v, want dispatch-created, inference-request, inference-response, dispatch-completed", functionalEventTypes(events))
	}

	if _, err := events[dispatchIndex].Payload.AsDispatchRequestEventPayload(); err != nil {
		t.Fatalf("decode dispatch-created payload: %v", err)
	}
	request, err := events[requestIndex].Payload.AsInferenceRequestEventPayload()
	if err != nil {
		t.Fatalf("decode inference-request payload: %v", err)
	}
	response, err := events[responseIndex].Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode inference-response payload: %v", err)
	}
	if _, err := events[completedIndex].Payload.AsDispatchResponseEventPayload(); err != nil {
		t.Fatalf("decode dispatch-completed payload: %v", err)
	}

	dispatchID := stringValueFromFunctionalPtr(events[dispatchIndex].Context.DispatchId)
	if stringValueFromFunctionalPtr(events[requestIndex].Context.DispatchId) != dispatchID ||
		stringValueFromFunctionalPtr(events[responseIndex].Context.DispatchId) != dispatchID ||
		stringValueFromFunctionalPtr(events[completedIndex].Context.DispatchId) != dispatchID {
		t.Fatalf("dispatch correlation mismatch: dispatch=%s request=%s response=%s completed=%s",
			dispatchID,
			stringValueFromFunctionalPtr(events[requestIndex].Context.DispatchId),
			stringValueFromFunctionalPtr(events[responseIndex].Context.DispatchId),
			stringValueFromFunctionalPtr(events[completedIndex].Context.DispatchId))
	}
	if request.Attempt != 1 || response.Attempt != request.Attempt {
		t.Fatalf("attempt correlation mismatch: request=%d response=%d", request.Attempt, response.Attempt)
	}
	if request.InferenceRequestId == "" || response.InferenceRequestId != request.InferenceRequestId {
		t.Fatalf("inference request correlation mismatch: request=%q response=%q", request.InferenceRequestId, response.InferenceRequestId)
	}
	assertRawInferenceEventUsesContextDispatchIdentity(t, events[requestIndex], request.InferenceRequestId)
	assertRawInferenceEventUsesContextDispatchIdentity(t, events[responseIndex], response.InferenceRequestId)
	if request.Prompt == "" {
		t.Fatal("inference request prompt is empty")
	}
	if response.Outcome != factoryapi.InferenceOutcomeSucceeded || stringValueFromFunctionalPtr(response.Response) != "Step one done. COMPLETE" {
		t.Fatalf("inference response = %#v, want succeeded first provider response", response)
	}
	if response.DurationMillis < 0 {
		t.Fatalf("durationMillis = %d, want non-negative", response.DurationMillis)
	}
}

func collectFunctionalEventsUntilDispatchCompletions(t *testing.T, stream *factoryEventHTTPStream, wantCompletions int, timeout time.Duration) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	events := make([]factoryapi.FactoryEvent, 0, 12)
	completions := 0
	for time.Now().Before(deadline) && completions < wantCompletions {
		event := stream.next(time.Until(deadline))
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			completions++
		}
	}
	if completions < wantCompletions {
		t.Fatalf("collected %d dispatch completions, want %d; events=%v", completions, wantCompletions, functionalEventTypes(events))
	}
	return events
}

func assertHTTPInferenceRetrySequence(t *testing.T, events []factoryapi.FactoryEvent) string {
	t.Helper()

	dispatchIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0)
	if dispatchIndex < 0 {
		t.Fatalf("missing dispatch-created event in %v", functionalEventTypes(events))
	}
	if _, err := events[dispatchIndex].Payload.AsDispatchRequestEventPayload(); err != nil {
		t.Fatalf("decode dispatch-created payload: %v", err)
	}

	next := dispatchIndex + 1
	for attempt := 1; attempt <= 3; attempt++ {
		requestIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest, next)
		responseIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse, requestIndex+1)
		if requestIndex < 0 || responseIndex < 0 {
			t.Fatalf("event order = %v, want three request/response pairs after first dispatch", functionalEventTypes(events))
		}
		dispatchID := stringValueFromFunctionalPtr(events[dispatchIndex].Context.DispatchId)
		request := assertFunctionalInferenceRequest(t, events[requestIndex], dispatchID, attempt)
		response := assertFunctionalInferenceResponse(t, events[responseIndex], dispatchID, request.InferenceRequestId, attempt)
		assertRawInferenceEventUsesContextDispatchIdentity(t, events[requestIndex], request.InferenceRequestId)
		assertRawInferenceEventUsesContextDispatchIdentity(t, events[responseIndex], response.InferenceRequestId)
		if attempt < 3 && response.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("attempt %d outcome = %s, want FAILED", attempt, response.Outcome)
		}
		if attempt == 3 {
			if response.Outcome != factoryapi.InferenceOutcomeSucceeded || stringValueFromFunctionalPtr(response.Response) != "Step one recovered. COMPLETE" {
				t.Fatalf("attempt 3 response = %#v, want recovered success response", response)
			}
		}
		next = responseIndex + 1
	}

	completedIndex := indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse, next)
	if completedIndex < 0 {
		t.Fatalf("event order = %v, want dispatch-completed after retry response", functionalEventTypes(events))
	}
	if _, err := events[completedIndex].Payload.AsDispatchResponseEventPayload(); err != nil {
		t.Fatalf("decode dispatch-completed payload: %v", err)
	}
	if stringValueFromFunctionalPtr(events[completedIndex].Context.DispatchId) != stringValueFromFunctionalPtr(events[dispatchIndex].Context.DispatchId) {
		t.Fatalf("dispatch completion id = %s, want %s", stringValueFromFunctionalPtr(events[completedIndex].Context.DispatchId), stringValueFromFunctionalPtr(events[dispatchIndex].Context.DispatchId))
	}
	return stringValueFromFunctionalPtr(events[dispatchIndex].Context.DispatchId)
}

func assertFunctionalInferenceRequest(t *testing.T, event factoryapi.FactoryEvent, dispatchID string, attempt int) factoryapi.InferenceRequestEventPayload {
	t.Helper()

	request, err := event.Payload.AsInferenceRequestEventPayload()
	if err != nil {
		t.Fatalf("decode inference-request payload: %v", err)
	}
	if stringValueFromFunctionalPtr(event.Context.DispatchId) != dispatchID || request.Attempt != attempt {
		t.Fatalf("inference request correlation = %#v, want dispatch=%s attempt=%d", request, dispatchID, attempt)
	}
	if request.InferenceRequestId == "" || request.Prompt == "" {
		t.Fatalf("inference request missing request ID or prompt: %#v", request)
	}
	return request
}

func assertFunctionalInferenceResponse(t *testing.T, event factoryapi.FactoryEvent, dispatchID, requestID string, attempt int) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	response, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode inference-response payload: %v", err)
	}
	if stringValueFromFunctionalPtr(event.Context.DispatchId) != dispatchID ||
		response.InferenceRequestId != requestID || response.Attempt != attempt {
		t.Fatalf("inference response correlation = %#v, want dispatch=%s request=%s attempt=%d", response, dispatchID, requestID, attempt)
	}
	if response.DurationMillis < 0 {
		t.Fatalf("durationMillis = %d, want non-negative", response.DurationMillis)
	}
	return response
}

func assertDashboardInferenceProjection(t *testing.T, dashboard DashboardResponse, dispatchID, traceID string) {
	t.Helper()

	if !dashboardDispatchHistoryContainsTrace(dashboard, dispatchID, traceID) {
		t.Fatalf("dashboard dispatch history missing dispatch %s for trace %s", dispatchID, traceID)
	}
	attemptsByDispatch := mapValue(dashboard.Runtime.InferenceAttemptsByDispatchId)
	attempts := attemptsByDispatch[dispatchID]
	if len(attempts) != 3 {
		t.Fatalf("dashboard inference attempts for dispatch %s = %#v, want three retry attempts", dispatchID, attempts)
	}
	for _, attempt := range attempts {
		if attempt.DispatchId != dispatchID || attempt.InferenceRequestId == "" || attempt.Prompt == "" || attempt.RequestTime == "" {
			t.Fatalf("dashboard inference attempt missing request details: %#v", attempt)
		}
		if attempt.Attempt < 1 || attempt.Attempt > 3 {
			t.Fatalf("dashboard inference attempt number = %d, want 1..3", attempt.Attempt)
		}
		if attempt.Attempt < 3 && (attempt.Outcome != string(factoryapi.InferenceOutcomeFailed) || attempt.ErrorClass == "") {
			t.Fatalf("dashboard failed retry attempt = %#v, want FAILED with errorClass", attempt)
		}
		if attempt.Attempt == 3 && (attempt.Outcome != string(factoryapi.InferenceOutcomeSucceeded) || attempt.Response != "Step one recovered. COMPLETE" || attempt.ResponseTime == "") {
			t.Fatalf("dashboard successful retry attempt = %#v, want final response details", attempt)
		}
	}
}

func assertRawInferenceEventUsesContextDispatchIdentity(t *testing.T, event factoryapi.FactoryEvent, inferenceRequestID string) {
	t.Helper()

	raw := marshalFunctionalEventToRawObject(t, event)
	context := rawFunctionalEventContext(t, raw, event.Id)
	if dispatchID, ok := context["dispatchId"].(string); !ok || dispatchID == "" {
		t.Fatalf("raw inference event context.dispatchId = %#v, want non-empty string", context["dispatchId"])
	}

	payload := rawFunctionalEventPayload(t, raw, event.Id)
	if got, ok := payload["inferenceRequestId"].(string); !ok || got != inferenceRequestID {
		t.Fatalf("raw inference event payload.inferenceRequestId = %#v, want %q", payload["inferenceRequestId"], inferenceRequestID)
	}
	if _, ok := payload["dispatchId"]; ok {
		t.Fatalf("raw inference event payload unexpectedly carried retired dispatchId: %#v", payload)
	}
	if _, ok := payload["transitionId"]; ok {
		t.Fatalf("raw inference event payload unexpectedly carried retired transitionId: %#v", payload)
	}
}

func assertRawThinDispatchRequestEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	raw := marshalFunctionalEventToRawObject(t, event)
	context := rawFunctionalEventContext(t, raw, event.Id)
	if dispatchID, ok := context["dispatchId"].(string); !ok || dispatchID == "" {
		t.Fatalf("raw dispatch request context.dispatchId = %#v, want non-empty string", context["dispatchId"])
	}

	payload := rawFunctionalEventPayload(t, raw, event.Id)
	if _, ok := payload["dispatchId"]; ok {
		t.Fatalf("raw dispatch request payload unexpectedly carried retired dispatchId: %#v", payload)
	}
	if _, ok := payload["worker"]; ok {
		t.Fatalf("raw dispatch request payload unexpectedly carried retired worker copy: %#v", payload)
	}
	if _, ok := payload["workstation"]; ok {
		t.Fatalf("raw dispatch request payload unexpectedly carried retired workstation copy: %#v", payload)
	}
	if metadataValue, ok := payload["metadata"]; ok {
		metadata, ok := metadataValue.(map[string]any)
		if !ok {
			t.Fatalf("raw dispatch request metadata = %#v, want object", metadataValue)
		}
		if _, ok := metadata["requestId"]; ok {
			t.Fatalf("raw dispatch request metadata unexpectedly carried retired requestId: %#v", metadata)
		}
	}
}

func assertRawThinDispatchResponseEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	raw := marshalFunctionalEventToRawObject(t, event)
	context := rawFunctionalEventContext(t, raw, event.Id)
	if dispatchID, ok := context["dispatchId"].(string); !ok || dispatchID == "" {
		t.Fatalf("raw dispatch response context.dispatchId = %#v, want non-empty string", context["dispatchId"])
	}

	payload := rawFunctionalEventPayload(t, raw, event.Id)
	if _, ok := payload["dispatchId"]; ok {
		t.Fatalf("raw dispatch response payload unexpectedly carried retired dispatchId: %#v", payload)
	}
	if _, ok := payload["worker"]; ok {
		t.Fatalf("raw dispatch response payload unexpectedly carried retired worker copy: %#v", payload)
	}
	if _, ok := payload["workstation"]; ok {
		t.Fatalf("raw dispatch response payload unexpectedly carried retired workstation copy: %#v", payload)
	}
	if _, ok := payload["providerSession"]; ok {
		t.Fatalf("raw dispatch response payload unexpectedly carried retired providerSession: %#v", payload)
	}
	if _, ok := payload["diagnostics"]; ok {
		t.Fatalf("raw dispatch response payload unexpectedly carried retired diagnostics: %#v", payload)
	}
}

func marshalFunctionalEventToRawObject(t *testing.T, event factoryapi.FactoryEvent) map[string]any {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event %s: %v", event.Id, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal event %s: %v", event.Id, err)
	}
	return raw
}

func rawFunctionalEventContext(t *testing.T, raw map[string]any, eventID string) map[string]any {
	t.Helper()

	context, ok := raw["context"].(map[string]any)
	if !ok {
		t.Fatalf("raw event %s context = %#v, want object", eventID, raw["context"])
	}
	return context
}

func rawFunctionalEventPayload(t *testing.T, raw map[string]any, eventID string) map[string]any {
	t.Helper()

	payload, ok := raw["payload"].(map[string]any)
	if !ok {
		t.Fatalf("raw event %s payload = %#v, want object", eventID, raw["payload"])
	}
	return payload
}

func indexOfFunctionalDispatchEvent(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType, dispatchID string) int {
	return indexOfFunctionalDispatchEventAfter(events, eventType, dispatchID, 0)
}

func indexOfFunctionalDispatchEventAfter(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType, dispatchID string, start int) int {
	for i := start; i < len(events); i++ {
		if events[i].Type == eventType && stringValueFromFunctionalPtr(events[i].Context.DispatchId) == dispatchID {
			return i
		}
	}
	return -1
}

func indexOfFunctionalInferenceResponseForRequest(events []factoryapi.FactoryEvent, dispatchID, inferenceRequestID string) int {
	for i, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse || stringValueFromFunctionalPtr(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err == nil && payload.InferenceRequestId == inferenceRequestID {
			return i
		}
	}
	return -1
}

func dashboardDispatchHistoryContainsTrace(dashboard DashboardResponse, dispatchID, traceID string) bool {
	for _, dispatch := range sliceValue(dashboard.Runtime.Session.DispatchHistory) {
		if dispatch.DispatchId != dispatchID {
			continue
		}
		for _, workItem := range sliceValue(dispatch.WorkItems) {
			if stringValueFromFunctionalPtr(workItem.TraceId) == traceID {
				return true
			}
		}
	}
	return false
}

func assertInferenceEventsRecordedInArtifact(t *testing.T, liveEvents []factoryapi.FactoryEvent, recordedEvents []factoryapi.FactoryEvent) {
	t.Helper()

	recordedByID := make(map[string]factoryapi.FactoryEvent, len(recordedEvents))
	for _, event := range recordedEvents {
		recordedByID[event.Id] = event
	}
	for _, live := range liveEvents {
		if live.Type != factoryapi.FactoryEventTypeInferenceRequest && live.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		recorded, ok := recordedByID[live.Id]
		if !ok {
			t.Fatalf("recorded artifact missing inference event %s from live history; artifact events=%v", live.Id, functionalEventTypes(recordedEvents))
		}
		if recorded.Type != live.Type {
			t.Fatalf("recorded inference event %s = type %s, live type %s", live.Id, recorded.Type, live.Type)
		}
	}
}

func hasFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	return indexOfFunctionalEventType(events, eventType, 0) >= 0
}

type DashboardResponse struct {
	Runtime DashboardRuntime `json:"runtime"`
}

type DashboardRuntime struct {
	ActiveThrottlePauses          *[]DashboardThrottlePause               `json:"active_throttle_pauses,omitempty"`
	ActiveWorkstationNodeIds      *[]string                               `json:"active_workstation_node_ids,omitempty"`
	InFlightDispatchCount         int                                     `json:"in_flight_dispatch_count"`
	InferenceAttemptsByDispatchId *map[string]map[string]InferenceAttempt `json:"inference_attempts_by_dispatch_id,omitempty"`
	Session                       DashboardSessionRuntime                 `json:"session"`
}

type DashboardThrottlePause struct {
	AffectedTransitionIds    *[]string  `json:"affected_transition_ids,omitempty"`
	AffectedWorkTypeIds      *[]string  `json:"affected_work_type_ids,omitempty"`
	AffectedWorkerTypes      *[]string  `json:"affected_worker_types,omitempty"`
	AffectedWorkstationNames *[]string  `json:"affected_workstation_names,omitempty"`
	LaneId                   string     `json:"lane_id"`
	Model                    string     `json:"model"`
	PausedAt                 *time.Time `json:"paused_at,omitempty"`
	PausedUntil              time.Time  `json:"paused_until"`
	Provider                 string     `json:"provider"`
	RecoverAt                time.Time  `json:"recover_at"`
}

type InferenceAttempt struct {
	Attempt            int    `json:"attempt"`
	DispatchId         string `json:"dispatch_id"`
	DurationMillis     int64  `json:"duration_millis,omitempty"`
	ErrorClass         string `json:"error_class,omitempty"`
	ExitCode           *int   `json:"exit_code,omitempty"`
	InferenceRequestId string `json:"inference_request_id"`
	Outcome            string `json:"outcome,omitempty"`
	Prompt             string `json:"prompt"`
	RequestTime        string `json:"request_time"`
	Response           string `json:"response,omitempty"`
	ResponseTime       string `json:"response_time,omitempty"`
	TransitionId       string `json:"transition_id"`
	WorkingDirectory   string `json:"working_directory,omitempty"`
	Worktree           string `json:"worktree,omitempty"`
}

type DashboardSessionRuntime struct {
	CompletedCount      int                       `json:"completed_count"`
	CompletedWorkLabels *[]string                 `json:"completed_work_labels,omitempty"`
	DispatchHistory     *[]DashboardDispatchView  `json:"dispatch_history,omitempty"`
	ProviderSessions    *[]ProviderSessionAttempt `json:"provider_sessions,omitempty"`
}

type DashboardDispatchView struct {
	DispatchId string                  `json:"dispatch_id"`
	WorkItems  *[]DashboardWorkItemRef `json:"work_items,omitempty"`
}

type DashboardWorkItemRef struct {
	TraceId *string `json:"trace_id,omitempty"`
}

type ProviderSessionAttempt struct {
	DispatchId string `json:"dispatch_id"`
}

func (fs *functionalAPIServer) GetDashboard(t *testing.T) DashboardResponse {
	t.Helper()

	snapshot := fs.GetEngineStateSnapshot(t)
	events := fs.GetFactoryEvents(t)
	worldState, err := projections.ReconstructFactoryWorldState(events, snapshot.TickCount)
	if err != nil {
		t.Fatalf("reconstruct world state: %v", err)
	}
	worldView := projections.BuildFactoryWorldViewWithActiveThrottlePauses(worldState, snapshot.ActiveThrottlePauses)
	return dashboardResponseFromWorldView(worldView)
}

func (fs *functionalAPIServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return getGeneratedJSON[factoryapi.ListWorkResponse](t, fs.URL()+"/work")
}

func dashboardResponseFromWorldView(worldView interfaces.FactoryWorldView) DashboardResponse {
	return DashboardResponse{
		Runtime: DashboardRuntime{
			ActiveThrottlePauses:          dashboardThrottlePauses(worldView.Runtime.ActiveThrottlePauses),
			ActiveWorkstationNodeIds:      stringSlicePtr(worldView.Runtime.ActiveWorkstationNodeIDs),
			InFlightDispatchCount:         worldView.Runtime.InFlightDispatchCount,
			InferenceAttemptsByDispatchId: dashboardInferenceAttemptsByDispatchID(worldView.Runtime.InferenceAttemptsByDispatchID),
			Session: DashboardSessionRuntime{
				CompletedCount:      worldView.Runtime.Session.CompletedCount,
				CompletedWorkLabels: dashboardCompletedWorkLabels(worldView),
				DispatchHistory:     dashboardDispatchHistory(worldView.Runtime.Session.DispatchHistory),
				ProviderSessions:    dashboardProviderSessions(worldView.Runtime.Session.ProviderSessions),
			},
		},
	}
}

func dashboardThrottlePauses(input []interfaces.FactoryWorldThrottlePause) *[]DashboardThrottlePause {
	if len(input) == 0 {
		return nil
	}
	out := make([]DashboardThrottlePause, 0, len(input))
	for _, pause := range input {
		out = append(out, DashboardThrottlePause{
			AffectedTransitionIds:    stringSlicePtr(pause.AffectedTransitionIDs),
			AffectedWorkTypeIds:      stringSlicePtr(pause.AffectedWorkTypeIDs),
			AffectedWorkerTypes:      stringSlicePtr(pause.AffectedWorkerTypes),
			AffectedWorkstationNames: stringSlicePtr(pause.AffectedWorkstationNames),
			LaneId:                   pause.LaneID,
			Model:                    pause.Model,
			PausedAt:                 dashboardOptionalTimePtr(pause.PausedAt),
			PausedUntil:              pause.PausedUntil,
			Provider:                 pause.Provider,
			RecoverAt:                pause.RecoverAt,
		})
	}
	return &out
}

func dashboardInferenceAttemptsByDispatchID(input map[string]map[string]interfaces.FactoryWorldInferenceAttempt) *map[string]map[string]InferenceAttempt {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]map[string]InferenceAttempt, len(input))
	for dispatchID, attempts := range input {
		if len(attempts) == 0 {
			continue
		}
		converted := make(map[string]InferenceAttempt, len(attempts))
		for requestID, attempt := range attempts {
			converted[requestID] = InferenceAttempt{
				Attempt:            attempt.Attempt,
				DispatchId:         attempt.DispatchID,
				DurationMillis:     attempt.DurationMillis,
				ErrorClass:         attempt.ErrorClass,
				ExitCode:           copyIntPointer(attempt.ExitCode),
				InferenceRequestId: attempt.InferenceRequestID,
				Outcome:            attempt.Outcome,
				Prompt:             attempt.Prompt,
				RequestTime:        dashboardTimeString(attempt.RequestTime),
				Response:           attempt.Response,
				ResponseTime:       dashboardTimeString(attempt.ResponseTime),
				TransitionId:       attempt.TransitionID,
				WorkingDirectory:   attempt.WorkingDirectory,
				Worktree:           attempt.Worktree,
			}
		}
		out[dispatchID] = converted
	}
	return &out
}

func dashboardDispatchHistory(input []interfaces.FactoryWorldDispatchCompletion) *[]DashboardDispatchView {
	if len(input) == 0 {
		return nil
	}
	out := make([]DashboardDispatchView, 0, len(input))
	for _, dispatch := range input {
		out = append(out, DashboardDispatchView{
			DispatchId: dispatch.DispatchID,
			WorkItems:  dashboardDispatchWorkItems(dispatch),
		})
	}
	return &out
}

func dashboardDispatchWorkItems(dispatch interfaces.FactoryWorldDispatchCompletion) *[]DashboardWorkItemRef {
	workItems := make([]DashboardWorkItemRef, 0, len(dispatch.TraceIDs))
	for _, traceID := range dispatch.TraceIDs {
		traceID := traceID
		workItems = append(workItems, DashboardWorkItemRef{TraceId: &traceID})
	}
	if len(workItems) == 0 {
		return nil
	}
	return &workItems
}

func dashboardProviderSessions(input []interfaces.FactoryWorldProviderSessionRecord) *[]ProviderSessionAttempt {
	if len(input) == 0 {
		return nil
	}
	out := make([]ProviderSessionAttempt, 0, len(input))
	for _, session := range input {
		out = append(out, ProviderSessionAttempt{DispatchId: session.DispatchID})
	}
	return &out
}

func dashboardCompletedWorkLabels(worldView interfaces.FactoryWorldView) *[]string {
	labels := make([]string, 0)
	for _, dispatch := range worldView.Runtime.Session.DispatchHistory {
		for _, workItem := range dispatch.OutputWorkItems {
			label := workItem.DisplayName
			if label == "" {
				label = workItem.ID
			}
			if label != "" {
				labels = append(labels, label)
			}
		}
		if dispatch.TerminalWork != nil {
			label := dispatch.TerminalWork.WorkItem.DisplayName
			if label == "" {
				label = dispatch.TerminalWork.WorkItem.ID
			}
			if label != "" {
				labels = append(labels, label)
			}
		}
		for _, workItem := range dispatch.InputWorkItems {
			label := workItem.DisplayName
			if label == "" {
				label = workItem.ID
			}
			if label != "" {
				labels = append(labels, label)
			}
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return &labels
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}

func dashboardTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func dashboardOptionalTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
