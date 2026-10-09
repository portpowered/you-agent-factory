package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Independent explicit sessions share one immutable process graph. The public
// resume command consumes real temporary recordings; provider observations are
// scoped by each scenario's Factory Session and never execute paid work.
func TestAcceptedResponseSurvivesInvalidRecordingWithoutRetry(t *testing.T) {
	t.Parallel()
	runner := &invalidAcceptedResponseRunner{calls: make(map[string]int)}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	for _, shape := range []string{"missing", "preview", "inconsistent", "truncated", "missing envelope", "invalid proposed Work"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			config := acceptedResponsePauseFactory()
			if shape == "missing envelope" || shape == "invalid proposed Work" {
				config = acceptedEnvelopeFactory()
			}
			dir := support.ScaffoldFactory(t, config)
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
			for _, name := range []string{"process", "finish"} {
				support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			}
			session := uuid.NewString()
			payload := invalidAcceptedRecording(t, shape, session)
			if shape == "missing envelope" || shape == "invalid proposed Work" {
				payload = invalidAcceptedEnvelopeRecording(t, config, session, shape)
			}
			source := filepath.Join(dir, "accepted-source.json")
			if err := os.WriteFile(source, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			inputs := support.FakeInputs(ctx, []string{"you", "run", "--session", session, "--dir", dir, "--resume", source, "--quiet"})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err := process.Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), "cannot recover accepted dispatch") ||
				!strings.Contains(err.Error(), "will not be retried") {
				t.Fatalf("public resume = %v; stderr=%s", err, inputs.Stderr())
			}
			if strings.Contains(err.Error()+inputs.Stderr()+inputs.Stdout(), "PRIVATE-ACCEPTED-PAYLOAD") {
				t.Fatal("resume diagnostic exposed recorded output")
			}
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("failed resume changed its source recording")
			}
			runner.mu.Lock()
			calls := runner.calls[session]
			runner.mu.Unlock()
			if calls != 0 {
				t.Fatalf("failed accepted recovery executed provider %d times", calls)
			}
		})
	}
}

type invalidAcceptedResponseRunner struct {
	mu    sync.Mutex
	calls map[string]int
}

func (runner *invalidAcceptedResponseRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls[request.ExecutionScopeID]++
	runner.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider execution during accepted recovery")
}

func invalidAcceptedRecording(t *testing.T, shape, session string) []byte {
	t.Helper()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, false), &artifact); err != nil {
		t.Fatal(err)
	}
	snapshot, err := definitions.NewFactorySnapshot(acceptedResponsePauseFactory())
	if err != nil {
		t.Fatal(err)
	}
	base := artifact.RecordedAt
	artifact.Events[0] = seededReplayResumeEvent(t, "run-request", 0, 0, base, definitions.FactoryEventTypeRunRequest,
		definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base})
	dispatch := "dispatch-accepted"
	addEvent := func(kind definitions.FactoryEventType, payload any) {
		sequence := len(artifact.Events)
		event := seededReplayResumeEvent(t, fmt.Sprintf("accepted/%d", sequence), sequence, sequence, base.Add(time.Duration(sequence)*time.Second), kind, payload)
		event.Context.DispatchID = &dispatch
		event.Context.WorkIDs = &[]string{"work-seeded-replay-resume"}
		artifact.Events = append(artifact.Events, event)
	}
	addEvent(definitions.FactoryEventTypeDispatchRequest, definitions.DispatchRequestEventPayload{
		TransitionID: "process", Inputs: []definitions.DispatchConsumedWorkRef{{WorkID: "work-seeded-replay-resume"}},
	})
	addEvent(definitions.FactoryEventTypeModelRequest, workers.ModelRequestEventPayload{ModelRequestID: "accepted-model", Attempt: 1})
	if shape != "missing" {
		preview := "PRIVATE-ACCEPTED-PAYLOAD diagnostic only"
		response := workers.ModelResponseEventPayload{
			ModelRequestID: "accepted-model", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputPreview: &preview,
		}
		if shape == "inconsistent" {
			content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "PRIVATE-ACCEPTED-PAYLOAD complete"}}
			response.OutputContent = &content
			response.ModelRequestID = "different-model-request"
		}
		if shape == "complete" {
			content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "accepted recorded output COMPLETE"}}
			response.OutputContent = &content
		}
		if shape == "truncated" {
			content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: `{"output":"PRIVATE-ACCEPTED-PAYLOAD`}}
			response.OutputContent = &content
		}
		addEvent(definitions.FactoryEventTypeModelResponse, response)
	}
	addEvent(definitions.FactoryEventTypeAgentRunResponse, workers.AgentRunResponseEventPayload{AgentRunID: dispatch + "/agent-run/1", Outcome: "ACCEPTED"})
	for index := range artifact.Events {
		artifact.Events[index].Context.SessionID = &session
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func invalidAcceptedEnvelopeRecording(t *testing.T, config map[string]any, session, shape string) []byte {
	t.Helper()
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(acceptedEnvelopeRecording(t, config, session, "text"), &artifact); err != nil {
		t.Fatal(err)
	}
	events := artifact.Events[:0]
	for _, event := range artifact.Events {
		if event.Type == definitions.FactoryEventTypeInferenceResponse {
			if shape == "missing envelope" {
				continue
			}
			var response workers.InferenceResponseEventPayload
			if err := event.DecodePayload(&response); err != nil {
				t.Fatal(err)
			}
			raw := strings.ReplaceAll(*response.Response, `"workTypeId":"task"`, `"workTypeId":"PRIVATE-ACCEPTED-PAYLOAD-unknown-type"`)
			response.Response = &raw
			var err error
			event.Payload, err = json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
		}
		events = append(events, event)
	}
	artifact.Events = events
	for index := range artifact.Events {
		artifact.Events[index].Context.Sequence = index
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestAcceptedResponseSurvivesRecordedCompletionWithoutRedispatch(t *testing.T) {
	t.Parallel()
	reusable := newSeededReplayResumeProcess(t)
	for _, name := range []string{"first logical session", "independent logical session"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, acceptedResponsePauseFactory())
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
			for _, station := range []string{"process", "finish"} {
				support.WriteWorkstationConfig(t, dir, station, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			}
			session := uuid.NewString()
			payload := invalidAcceptedRecording(t, "complete", session)
			source := filepath.Join(dir, "accepted-source.json")
			if err := os.WriteFile(source, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			running := reusable.runForSession(t, dir, source, session, "--resume", source, "--record", filepath.Join(dir, "recovered.json"))
			// Completion events precede publication of the final runtime snapshot.
			// Observe the public terminal projection, which has no combined event
			// signal, before reading Work and the already-retained event history.
			support.WaitForSessionTerminalStatus(t, running.url, session, time.Minute)
			listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(running.url, session, "/work"))
			if !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", "task:complete") {
				t.Fatalf("recovered Work = %#v, want task:complete", listed.Results)
			}
			completions, originalRequests, processRequests := 0, 0, 0
			for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, session) {
				if string(event.Type) == "DISPATCH_REQUEST" {
					request, err := event.Payload.AsDispatchRequestEventPayload()
					if err != nil {
						t.Fatal(err)
					}
					if request.TransitionId == "process" {
						processRequests++
					}
				}
				if event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-accepted" {
					continue
				}
				switch string(event.Type) {
				case "DISPATCH_REQUEST":
					originalRequests++
				case "DISPATCH_RESPONSE":
					completions++
					response, err := event.Payload.AsDispatchResponseEventPayload()
					if err != nil || response.Output == nil || *response.Output != "accepted recorded output COMPLETE" {
						t.Fatalf("recovered response output = %#v, %v", response.Output, err)
					}
				case "DISPATCH_INTERRUPTED":
					t.Fatal("accepted dispatch was classified as interrupted")
				}
			}
			if completions != 1 || originalRequests != 1 || processRequests != 1 {
				t.Fatalf("original requests=%d completions=%d process requests=%d, want one recorded request and one recovered completion", originalRequests, completions, processRequests)
			}
			running.daemon.Stop(t)
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("accepted recovery changed its source recording")
			}
		})
	}
}
