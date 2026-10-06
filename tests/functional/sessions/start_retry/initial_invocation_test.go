package start_retry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func newInitialOpeningChildScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := map[string]any{"name": "initial-opening-child"}
	// Only this explicitly opened Factory has this resource. Looking up the
	// process's default runtime or the active peer cannot admit this child.
	config["resources"] = []map[string]any{{"id": "opening-child-slot", "name": "child-slot", "capacity": 1}}
	config["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{"name": "prompt", "required": false,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}}}},
	}
	config["orchestrator"] = map[string]any{
		"kind": "JAVASCRIPT",
		"javascript": map[string]any{
			"argsSchema": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "additionalProperties": false},
			"inlineSource": map[string]any{
				"encoding": "utf-8",
				"inline": `return (async function () {
  return await agent.run({prompt: "prove opened session child", label: "opening-child",
    modelProvider: "codex", model: "gpt-5-codex", resourceId: "opening-child-slot"});
})();`,
			},
		},
	}
	// The peer remains a Petri Factory so its existing public Work/history
	// observers distinguish the candidate's child execution from peer Work.
	scenario := newInitialOpeningScenario(t)
	scenario.candidateDir = support.ScaffoldFactory(t, config)
	return scenario
}

func testInitialOpeningChildInvocation(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, effects *initialOpeningEffects, serverURL string) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", "invoke the opened session child",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI child invocation: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode invocation response: %v stdout=%s", err, inputs.Stdout())
	}
	if response.SessionId == nil || *response.SessionId == "" {
		t.Fatalf("child invocation omitted durable session identity: %#v", response)
	}
	if !strings.Contains(inputs.Stdout(), "initial opening COMPLETE") {
		t.Fatalf("child failed to return selected provider output: %s effects=%v", inputs.Stdout(), effects.forScenario(scenario))
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("child invocation status = %s, want COMPLETED", response.Status)
	}
	read, err := sessions.GetSession(t.Context(), *response.SessionId)
	if err != nil || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("read durable child completion: %#v, %v", read, err)
	}
	assertInitialChildResponses(t, sessions, *response.SessionId)
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func assertInitialChildResponses(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	// The legacy response subscription selects only live sessions. This public
	// cursor selects the completed durable child's retained progress instead.
	subscription, err := sessions.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("subscribe durable child responses: %v", err)
	}
	defer subscription.Cursor.Detach()
	events, err := subscription.Cursor.Next(t.Context())
	if err != nil {
		t.Fatalf("read retained child responses: %v", err)
	}
	var messages, terminals int
	var lastSequence int64
	for _, event := range events {
		if event.FactorySessionID != sessionID || event.Sequence <= lastSequence || event.DispatchID == "" {
			t.Fatalf("child response lost identity or order: session=%s dispatch=%s sequence=%d after=%d", event.FactorySessionID, event.DispatchID, event.Sequence, lastSequence)
		}
		lastSequence = event.Sequence
		if event.Provenance.NativeEventType == "STREAM_COMPLETED" && event.Phase == factorysessions.ResponseEventPhaseCompleted {
			terminals++
		}
		if assertInitialChildNativeMessage(t, event) {
			messages++
		}
	}
	if messages != 1 || terminals != 1 {
		t.Fatalf("retained child responses: native messages=%d terminal completions=%d, want one each", messages, terminals)
	}
}

func assertInitialChildNativeMessage(t *testing.T, event factorysessions.FactoryResponseEvent) bool {
	t.Helper()
	// Compatibility deltas may repeat text from the provider's snapshot.
	// Native provenance distinguishes callback delivery from result shaping.
	if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseCompleted || event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
		return false
	}
	var message factorysessions.ResponseEventMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		t.Fatalf("decode provider message: %v", err)
	}
	if event.Provenance.Provider != "codex" || len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != "initial opening COMPLETE" {
		t.Fatalf("child provider message = %s provenance=%#v", event.Payload, event.Provenance)
	}
	return true
}

// This public journey replaces the composed engine/dispatch unit fixture.
func testInitialOpeningAttributedHistory(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, serverURL string) {
	t.Helper()
	peerBefore := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	for _, sessionID := range []string{scenario.candidateID, scenario.peerID} {
		requestID := "request-" + sessionID
		ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
		stream, err := sessions.SubscribeFactoryEventsForSession(ctx, sessionID, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		inputs := support.FakeInputs(ctx, []string{"you", "--json", "--server", serverURL, "--session", sessionID,
			"submit", "batch", fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"workId":%q,"name":"attributed","workTypeName":"task","payload":{"title":"independent history"}}]}`, requestID, "work-"+sessionID)})
		inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
		inputs.Input.WorkingDirectory = scenario.candidateDir
		if err := process.Execute(inputs.Input); err != nil {
			cancel()
			t.Fatalf("submit %s: %v stdout=%s stderr=%s", sessionID, err, inputs.Stdout(), inputs.Stderr())
		}
		assertInitialOpeningAttributedEvents(t, ctx, stream, sessionID, requestID)
		assertInitialOpeningWorkProjection(t, process, scenario, serverURL, sessionID, requestID)
		cancel()
		if sessionID == scenario.candidateID {
			assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerBefore)
		}
	}
}

func assertInitialOpeningAttributedEvents(t *testing.T, ctx context.Context, stream *factorydefinitions.FactoryEventStream, sessionID, requestID string) {
	t.Helper()
	counts := map[factorydefinitions.FactoryEventType]int{}
	dispatchID := ""
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				t.Fatal("event stream closed before attributed completion")
			}
			if !matchesInitialOpeningRequest(event, requestID, dispatchID) {
				continue
			}

			if event.Type == factorydefinitions.FactoryEventTypeDispatchRequest {
				if event.Context.DispatchID == nil || *event.Context.DispatchID == "" {
					t.Fatal("attributed dispatch omitted its identity")
				}
				dispatchID = *event.Context.DispatchID
			}
			if event.Context.SessionID == nil || *event.Context.SessionID != sessionID {
				t.Fatalf("request %s reached another session: %#v", requestID, event.Context)
			}
			counts[event.Type]++
			if event.Type == factorydefinitions.FactoryEventTypeDispatchResponse {
				assertInitialOpeningCompletedEvents(t, counts, stream.History, requestID)

				return
			}
		case <-ctx.Done():
			t.Fatalf("attributed submission/dispatch completion: %v counts=%v", ctx.Err(), counts)
		}
	}
}

func testNestedInitialOpening(t *testing.T, sessions factorysessions.Service, process support.Process, serverURL, workspace, home string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "session", "create", "--dir", workspace, "--init-new-factory"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workspace
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("create nested workspace: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var created struct {
		FolderPath string `json:"folderPath"`
		Session    struct {
			ID         string `json:"id"`
			FolderPath string `json:"folderPath"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &created); err != nil {
		t.Fatalf("decode created session: %v stdout=%s", err, inputs.Stdout())
	}
	if created.FolderPath != workspace || created.Session.FolderPath != workspace || created.Session.ID == "" {
		t.Fatalf("nested session identity = %#v, want workspace %s", created, workspace)
	}
	t.Cleanup(func() {
		_, err := sessions.Control(context.Background(), factorysessions.SessionControlRequest{SessionID: created.Session.ID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose})
		if err != nil {
			t.Error(err)
		}
	})
	read, err := sessions.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: created.Session.ID, Mode: factorysessions.SessionOperationModeLive})
	if err != nil || read.Session.FolderPath != workspace {
		t.Fatalf("read nested session = %#v, %v", read, err)
	}
	response, err := http.Get(serverURL + "/factory-sessions/" + url.PathEscape(created.Session.ID) + "/factory")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var current struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil || response.StatusCode != http.StatusOK || current.Name == "" || current.Name == "UNDEFINED" {
		t.Fatalf("nested Current Factory = %#v status=%d error=%v", current, response.StatusCode, err)
	}
}

func matchesInitialOpeningRequest(event factorydefinitions.FactoryEvent, requestID, dispatchID string) bool {
	if event.Context.RequestID != nil && *event.Context.RequestID == requestID {
		return true
	}
	return event.Type == factorydefinitions.FactoryEventTypeDispatchResponse && event.Context.DispatchID != nil && *event.Context.DispatchID == dispatchID
}

func runInitialOpeningCompatibilityScenarios(t *testing.T, sessions factorysessions.Service, process support.Process, serverURL, home string) {
	t.Helper()
	t.Run("nested workspace keeps folder identity and current Factory", func(t *testing.T) {
		t.Parallel()
		testNestedInitialOpening(t, sessions, process, serverURL, t.TempDir(), home)
	})
	t.Run("CLI submission dispatch and history stay independently attributed", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningAttributedHistory(t, sessions, process, newInitialOpeningScenario(t), serverURL)
	})
}

func assertInitialOpeningCompletedEvents(t *testing.T, counts map[factorydefinitions.FactoryEventType]int, before []factorydefinitions.FactoryEvent, requestID string) {
	t.Helper()
	for _, kind := range []factorydefinitions.FactoryEventType{factorydefinitions.FactoryEventTypeWorkRequest, factorydefinitions.FactoryEventTypeDispatchRequest, factorydefinitions.FactoryEventTypeDispatchResponse} {
		if counts[kind] != 1 {
			t.Fatalf("request event attribution/order = %v, want one submission, dispatch and completion", counts)
		}
	}
	for _, old := range before {
		if old.Context.RequestID != nil && *old.Context.RequestID == requestID {
			t.Fatal("fresh request already present in prior history")
		}
	}
}

func assertInitialOpeningWorkProjection(t *testing.T, process support.Process, scenario initialOpeningScenario, serverURL, sessionID, requestID string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "--session", sessionID, "work", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("read projected Work: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var projected struct {
		Results []struct {
			WorkID    string `json:"workId"`
			RequestID string `json:"requestId"`
			Name      string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &projected); err != nil {
		t.Fatalf("decode projected Work: %v stdout=%s", err, inputs.Stdout())
	}
	found := false
	for _, item := range projected.Results {
		if item.Name != "attributed" {
			continue
		}
		if item.RequestID != requestID || item.WorkID != "work-"+sessionID {
			t.Fatalf("projected Work crossed session/request identity: %#v", item)
		}
		found = true
	}
	if !found {
		t.Fatalf("projected history lost submitted Work: %s", inputs.Stdout())
	}
}

func testInitialOpeningBoardRecovery(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, effects *initialOpeningEffects, logs *observer.ObservedLogs, serverURL, recovery string) {
	t.Helper()
	peer := scenario.startPeer(t, sessions)
	request := scenario.request()
	request.Persistence = factorysessions.PersistencePolicyEnabled
	recordPath := filepath.Join(t.TempDir(), "board.jsonl")
	request.RuntimeSelection.Recording.RecordPath = recordPath
	startInitialOpeningSession(t, sessions, request)
	requestID := "completed-" + scenario.candidateID
	stream := initialOpeningHistory(t, sessions, scenario.candidateID)
	submitInitialRecoveryWork(t, process, scenario, serverURL, requestID, "completed", true)
	assertInitialOpeningAttributedEvents(t, t.Context(), stream, scenario.candidateID, requestID)
	// A nonterminal waiting state has no workstation input until explicitly
	// moved to init, so it can be persisted without opening a dispatch.
	submitInitialRecoveryWork(t, process, scenario, serverURL, "pending-"+scenario.candidateID, "pending", false)
	waitInitialRecoveryAdmission(t, stream, "pending-"+scenario.candidateID)
	before := initialOpeningHistory(t, sessions, scenario.candidateID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	// A successful close finalizes that runtime generation. Recovery reopens
	// the same logical session with a fresh runtime generation.
	request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	prepareInitialRecoveryArtifact(t, sessions, &request, scenario, recordPath, recovery)
	startInitialOpeningSession(t, sessions, request)
	if recovery == "missing" {
		entries := logs.FilterField(zap.String("session_id", scenario.candidateID)).FilterField(zap.String("recovery", "missing_board_recording_after_durable_state")).All()
		if len(entries) != 1 {
			t.Fatalf("missing-board warnings=%d, want one preserved-durable-state warning", len(entries))
		}
	} else {
		after := initialOpeningHistory(t, sessions, scenario.candidateID)
		for index, event := range before.History {
			if index >= len(after.History) || after.History[index].Id != event.Id {
				t.Fatalf("reopening lost selected prefix at %d", index)
			}
		}
		assertInitialRecoveryWorkStates(t, process, scenario, serverURL)
		move := support.FakeInputs(t.Context(), []string{"you", "--server", serverURL, "--session", scenario.candidateID,
			"work", "move", "pending-" + scenario.candidateID, "init"})
		move.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
		move.Input.WorkingDirectory = scenario.candidateDir
		if err := process.Execute(move.Input); err != nil {
			t.Fatalf("release recovered pending Work: %v %s", err, move.Stderr())
		}

	}
	// Running the successor must leave the completed Work terminal. Count the
	// exact owned external executions: one before close, pending only if its
	// board survived, and one new invocation after reopen.
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	want := 3
	if recovery == "missing" {
		want = 2
	}
	calls := effects.forScenario(scenario)
	if calls[filepath.Clean(scenario.candidateDir)+"|worker.run"] != want {
		t.Fatalf("recovery ran completed Work again or lost pending Work: %v want=%d", calls, want)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peer)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func submitInitialRecoveryWork(t *testing.T, process support.Process, scenario initialOpeningScenario, serverURL, requestID, label string, running bool) {
	t.Helper()
	state := "init"
	if !running {
		state = "waiting"
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "--session", scenario.candidateID, "submit", "batch",
		fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"workId":%q,"name":%q,"state":%q,"workTypeName":"task","payload":{"title":"recover"}}]}`, requestID, label+"-"+scenario.candidateID, label, state)})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit recovery Work (running=%v): %v %s", running, err, inputs.Stderr())
	}
}

func assertInitialRecoveryWorkStates(t *testing.T, process support.Process, scenario initialOpeningScenario, serverURL string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "--session", scenario.candidateID, "work", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatal(err)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &listed); err != nil {
		t.Fatal(err)
	}
	if !support.HasWorkAtCustomerState(listed, "completed-"+scenario.candidateID, "task:complete") || !support.HasWorkAtCustomerState(listed, "pending-"+scenario.candidateID, "task:waiting") {
		t.Fatalf("recovered Work state: %s", inputs.Stdout())
	}
}

func prepareInitialRecoveryArtifact(t *testing.T, sessions factorysessions.Service, request *factorysessions.SessionStartRequest, scenario initialOpeningScenario, recordPath, recovery string) {
	t.Helper()
	selectedPath := filepath.Join(filepath.Dir(recordPath), "board."+scenario.candidateID+".jsonl")
	original, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	// Initial compatibility recordings use the selected filename; explicit
	// current-board reads use the session-suffixed filename. Seed the selected
	// recovery destination from finalized public history, as when restoring a
	// trusted backup. This proves recovery of selected facts, not path naming.
	if err := os.WriteFile(selectedPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if recovery == "corrupt" {
		corrupt := []byte("{invalid board")
		if err := os.WriteFile(selectedPath, corrupt, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := sessions.Start(t.Context(), *request)
		if err == nil || !strings.Contains(err.Error(), "CORRUPT_HISTORY") {
			t.Fatalf("corrupt board error: %v", err)
		}
		assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
		after, err := os.ReadFile(selectedPath)
		if err != nil || !bytes.Equal(after, corrupt) {
			t.Fatalf("failed opening changed corrupt source: %v", err)
		}
		if err := os.WriteFile(selectedPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
	} else if recovery == "missing" {
		if err := os.Remove(selectedPath); err != nil {
			t.Fatal(err)
		}
		// Select a fresh missing-board destination for this successor. The
		// reusable Recordings process retains the old finalized writer cursor;
		// reusing that physical writer tests a separate recording lifecycle.
		request.RuntimeSelection.Recording.RecordPath = filepath.Join(t.TempDir(), "missing-board.jsonl")
	}
}

func waitInitialRecoveryAdmission(t *testing.T, stream *factorydefinitions.FactoryEventStream, requestID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				t.Fatal("pending Work stream closed")
			}
			if event.Type == factorydefinitions.FactoryEventTypeWorkRequest && event.Context.RequestID != nil && *event.Context.RequestID == requestID {
				return
			}
		case <-ctx.Done():
			t.Fatalf("pending Work admission: %v", ctx.Err())
		}
	}
}
