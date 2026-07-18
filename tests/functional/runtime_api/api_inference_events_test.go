package runtime_api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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

func TestInferenceEvents_RootRunHTTPStreamCorrelatesProviderAttempts(t *testing.T) {
	support.SkipLongFunctional(t, "slow root-run inference-event stream sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Step one done. COMPLETE")},
		workers.CommandResult{Stdout: []byte("Step two done. COMPLETE")},
	)
	host, stream := startWorkerOverrideRootRunHost(t, dir, true, wire.FunctionalEdges{
		ProviderCommandRunner: runner,
	})

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "Provider inference stream",
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "correlate provider attempts",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	assertHTTPInferenceSuccessSequence(t, stream, traceID, "step-one", "step-two")
	assertTerminalWorkerOverrideWork(t, host.Endpoint(), traceID, "complete")
}

func assertHTTPInferenceSuccessSequence(
	t *testing.T,
	stream *factoryEventHTTPStream,
	traceID string,
	wantTransitions ...string,
) {
	t.Helper()

transitionLoop:
	for _, wantTransition := range wantTransitions {
		deadline := time.Now().Add(5 * time.Second)
		var request factoryapi.InferenceRequestEventPayload
		var requestDispatchID string
		responseSeen := false
		for time.Now().Before(deadline) {
			event := stream.next(time.Until(deadline))
			switch event.Type {
			case factoryapi.FactoryEventTypeInferenceRequest:
				if !functionalEventContextContainsTrace(event, traceID) {
					continue
				}
				var err error
				request, err = event.Payload.AsInferenceRequestEventPayload()
				if err != nil {
					t.Fatalf("decode INFERENCE_REQUEST for %s: %v", wantTransition, err)
				}
				requestDispatchID = stringValueFromFunctionalPtr(event.Context.DispatchId)
				assertRawInferenceEventUsesContextDispatchIdentity(t, event, request.InferenceRequestId)
			case factoryapi.FactoryEventTypeInferenceResponse:
				if request.InferenceRequestId == "" || stringValueFromFunctionalPtr(event.Context.DispatchId) != requestDispatchID {
					continue
				}
				response, err := event.Payload.AsInferenceResponseEventPayload()
				if err != nil {
					t.Fatalf("decode INFERENCE_RESPONSE for %s: %v", wantTransition, err)
				}
				if response.InferenceRequestId != request.InferenceRequestId || response.Attempt != 1 || response.Outcome != factoryapi.InferenceOutcomeSucceeded {
					t.Fatalf("INFERENCE_RESPONSE for %s = %#v, want correlated first-attempt success", wantTransition, response)
				}
				assertRawInferenceEventUsesContextDispatchIdentity(t, event, response.InferenceRequestId)
				responseSeen = true
			case factoryapi.FactoryEventTypeDispatchResponse:
				if !functionalEventContextContainsTrace(event, traceID) {
					continue
				}
				payload, err := event.Payload.AsDispatchResponseEventPayload()
				if err != nil {
					t.Fatalf("decode DISPATCH_RESPONSE for %s: %v", wantTransition, err)
				}
				if payload.TransitionId != wantTransition || payload.Outcome != factoryapi.WorkOutcomeAccepted {
					t.Fatalf("DISPATCH_RESPONSE = transition %q outcome %q, want %q/ACCEPTED", payload.TransitionId, payload.Outcome, wantTransition)
				}
				if !responseSeen || stringValueFromFunctionalPtr(event.Context.DispatchId) != requestDispatchID {
					t.Fatalf("DISPATCH_RESPONSE for %s was not preceded by correlated inference request/response", wantTransition)
				}
				continue transitionLoop
			}
		}
		t.Fatalf("canonical session stream did not expose inference and dispatch success for transition %q", wantTransition)
	}
}

func functionalEventContextContainsTrace(event factoryapi.FactoryEvent, traceID string) bool {
	if event.Context.TraceIds == nil {
		return false
	}
	for _, candidate := range *event.Context.TraceIds {
		if candidate == traceID {
			return true
		}
	}
	return false
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
