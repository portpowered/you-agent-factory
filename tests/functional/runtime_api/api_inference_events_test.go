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

func (fs *functionalAPIServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(fs.URL(), "/work"))
}
