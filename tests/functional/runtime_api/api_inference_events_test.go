package runtime_api

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event artifact sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordPath := filepath.Join(t.TempDir(), "inference-events.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-inference-events",
		WorkTypeID: "task",
		TraceID:    "trace-inference-events",
		Payload:    []byte(`{"title":"inspect inference events"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", recordPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	events := server.GetFactoryEvents(t)
	assertFirstInferenceAttemptOrder(t, events)
	server.Stop(t)
	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertInferenceEventsRecordedInArtifact(t, events, testutil.GeneratedFactoryEvents(t, artifact.Events))
}

func TestInferenceEvents_ScriptWorkersDoNotEmitInferenceEvents(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event script-worker sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-script-no-inference",
		WorkTypeID: "task",
		TraceID:    "trace-script-no-inference",
		Payload:    []byte("script input"),
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: support.NewStaticSuccessCommandRunner("script-output-ok"),
		},
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	events := server.GetFactoryEvents(t)
	if !hasFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest) ||
		!hasFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("script worker canonical events = %v, want dispatch lifecycle events", functionalEventTypes(events))
	}
	if hasFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest) ||
		hasFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceResponse) {
		t.Fatalf("script worker emitted inference events: %v", functionalEventTypes(events))
	}
}

func TestInferenceEvents_HTTPStreamAndPublicWorkCorrelateRetryAttempts(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event stream-projection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{
			{},
			{},
			{Content: "Step one recovered. COMPLETE"},
			{Content: "Step two done. COMPLETE"},
		},
		[]error{
			workerexecution.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timeout", nil),
			workerexecution.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider 500", nil),
			nil,
			nil,
		},
	)
	server := startFunctionalServerWithArgs(
		t,
		dir,
		false,
		nil,
		withProvider(provider),
	)

	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	_, _ = requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("Retrying inference stream"),
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "retry provider attempts",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	events := collectFunctionalEventsUntilDispatchCompletions(t, stream, 2, 10*time.Second)
	assertHTTPInferenceRetrySequence(t, events)
	assertInferenceTraceReachedPublicWork(t, server.ListWork(t), traceID)
}

func TestInferenceEvents_ThinEventSmoke_CapturesThinnedDispatchInferenceSequenceAndReconstructsViews(t *testing.T) {
	support.SkipLongFunctional(t, "slow inference-event thin-event sweep")
	smoke := newThinEventSmokeHarness(t)

	active := captureThinEventSmokeActiveSnapshot(t, smoke)
	assertThinEventSmokeActiveSnapshot(t, active)

	smoke.provider.ReleaseFirst()
	waitForFunctionalHarnessCompletion(t, smoke.server, 5*time.Second)

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

func assertInferenceTraceReachedPublicWork(t *testing.T, work factoryapi.ListWorkResponse, traceID string) {
	t.Helper()

	for _, item := range work.Results {
		if stringValueFromFunctionalPtr(item.TraceId) == traceID {
			return
		}
	}
	t.Fatalf("public Work listing = %#v, want trace %q", work.Results, traceID)
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

func (fs *functionalAPIServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(fs.URL(), "/work"))
}
