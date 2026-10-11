package start_retry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// The public close must cancel only the selected session's running worker.
// All four exact command edges enter before close. The three peers stay gated
// until their live routes and event generations are observed after cleanup.
func testSelectedProviderCancellation(t *testing.T, sessions factorysessions.Service, scenarios []initialOpeningScenario, gate *selectedProviderGate, baseURL string) {
	t.Helper()
	defer gate.unblock()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make([]chan error, len(scenarios))
	joinSelectedProviderCommands(t, cancel, gate, done)
	history := make([]*factorydefinitions.FactoryEventStream, len(scenarios))
	identities := make([]factoryapi.FactorySessionStreamIdentity, len(scenarios))
	retry := scenarios[0].request()
	retry.Persistence = factorysessions.PersistencePolicyEnabled
	recordPath := filepath.Join(t.TempDir(), "board.jsonl")
	retry.RuntimeSelection.Recording.RecordPath = recordPath
	for i, scenario := range scenarios {
		request := scenario.request()
		if i == 0 {
			request = retry
		}
		startInitialOpeningSession(t, sessions, request)
		identities[i] = selectedProviderStreamIdentity(t, baseURL, scenario.candidateID)
		done[i] = make(chan error, 1)
		go func() {
			defer close(done[i])
			done[i] <- selectedProviderInvoke(ctx, sessions, scenario.candidateID)
		}()
	}
	awaitSelectedProviders(t, ctx, gate)
	for i, scenario := range scenarios {
		history[i] = initialOpeningHistory(t, sessions, scenario.candidateID)
		if history[i].StreamGenerationID != identities[i].StreamGenerationID {
			t.Fatal("session route and retained history selected different generations")
		}
		for j := range i {
			if identities[j].StreamGenerationID == identities[i].StreamGenerationID {
				t.Fatal("explicit sessions shared an event generation")
			}
		}
		assertSelectedRunningWorker(t, baseURL, scenario.candidateID)
	}
	selected := support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(baseURL, scenarios[0].candidateID, "/work"))
	workID := *selected.Results[0].WorkId
	closeInitialOpeningSession(t, sessions, scenarios[0].candidateID)
	select {
	case err := <-done[0]:
		if err == nil {
			t.Fatal("closed candidate returned successful worker output")
		}
	case <-ctx.Done():
		t.Fatal("closing the candidate did not cancel its invocation")
	}
	assertInitialOpeningNotPublished(t, sessions, scenarios[0].candidateID)
	for i := 1; i < len(scenarios); i++ {
		scenario := scenarios[i]
		id := scenario.candidateID
		assertInitialOpeningHistoryPreserved(t, sessions, id, history[i])
		if !reflect.DeepEqual(selectedProviderStreamIdentity(t, baseURL, id), identities[i]) {
			t.Fatal("closing another session retargeted a peer stream")
		}
		assertGatedPeerWork(t, initialOpeningScenario{peerID: id}, baseURL, done[i])
		assertSelectedRunningWorker(t, baseURL, id)
	}
	gate.unblock()
	for i := 1; i < len(scenarios); i++ {
		awaitSelectedProviderCompletion(t, ctx, done[i])
		assertInitialOpeningHistoryPreserved(t, sessions, scenarios[i].candidateID, history[i])
		assertSelectedProviderResponses(t, ctx, sessions, scenarios[i].candidateID)
		if err := selectedProviderInvoke(ctx, sessions, scenarios[i].candidateID); err != nil {
			t.Fatal(err)
		}
	}
	// Restore the finalized board into a fresh runtime generation of the same
	// Session. The original canceled Work must execute again, rather than a
	// newly submitted invocation standing in for retryability.
	retry.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	prepareInitialRecoveryArtifact(t, sessions, &retry, scenarios[0], recordPath, "valid")
	startInitialOpeningSession(t, sessions, retry)
	assertCanceledWorkRetry(t, baseURL, scenarios[0].candidateID, workID)
}

func assertCanceledWorkRetry(t *testing.T, baseURL, sessionID, workID string) {
	t.Helper()
	listed, err := support.WaitForObservation(initialOpeningReadCeiling,
		func() (factoryapi.ListWorkResponse, error) {
			return support.GetJSON[factoryapi.ListWorkResponse](t,
				support.SessionWorkURL(baseURL, sessionID, "/work")), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			return support.HasWorkAtCustomerState(listed, workID, "task:complete")
		})
	if err != nil {
		t.Fatalf("original canceled Work %s did not retry to completion: %v work=%#v", workID, err, listed)
	}
	if len(listed.Results) != 1 || fmt.Sprint(listed.Results[0].Payload) != sessionID+" selected Work" {
		t.Fatalf("retry replaced the original Work or crossed Sessions: %#v", listed)
	}
	workers := support.ListSessionWorkerSessions(t, baseURL, sessionID, workID)
	if len(workers.Sessions) == 0 {
		t.Fatal("retried Work has no public Worker Session observation")
	}
	for _, worker := range workers.Sessions {
		if worker.FactorySessionId == nil || *worker.FactorySessionId != sessionID || worker.WorkId == nil || *worker.WorkId != workID {
			t.Fatalf("retried Worker lost original Work/Session identity: %#v", worker)
		}
	}
	assertCanceledWorkRetryEvents(t, baseURL, sessionID)
}

func assertCanceledWorkRetryEvents(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	completed := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		response, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("retry response crossed Sessions: %#v", event.Context)
		}
		if response.Outcome == factoryapi.WorkOutcomeAccepted {
			completed = response.Output != nil && strings.Contains(*response.Output, sessionID+" COMPLETE")
		}
	}
	if !completed {
		t.Fatal("retried Work has no attributed accepted provider output")
	}
}

// Read while the provider edge is held, so terminal output cannot substitute
// for a live, correctly attributed Worker observation during the overlap.
func assertSelectedRunningWorker(t *testing.T, baseURL, id string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, id, "/work"))
	if len(listed.Results) != 1 || listed.Results[0].WorkId == nil {
		t.Fatalf("gated Work for %s = %#v, want one identified Work", id, listed)
	}
	workers := support.ListSessionWorkerSessions(t, baseURL, id, *listed.Results[0].WorkId)
	if len(workers.Sessions) != 1 {
		t.Fatalf("gated Worker observations for %s = %#v, want one", id, workers)
	}
	worker := workers.Sessions[0]
	if worker.FactorySessionId == nil || *worker.FactorySessionId != id ||
		worker.WorkId == nil || *worker.WorkId != *listed.Results[0].WorkId ||
		worker.WorkerSessionId == "" || worker.AttemptId == "" || worker.EndedAt != nil ||
		worker.Provider == nil || *worker.Provider != "codex" ||
		worker.Model == nil || *worker.Model != "gpt-5-codex" {
		t.Fatalf("gated Worker observation lost selected identity/model: %#v", worker)
	}
}

func assertSelectedProviderResponses(t *testing.T, ctx context.Context, sessions factorysessions.Service, id string) {
	t.Helper()
	subscription, err := sessions.SubscribeResponses(ctx, factorysessions.SessionResponseSubscriptionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cursor.Detach()
	var lastSequence int64
	var messages int
	var observed []factorysessions.FactoryResponseEvent
	// Invocation completion is asserted by selectedProviderInvoke. The public
	// response cursor independently proves the provider's attributed message;
	// this live Work route does not promise the durable child's terminal frame.
	for messages == 0 {
		events, err := subscription.Cursor.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			observed = append(observed, event)
			if event.FactorySessionID != id || event.Sequence <= lastSequence || event.DispatchID == "" {
				t.Fatalf("peer response lost identity or ordering: %+v", event)
			}
			lastSequence = event.Sequence
			if assertSelectedProviderNativeMessage(t, id, event) {
				messages++
			}
		}
	}
	if messages != 1 {
		t.Fatalf("peer completed native response messages=%d, want one", messages)
	}
	assertSelectedProviderCursorReplay(t, ctx, sessions, id, observed)
}

func assertSelectedProviderNativeMessage(t *testing.T, id string, event factorysessions.FactoryResponseEvent) bool {
	t.Helper()
	if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseCompleted || event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
		return false
	}
	var message factorysessions.ResponseEventMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		t.Fatal(err)
	}
	if event.Provenance.Provider != "codex" || len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != id+" COMPLETE" {
		t.Fatalf("peer response crossed provider/session output: %s", event.Payload)
	}
	return true
}

func assertSelectedProviderCursorReplay(t *testing.T, ctx context.Context, sessions factorysessions.Service, id string, observed []factorysessions.FactoryResponseEvent) {
	t.Helper()
	if len(observed) < 2 {
		t.Fatal("selected response history has no acknowledged prefix and suffix")
	}
	reconnected, err := sessions.SubscribeResponses(ctx, factorysessions.SessionResponseSubscriptionRequest{
		SessionID: id, AfterSequence: observed[0].Sequence,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Cursor.Detach()
	remaining := observed[1:]
	for len(remaining) > 0 {
		events, err := reconnected.Cursor.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if len(remaining) == 0 {
				break
			}
			if !reflect.DeepEqual(event, remaining[0]) {
				t.Fatalf("selected cursor replay changed retained event: got %#v, want %#v", event, remaining[0])
			}
			remaining = remaining[1:]
		}
	}
}

func joinSelectedProviderCommands(t *testing.T, cancel context.CancelFunc, gate *selectedProviderGate, done []chan error) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		gate.unblock()
		for _, completion := range done {
			if completion == nil {
				continue
			}
			select {
			case <-completion:
			case <-time.After(initialOpeningReadCeiling):
				t.Error("owned explicit invocation did not join")
			}
		}
	})
}

func selectedProviderStreamIdentity(t *testing.T, baseURL, id string) factoryapi.FactorySessionStreamIdentity {
	t.Helper()
	session := support.GetJSON[factoryapi.FactorySession](t, baseURL+"/factory-sessions/"+id)
	if session.Id != id || session.Runtime.StreamIdentity == nil || session.Runtime.StreamIdentity.FactorySessionID != id || session.Runtime.StreamIdentity.StreamGenerationID == "" {
		t.Fatalf("selected session stream identity = %#v, want %s", session, id)
	}
	return *session.Runtime.StreamIdentity
}

func awaitSelectedProviderCompletion(t *testing.T, ctx context.Context, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("peer did not retain its selected result after candidate cleanup: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer invocation did not finish after release")
	}
}

func awaitSelectedProviders(t *testing.T, ctx context.Context, gate *selectedProviderGate) {
	t.Helper()
	seen := make(map[string]bool)
	for range len(gate.paths) {
		select {
		case request := <-gate.entered:
			id := gate.paths[request.WorkDir]
			if id == "" || seen[id] {
				t.Fatalf("overlapping command identity = %q, seen=%v", id, seen)
			}
			seen[id] = true
		case <-ctx.Done():
			t.Fatal("all workers did not reach their owned command gates")
		}
	}
}

func assertGatedPeerWork(t *testing.T, scenario initialOpeningScenario, baseURL string, peerDone <-chan error) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.peerID, "/work"))
	if len(listed.Results) != 1 || fmt.Sprint(listed.Results[0].Payload) != scenario.peerID+" selected Work" {
		t.Fatalf("peer Work changed during candidate cleanup: %#v", listed)
	}
	select {
	case err := <-peerDone:
		t.Fatalf("peer invocation ended before its gate was released: %v", err)
	default:
	}
}

// Select the failed listener through the public host request. The exact edge
// fails once; retry uses the same session identity and listener request.
const completionFailurePort = 24188

func testCompletionHostFailure(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, gate *selectedProviderGate, cause error, retryAPI *support.ProcessAPIServer) {
	t.Helper()
	defer gate.unblock()
	peer := scenario.request()
	peer.SessionID, peer.FolderPath = scenario.peerID, scenario.peerDir
	peer.RuntimeSelection.DefinitionSourcePath = scenario.peerDir + "/factory.json"
	peer.RuntimeSelection.ExecutionBaseDir, peer.RuntimeSelection.RuntimeInstanceID = scenario.peerDir, uuid.NewString()
	startInitialOpeningSession(t, sessions, peer)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- selectedProviderInvoke(ctx, sessions, scenario.peerID) }()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("peer did not enter command gate")
	}
	history := initialOpeningHistory(t, sessions, scenario.peerID)
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session", scenario.candidateID, "--dir", scenario.candidateDir, "--continuously", "--with-server", "--server", "http://127.0.0.1:24188", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	err := process.Execute(inputs.Input)
	if !errors.Is(err, cause) && (err == nil || !strings.Contains(inputs.Stderr(), cause.Error())) {
		t.Fatalf("host failure lost primary cause: %v stderr=%s", err, inputs.Stderr())
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, history)
	select {
	case err := <-done:
		t.Fatalf("host rollback ended gated peer: %v", err)
	default:
	}
	retry := support.StartProcessCommand(t, process, inputs.Input)
	retryAPI.WaitForURL(t)
	t.Cleanup(func() { retry.Stop(t) })
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, history)
	gate.unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("peer lost result after retry: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer did not complete")
	}
	if err := selectedProviderInvoke(ctx, sessions, scenario.peerID); err != nil {
		t.Fatal(err)
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

func testSelectedReplayFailureRetry(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	recordPath := filepath.Join(t.TempDir(), "selected.jsonl")
	request.RuntimeSelection.Recording.RecordPath = recordPath
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	history := initialOpeningHistory(t, sessions, scenario.candidateID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	valid, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(t.TempDir(), "retry.jsonl")
	inputs := selectedReplayRetryInputs(t, scenario, selectedPath)
	err = process.Execute(inputs.Input)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing selected replay lost its cause: %v", err)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	// Recognizable portable input with an unsupported field must preserve the
	// typed diagnostic and redact its payload before returning to the customer.
	private := "selected-private-replay-payload"
	invalid := fmt.Sprintf(`{"recordingKind":%q,"unknown":%q}`, recordings.KindJavaScriptFactorySession, private)
	if err := os.WriteFile(selectedPath, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs = selectedReplayRetryInputs(t, scenario, selectedPath)
	err = process.Execute(inputs.Input)
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) || inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticMalformed {
		t.Fatalf("invalid selected replay lost typed classification: %v", err)
	}
	if strings.Contains(inputs.Stdout()+inputs.Stderr(), private) || strings.Contains(fmt.Sprint(inputErr.Diagnostic), private) {
		t.Fatal("replay diagnostic exposed private payload")
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	if err := os.WriteFile(selectedPath, selectedForeignReplay(t, valid), 0o600); err != nil {
		t.Fatal(err)
	}
	err = process.Execute(selectedReplayRetryInputs(t, scenario, selectedPath).Input)
	if !errors.As(err, &inputErr) || inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticForeignReference {
		t.Fatalf("foreign selected replay lost typed classification: %v", err)
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	peerAfterFailure := initialOpeningHistory(t, sessions, scenario.peerID)
	if err := os.WriteFile(selectedPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningReplay(t, process, selectedPath, scenario.candidateID, history)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerAfterFailure)
	assertSelectedHostedLegacyReplay(t, sessions, scenario, selectedPath, history)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func selectedForeignReplay(t *testing.T, valid []byte) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(valid)), "\n")
	for i, line := range lines {
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		var event factorydefinitions.FactoryEvent
		if json.Unmarshal(row["event"], &event) != nil || string(event.Type) != "DISPATCH_REQUEST" {
			continue
		}
		var payload factorydefinitions.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatal(err)
		}
		payload.Inputs = []factorydefinitions.DispatchConsumedWorkRef{{WorkID: "foreign-work"}}
		event.Payload, _ = json.Marshal(payload)
		row["event"], _ = json.Marshal(event)
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		lines[i] = string(encoded)
		return []byte(strings.Join(lines, "\n") + "\n")
	}
	t.Fatal("selected recording has no dispatch to corrupt")
	return nil
}

func assertSelectedHostedLegacyReplay(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, path string, history *factorydefinitions.FactoryEventStream) {
	t.Helper()
	peerBefore := initialOpeningHistory(t, sessions, scenario.peerID)
	request := scenario.request()
	request.SessionID = uuid.NewString()
	request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	request.RuntimeSelection.Recording.ReplayPath = path
	request.RuntimeSelection.Host = factorysessions.RuntimeHostRequest{Host: "127.0.0.1", Port: 24117}
	// The selected legacy config must load even when the current authored
	// definition is no longer valid. A hosted replay retains its runtime mode.
	if err := os.WriteFile(request.RuntimeSelection.DefinitionSourcePath, []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	startInitialOpeningSession(t, sessions, request)
	replayed := initialOpeningHistory(t, sessions, request.SessionID)
	// Host readiness precedes replay completion. In particular the dispatch
	// result wakes observers before its following canonical relocation. Join
	// that retained/live cut rather than asserting a readiness-time count.
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	for len(replayed.History) < len(history.History) {
		select {
		case event, ok := <-replayed.Events:
			if !ok {
				t.Fatal("hosted replay closed before restoring selected history")
			}
			replayed.History = append(replayed.History, event)
		case <-ctx.Done():
			t.Fatalf("hosted replay lost selected history: got=%d want>=%d: %v", len(replayed.History), len(history.History), ctx.Err())
		}
	}
	for i, event := range history.History {
		if replayed.History[i].Id != event.Id || replayed.History[i].Type != event.Type {
			t.Fatalf("hosted replay changed selected event %d: %#v", i, replayed.History[i])
		}
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerBefore)
}

// K-F3 holds two live provider commands while the ordinary CLI inspects a
// recording on that same process. Recorded time must remain distinct from the
// selected 2041 process timestamp; closing inspection must preserve live Work.
func assertPreparationReplayBesideLiveWork(t *testing.T, process support.Process, sessions factorysessions.Service, scenario initialOpeningScenario) {
	t.Helper()
	id := uuid.NewString()
	recordedAt := time.Date(2026, 7, 12, 12, 0, 2, 0, time.UTC)
	path, payload := writePreparationRecording(t, id, recordedAt)
	writer := &preparationInspectionWriter{entered: make(chan struct{}), release: make(chan struct{})}
	unblock := func() { writer.once.Do(func() { close(writer.release) }) }
	t.Cleanup(unblock)
	inputs := selectedReplayRetryInputs(t, scenario, path)
	inputs.Input.Args = []string{"you", "run", "--dir", scenario.candidateDir, "--session", uuid.NewString(), "--replay", path, "--no-record"}
	inputs.Input.Stdout = writer
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input); close(done) }()
	t.Cleanup(func() {
		unblock()
		if err := <-done; err != nil {
			t.Errorf("historical CLI inspection: %v", err)
		}
	})
	select {
	case <-writer.entered:
	case err := <-done:
		t.Fatalf("inspection ended before readiness: %v; %s", err, inputs.Stderr())
	case <-t.Context().Done():
		t.Fatal("inspection never reached CLI output")
	}
	assertPreparationHistoricalFacts(t, sessions, scenario, id, recordedAt, path, payload)
	unblock()
}

func writePreparationRecording(t *testing.T, id string, recordedAt time.Time) (string, []byte) {
	t.Helper()
	recording, err := recordings.BuildPortableRecording(recordings.PortableRecordingCanonicalFacts{
		SessionID: id, Status: "SUCCEEDED", OrchestratorKind: "JAVASCRIPT", SourceRef: "workflow/selected.js",
		SourceHash: "sha256:" + strings.Repeat("1", 64), PolicyHash: "sha256:" + strings.Repeat("3", 64),
		Artifacts:  []recordings.PortableRecordingCanonicalArtifact{{ID: "checkpoint-artifact", Kind: "CHECKPOINT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("4", 64), SizeBytes: 42, CreatedAt: recordedAt, SecretsRedacted: 2}},
		Checkpoint: &recordings.PortableRecordingCanonicalCheckpoint{ID: "selected-checkpoint", ArtifactID: "checkpoint-artifact", Timestamp: recordedAt},
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"recorded-start","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-07-12T12:00:00Z"},"payload":{}}`),
			json.RawMessage(`{"id":"recorded-checkpoint","type":"JAVASCRIPT_CHECKPOINT_REF","context":{"sequence":1,"eventTime":"2026-07-12T12:00:01Z","checkpointId":"selected-checkpoint"},"payload":{"artifactIds":["checkpoint-artifact"]}}`),
			json.RawMessage(`{"id":"recorded-done","type":"SESSION_COMPLETED","context":{"sequence":2,"eventTime":"2026-07-12T12:00:02Z"},"payload":{}}`),
		},
		Result: &recordings.PortableRecordingCanonicalResult{Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"recorded"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(recording)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selected.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, payload
}

func assertPreparationHistoricalFacts(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, id string, recordedAt time.Time, path string, payload []byte) {
	t.Helper()
	read, err := sessions.GetSession(t.Context(), id)
	if err != nil || read.ResolvedSource.SourceRef != "workflow/selected.js" || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("selected historical facts = %#v, %v", read, err)
	}
	after := 1
	events, err := sessions.ReadEvents(t.Context(), id, factorysessions.EventReconnectRequest{AfterSequence: &after})
	if err != nil || len(events.Events) != 1 {
		t.Fatalf("selected recorded events = %#v, %v", events, err)
	}
	var event struct {
		ID      string `json:"id"`
		Context struct {
			EventTime time.Time `json:"eventTime"`
		} `json:"context"`
	}
	if err := json.Unmarshal(events.Events[0], &event); err != nil || event.ID != "recorded-done" || !event.Context.EventTime.Equal(recordedAt) {
		t.Fatalf("replay time leaked process clock: %s, %v", events.Events[0], err)
	}
	for _, liveID := range []string{scenario.candidateID, scenario.peerID} {
		initialOpeningHistory(t, sessions, liveID)
	}
	result, err := sessions.GetResult(t.Context(), id, factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal})
	if err != nil || string(result.PrimaryResult) != `{"answer":"recorded"}` {
		t.Fatalf("historical result = %#v, %v", result, err)
	}
	afterPayload, err := os.ReadFile(path)
	if err != nil || string(afterPayload) != string(payload) {
		t.Fatalf("inspection modified recording: %v", err)
	}
}

type preparationInspectionWriter struct {
	entered, release chan struct{}
	once             sync.Once
	ready            sync.Once
}

func (writer *preparationInspectionWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Replayed Factory Session:") {
		writer.ready.Do(func() { close(writer.entered) })
		<-writer.release
	}
	return len(p), nil
}
