package restart_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Only generations of this customer's board serialize. The invoking lane
// builds the CLI once; the controlled SCRIPT_WORKER crosses a real OS boundary.
func TestAcceptedResponseSurvivesPlainRestart(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	repo, home := t.TempDir(), t.TempDir()
	config := boardPersistenceFactoryConfig()
	factory := filepath.Join(repo, "factory")
	if err := os.Rename(scaffoldBoardPersistenceFactory(t, config), factory); err != nil {
		t.Fatal(err)
	}
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", boardPersistenceWorkerConfig(currentRestartWorkerExecutable(t)))
	release := filepath.Join(repo, "release")
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	// A reference alone is deliberately insufficient authority for plain
	// reopening. Establish the real durable store through a customer process.
	bootstrap := startBoardPersistenceDaemon(t, binary, factory, home, "", release)
	batch := `{"requestId":"restart-request","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"accepted-work","name":"accepted","workTypeName":"task","state":"processing"},{"workId":"interrupted-work","name":"interrupted","workTypeName":"task","state":"processing"}]}`
	submitBoardPersistenceBatchThroughCLI(t, bootstrap, binary, factory, home, batch, "restart-request", 2)
	waitPlainBoardConfirmed(t, bootstrap.baseURL, map[string]string{"accepted-work": "complete", "interrupted-work": "complete"})
	shutdownPlainBoard(t, bootstrap)
	source, payload := seedAcceptedRestartBoard(t, repo, factory, config)
	want := map[string]string{"accepted-work": "complete", "interrupted-work": "complete"}
	for generation := 0; generation < 2; generation++ {
		// No --resume or --record: production selects the repository's board.
		daemon := startBoardPersistenceDaemon(t, binary, factory, home, "", release)
		listed := waitPlainBoardConfirmed(t, daemon.baseURL, want)
		if len(listed.Results) != 2 {
			t.Fatalf("generation %d has %d Work items, want two", generation, len(listed.Results))
		}
		assertAcceptedRestartHistory(t, daemon.baseURL)
		for _, item := range listed.Results {
			if boardPersistenceStringPointerValue(item.WorkId) == "accepted-work" {
				if item.Content == nil || len(*item.Content) != 1 {
					t.Fatal("accepted output missing after plain relaunch")
				}
				text, err := (*item.Content)[0].AsWorkTextContentPart()
				if err != nil || text.Text != "full accepted output" {
					t.Fatalf("accepted output changed: %#v, %v", text, err)
				}
			}
		}
		shutdownPlainBoard(t, daemon)
		assertPlainRestartFileUnchanged(t, source, payload)
	}
	t.Logf("I-1 artifact SHA256=%s source=%s: accepted output completed once, unanswered dispatch retried once, graceful plain relaunch preserved both", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead)
}

func seedAcceptedRestartBoard(t *testing.T, repo, factory string, config map[string]any) (string, []byte) {
	t.Helper()
	config["factoryDirectory"] = factory
	snapshot, err := definitions.NewFactorySnapshot(config)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	session := "~default"
	artifact := definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base}
	add := func(kind definitions.FactoryEventType, dispatch, workID string, payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		sequence := len(artifact.Events)
		event := definitions.FactoryEvent{Id: fmt.Sprintf("accepted-restart/%d", sequence), Type: kind,
			SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: raw,
			Context: definitions.FactoryEventContext{Sequence: sequence, Tick: sequence, EventTime: base.Add(time.Duration(sequence) * time.Second), SessionID: &session}}
		if dispatch != "" {
			event.Context.DispatchID = &dispatch
		}
		if workID != "" {
			event.Context.WorkIDs = &[]string{workID}
		}
		artifact.Events = append(artifact.Events, event)
	}
	add(definitions.FactoryEventTypeRunRequest, "", "", definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base})
	for _, name := range []string{"accepted", "interrupted"} {
		id := name + "-work"
		add(definitions.FactoryEventTypeWorkRequest, "", id, work.WorkRequestEventPayload{Type: work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{Name: name, WorkID: id, RequestID: "restart-request", WorkTypeID: "task", State: &work.WorkEventState{Name: "processing", Type: "PROCESSING"}}}})
		add(definitions.FactoryEventTypeDispatchRequest, name+"-dispatch", id, definitions.DispatchRequestEventPayload{
			TransitionID: "hold-processing", Inputs: []definitions.DispatchConsumedWorkRef{{WorkID: id}}})
	}
	content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "full accepted output"}}
	add(definitions.FactoryEventTypeModelRequest, "accepted-dispatch", "accepted-work", workers.ModelRequestEventPayload{ModelRequestID: "accepted-model", Attempt: 1})
	add(definitions.FactoryEventTypeModelResponse, "accepted-dispatch", "accepted-work", workers.ModelResponseEventPayload{ModelRequestID: "accepted-model", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputContent: &content})
	add(definitions.FactoryEventTypeAgentRunResponse, "accepted-dispatch", "accepted-work", workers.AgentRunResponseEventPayload{AgentRunID: "accepted-dispatch/agent-run/1", Outcome: "ACCEPTED"})
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repo, ".you-agent-factory")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "accepted-source.json")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	reference, _ := json.Marshal(map[string]string{"schemaVersion": "factory-sessions.current-board.v1", "factoryDirectory": factory,
		"factorySessionId": "~default", "artifactReference": source})
	if err := os.WriteFile(filepath.Join(root, "current-board.json"), reference, 0600); err != nil {
		t.Fatal(err)
	}
	return source, payload
}

func assertAcceptedRestartHistory(t *testing.T, baseURL string) {
	t.Helper()
	events, err := readBoardEvents(t.Context(), baseURL)
	if err != nil {
		t.Fatal(err)
	}
	acceptedRequests, acceptedCompletions, interruptions, retries := 0, 0, 0, 0
	for _, event := range events {
		id := boardPersistenceStringPointerValue(event.Context.DispatchId)
		switch string(event.Type) {
		case "DISPATCH_REQUEST":
			if id == "accepted-dispatch" {
				acceptedRequests++
			} else if id != "interrupted-dispatch" {
				assertAcceptedRestartRetryRequest(t, event)
				retries++
			}
		case "DISPATCH_RESPONSE":
			if id == "accepted-dispatch" {
				acceptedCompletions++
			}
		case "DISPATCH_INTERRUPTED":
			assertAcceptedRestartInterruption(t, event, id)
			interruptions++
		}
	}
	if acceptedRequests != 1 || acceptedCompletions != 1 || interruptions != 1 || retries != 1 {
		t.Fatalf("requests=%d accepted completions=%d interruptions=%d retries=%d, want one each", acceptedRequests, acceptedCompletions, interruptions, retries)
	}
}

func assertAcceptedRestartRetryRequest(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	request, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil || len(request.Inputs) != 1 || request.Inputs[0].WorkId != "interrupted-work" {
		t.Fatalf("unexpected executed dispatch: %#v, %v", request, err)
	}
}

func assertAcceptedRestartInterruption(t *testing.T, event factoryapi.FactoryEvent, id string) {
	t.Helper()
	response, err := event.Payload.AsDispatchInterruptedEventPayload()
	if id != "interrupted-dispatch" || err != nil || !response.RetryPlanned {
		t.Fatalf("accepted dispatch interrupted or retry lost: %s %#v %v", id, response, err)
	}
}
