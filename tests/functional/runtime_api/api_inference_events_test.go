package runtime_api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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

func (fs *functionalAPIServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(fs.URL(), "/work"))
}
