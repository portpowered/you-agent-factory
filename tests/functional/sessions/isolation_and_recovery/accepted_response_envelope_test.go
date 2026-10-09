package isolation_and_recovery_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Generations serialize only within their logical board; independent sessions
// use one shared process, isolated profiles and public resume/read boundaries.
func TestAcceptedResponseSurvivesEnvelopeAndRepeatedReopen(t *testing.T) {
	t.Parallel()
	reusable := newSeededReplayResumeProcess(t)
	for _, shape := range []string{"text", "typed JSON"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			config := acceptedEnvelopeFactory()
			dir := support.ScaffoldFactory(t, config)
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\noutcomeFormat: decision-envelope\n---\n{{ (index .Inputs 0).Payload }}\n")
			session := uuid.NewString()
			payload := acceptedEnvelopeRecording(t, config, session, shape)
			source := filepath.Join(dir, "source.json")
			if err := os.WriteFile(source, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			var childID string
			for generation := 0; generation < 2; generation++ {
				successor := filepath.Join(dir, fmt.Sprintf("generation-%d.json", generation))
				running := reusable.runForSession(t, dir, source, session, "--resume", source, "--record", successor)
				// The terminal public projection publishes after completion events;
				// no combined HTTP readiness/completion signal exists.
				support.WaitForSessionTerminalStatus(t, running.url, session, time.Minute)
				childID = assertAcceptedEnvelopeWork(t, running, childID)
				assertAcceptedEnvelopeHistory(t, running)
				running.daemon.Stop(t)
				if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
					t.Fatal("resume altered its source recording")
				}
				source = successor
				payload = mustReadSeededReplayArtifact(t, source)
			}
		})
	}
}

func acceptedEnvelopeFactory() map[string]any {
	config := processExecuteRuntimeOpeningFactoryConfig()
	config["resources"] = []map[string]any{{"name": "accepted-slot", "capacity": 1}}
	station := config["workstations"].([]map[string]any)[0]
	station["resources"] = []map[string]any{{"name": "accepted-slot", "capacity": 1}}
	station["outcomeFormat"] = "decision-envelope"
	station["outputSchema"] = `{"type":"object","properties":{"answer":{"type":"integer"}},"required":["answer"]}`
	return config
}

func acceptedEnvelopeRecording(t *testing.T, config map[string]any, session, shape string) []byte {
	t.Helper()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(invalidAcceptedRecording(t, "complete", session), &artifact); err != nil {
		t.Fatal(err)
	}
	snapshot, err := definitions.NewFactorySnapshot(config)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Events[0] = seededReplayResumeEvent(t, "run-request", 0, 0, artifact.RecordedAt,
		definitions.FactoryEventTypeRunRequest, definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: artifact.RecordedAt})
	artifact.Events[0].Context.SessionID = &session
	for index := range artifact.Events {
		if artifact.Events[index].Type == definitions.FactoryEventTypeDispatchRequest {
			var request definitions.DispatchRequestEventPayload
			if err := artifact.Events[index].DecodePayload(&request); err != nil {
				t.Fatal(err)
			}
			request.Resources = &[]definitions.DispatchResourceRef{{Name: "accepted-slot", Capacity: 1}}
			artifact.Events[index].Payload, _ = json.Marshal(request)
		}
	}
	modelIndex := len(artifact.Events) - 2
	primary := `{"answer":42}`
	content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: primary}}
	if shape == "typed JSON" {
		content = []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(primary)}}
	}
	model := artifact.Events[modelIndex]
	model.Payload, err = json.Marshal(workers.ModelResponseEventPayload{
		ModelRequestID: "accepted-model", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputContent: &content,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(definitions.DecisionEnvelope{
		Decision: "ACCEPTED", Feedback: "recovered feedback", Output: primary,
		RecordedOutputWork: []work.FactoryWorkItem{{WorkTypeID: "task", State: "complete", DisplayName: "recovered child",
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "child content"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(envelope)
	request := model
	request.Id, request.Type = "accepted-inference-request", definitions.FactoryEventTypeInferenceRequest
	request.Payload, _ = json.Marshal(workers.InferenceRequestEventPayload{InferenceRequestID: "accepted-inference", Attempt: 1})
	response := model
	response.Id, response.Type = "accepted-inference-response", definitions.FactoryEventTypeInferenceResponse
	response.Payload, _ = json.Marshal(workers.InferenceResponseEventPayload{
		InferenceRequestID: "accepted-inference", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw,
	})
	accepted := artifact.Events[len(artifact.Events)-1]
	artifact.Events = append(artifact.Events[:modelIndex:modelIndex], request, response, model, accepted)
	for index := range artifact.Events {
		artifact.Events[index].Context.Sequence = index
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertAcceptedEnvelopeWork(t *testing.T, running seededReplayResumeRun, previousChildID string) string {
	t.Helper()
	assertAcceptedResourceReleased(t, running)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(running.url, running.sessionID, "/work"))
	// Explicit output Work replaces the default output token on the authored
	// arc. The original consumed Work remains visible in dispatch lineage.
	if len(listed.Results) != 1 || listed.Results[0].Name != "recovered child" {
		t.Fatalf("recovered envelope Work = %#v, want one child", listed.Results)
	}
	for _, item := range listed.Results {
		if item.WorkId == nil || *item.WorkId == "work-seeded-replay-resume" {
			continue
		}
		if previousChildID != "" && *item.WorkId != previousChildID {
			t.Fatal("second reopen replaced the generated child identity")
		}
		if !support.HasWorkAtCustomerState(listed, *item.WorkId, "task:complete") {
			t.Fatal("generated child lost its configured terminal state")
		}
		assertAcceptedEnvelopeChildContent(t, item)
		return *item.WorkId
	}
	t.Fatal("generated child is missing")
	return ""
}

func assertAcceptedEnvelopeChildContent(t *testing.T, item factoryapi.Work) {
	t.Helper()
	if item.StructuredResult == nil || item.Tags == nil || (*item.Tags)["_source_dispatch_id"] != "dispatch-accepted" {
		t.Fatalf("generated child lost structured result or parent lineage: %#v", item)
	}
	structured, err := json.Marshal(item.StructuredResult)
	if err != nil || string(structured) != `{"answer":42}` || item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("recovered child content/result = %#v, %v", item, err)
	}
	text, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil || text.Text != "child content" {
		t.Fatalf("recovered child content = %#v, %v", text, err)
	}
}

func assertAcceptedEnvelopeHistory(t *testing.T, running seededReplayResumeRun) {
	t.Helper()
	requests, completions := 0, 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID) {
		if string(event.Type) == "DISPATCH_REQUEST" && (event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-accepted") {
			t.Fatal("reopen executed a new dispatch")
		}
		if event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-accepted" {
			continue
		}
		switch string(event.Type) {
		case "DISPATCH_REQUEST":
			requests++
		case "DISPATCH_RESPONSE":
			completions++
			assertAcceptedEnvelopeResponse(t, event)
		case "DISPATCH_INTERRUPTED":
			t.Fatal("accepted turn was interrupted on reopen")
		}
	}
	if requests != 1 || completions != 1 {
		t.Fatalf("accepted requests=%d completions=%d, want one each", requests, completions)
	}
}

func assertAcceptedEnvelopeResponse(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	response, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil || response.Feedback == nil || *response.Feedback != "recovered feedback" ||
		response.Output == nil || *response.Output != `{"answer":42}` {
		t.Fatalf("recovered envelope response = %#v, %v", response, err)
	}
	if response.OutputResources == nil || len(*response.OutputResources) != 1 || (*response.OutputResources)[0].Name != "accepted-slot" || (*response.OutputResources)[0].Capacity != 1 {
		t.Fatalf("recovered resource release = %#v, want one accepted-slot", response.OutputResources)
	}
}

func assertAcceptedResourceReleased(t *testing.T, running seededReplayResumeRun) {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, running.url+"/factory-sessions/"+running.sessionID)
	session, err := response.AsFactorySession()
	if err != nil || session.Runtime.Petri == nil {
		t.Fatalf("read restored marking: %v", err)
	}
	available := 0
	for _, token := range session.Runtime.Petri.Marking {
		if token.PlaceId == "accepted-slot:available" {
			available++
		}
	}
	if available != 1 {
		t.Fatalf("available resources after recovery = %d, want one", available)
	}
}

func TestAcceptedResponseSurvivesTrueInterruptionRetry(t *testing.T) {
	t.Parallel()
	reusable := newSeededReplayResumeProcess(t)
	for _, modelSuccess := range []bool{false, true} {
		t.Run(fmt.Sprintf("model success=%t", modelSuccess), func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, acceptedResponsePauseFactory())
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
			for _, station := range []string{"process", "finish"} {
				support.WriteWorkstationConfig(t, dir, station, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			}
			session := uuid.NewString()
			var artifact definitions.ReplayArtifact
			if err := json.Unmarshal(invalidAcceptedRecording(t, "complete", session), &artifact); err != nil {
				t.Fatal(err)
			}
			artifact.Events = artifact.Events[:len(artifact.Events)-1]
			if !modelSuccess {
				artifact.Events = artifact.Events[:len(artifact.Events)-1]
			}
			payload, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "interrupted.json")
			if err := os.WriteFile(source, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			running := reusable.runForSession(t, dir, source, session, "--resume", source, "--record", filepath.Join(dir, "retry.json"))
			support.WaitForSessionTerminalStatus(t, running.url, session, time.Minute)
			listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(running.url, session, "/work"))
			if !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", "task:complete") {
				t.Fatalf("true interruption retry did not complete Work: %#v", listed.Results)
			}
			assertAcceptedInterruptionRetryHistory(t, running)
			running.daemon.Stop(t)
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("interruption retry altered the source recording")
			}
		})
	}
}

func assertAcceptedInterruptionRetryHistory(t *testing.T, running seededReplayResumeRun) {
	t.Helper()
	interruptions, retries := 0, 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID) {
		if string(event.Type) == "DISPATCH_INTERRUPTED" {
			interruptions++
			response, err := event.Payload.AsDispatchInterruptedEventPayload()
			if err != nil || !response.RetryPlanned || response.Reason != "daemon restart interrupted process-bound attempt" {
				t.Fatalf("restart interruption = %#v, %v", response, err)
			}
		}
		if string(event.Type) == "DISPATCH_REQUEST" {
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionId == "process" && event.Context.DispatchId != nil && *event.Context.DispatchId != "dispatch-accepted" {
				retries++
			}
		}
	}
	if interruptions != 1 || retries != 1 {
		t.Fatalf("true interruption events=%d retries=%d, want one each", interruptions, retries)
	}
}
